// 执行链路的共享逻辑：执行前解析 {{占位符}} 参数（含密钥引用
// {{user@服务}} / {{pass@服务}}，从系统密码管理器读取），执行后记录历史。
package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"cmd-mgr/internal/history"
	"cmd-mgr/internal/model"
	"cmd-mgr/internal/prompt"
	"cmd-mgr/internal/secret"
	"cmd-mgr/internal/ui"
)

// resolved 待执行的命令（占位符已替换完成）。
type resolved struct {
	command string            // 替换参数后的命令（真实值，仅用于执行/打印）
	masked  string            // 密钥脱敏后的命令（历史与展示用，不上屏真实密码）
	params  map[string]string // 填写的参数值（密钥项已脱敏；无占位符时为 nil）
}

// openHistory 打开执行历史；历史是附属功能，失败不阻塞执行（返回 nil）。
func openHistory() *history.Store {
	hist, err := history.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cm: 打开执行历史失败（不影响执行）: %v\n", err)
		return nil
	}
	return hist
}

// resolveCommand 处理占位符：无占位符直接返回原命令；有则打开参数表单
// （预填最近一次的值，tab 可复制历史参数），确认后再读取密钥引用。
// confirmed=false 表示用户取消。
func resolveCommand(a *model.Alias, hist *history.Store, output io.Writer) (r resolved, confirmed bool, err error) {
	params := model.ExtractParams(a.Command)
	if len(params) == 0 {
		return resolved{command: a.Command, masked: a.Command}, true, nil
	}
	hasRegular := false
	for _, p := range params {
		if !p.Secret {
			hasRegular = true
			break
		}
	}
	if !ui.IsTTY(os.Stdin) {
		if hasRegular {
			names := make([]string, len(params))
			for i, p := range params {
				names[i] = p.Name
			}
			return resolved{}, false, fmt.Errorf("命令 %s 含参数（%s），需交互式终端填写；历史参数可用 cm history %s 查看",
				a.Alias, strings.Join(names, ", "), a.Alias)
		}
		// 只有密钥引用：读取系统密码管理器不需要终端输入，脚本/管道可直接用
		values, err := resolveSecrets(params, false)
		if err != nil {
			return resolved{}, false, err
		}
		return finishResolved(a.Command, params, values), true, nil
	}
	res, err := prompt.Run(prompt.Config{
		Alias:    a.Alias,
		Template: a.Command,
		Params:   params,
		History:  hist.Recent(a.Alias, 5),
	}, output)
	if err != nil {
		return resolved{}, false, err
	}
	if !res.Confirm {
		return resolved{}, false, nil
	}
	// 表单退出后再取密钥：读取可能触发系统密码管理器的授权弹窗，
	// 缺失项的交互创建也需要普通终端（此时已从 raw mode 恢复）
	secrets, err := resolveSecrets(params, true)
	if err != nil {
		return resolved{}, false, err
	}
	for k, v := range secrets {
		res.Values[k] = v
	}
	return finishResolved(a.Command, params, res.Values), true, nil
}

// finishResolved 生成真实命令与脱敏版本：真实值只进最终执行的命令，
// 历史/预览一律用脱敏值，密码不上屏、不落盘。
func finishResolved(template string, params []model.Param, values map[string]string) resolved {
	masked := model.MaskedValues(params, values)
	return resolved{
		command: model.RenderCommand(template, values),
		masked:  model.RenderCommand(template, masked),
		params:  masked,
	}
}

// secretRef 一个密钥引用：类别（user/pass）+ 对应的参数名。
type secretRef struct{ kind, name string }

// 间接调用系统密码管理器与凭据创建（测试可替换）。
var (
	fetchSecret  = secret.Fetch
	createSecret = secret.Create
	createCreds  = createInteractive
)

// resolveSecrets 读取全部密钥引用的值，按服务去重（user@x 与 pass@x 是
// 同一条钥匙串项）。缺失的服务在交互模式下现场创建，否则报错。
func resolveSecrets(params []model.Param, interactive bool) (map[string]string, error) {
	refs := map[string][]secretRef{}
	var order []string
	for _, p := range params {
		if !p.Secret {
			continue
		}
		kind, svc, _ := model.ParseSecretName(p.Name)
		if _, seen := refs[svc]; !seen {
			order = append(order, svc)
		}
		refs[svc] = append(refs[svc], secretRef{kind, p.Name})
	}

	out := make(map[string]string, len(order))
	for _, svc := range order {
		rs := refs[svc]
		vals := make(map[string]string, len(rs))
		missing := false
		for _, r := range rs {
			v, err := fetchSecret(r.kind, svc)
			if err != nil {
				if !errors.Is(err, secret.ErrNotFound) {
					return nil, err
				}
				missing = true // user@/pass@ 同属一项，缺则全缺
				break
			}
			vals[r.kind] = v
		}
		if !missing {
			for _, r := range rs {
				out[r.name] = vals[r.kind]
			}
			continue
		}
		account, password, err := createCreds(svc, needsUser(rs), interactive)
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			if r.kind == secret.KindUser {
				out[r.name] = account
			} else {
				out[r.name] = password
			}
		}
	}
	return out, nil
}

// needsUser 引用中是否包含 user@（决定创建时是否要输入账号）。
func needsUser(rs []secretRef) bool {
	for _, r := range rs {
		if r.kind == secret.KindUser {
			return true
		}
	}
	return false
}

// createInteractive 现场创建缺失的凭据：账号明文输入（仅当命令引用了
// user@），密码隐藏输入。提示全部走 stderr——--pick/--print 模式下
// stdout 被 shell 捕获用于 eval，不能混入交互内容。非交互模式直接报错。
func createInteractive(service string, needUser, interactive bool) (account, password string, err error) {
	if !interactive {
		return "", "", fmt.Errorf("系统密码管理器中没有 %q 的凭据，非交互模式无法创建；请在终端交互执行一次，或用钥匙串访问手动添加", service)
	}
	fmt.Fprintf(os.Stderr, "\ncm: 系统密码管理器中没有 %q 的凭据，现在创建（只存入系统密码管理器，不写入 cm 的任何文件）\n", service)
	if needUser {
		fmt.Fprint(os.Stderr, "账号: ")
		line, err := readLine()
		if err != nil {
			return "", "", err
		}
		account = strings.TrimSpace(line)
	}
	fmt.Fprint(os.Stderr, "密码（输入不回显）: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", "", fmt.Errorf("读取密码失败: %w", err)
	}
	password = string(pw)
	if strings.TrimSpace(password) == "" {
		return "", "", fmt.Errorf("密码为空，已取消执行")
	}
	if err := createSecret(service, account, password); err != nil {
		return "", "", err
	}
	fmt.Fprintf(os.Stderr, "cm: ✓ 已存入系统密码管理器（服务 %s）\n\n", service)
	return account, password, nil
}

// readLine 从 stdin 读一行（交互式创建凭据时的明文输入）。
func readLine() (string, error) {
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return line, nil
}

// recordHistory 记录一次执行（命令与参数均为密钥脱敏版本）。code 为 nil
// 表示 shell 集成模式下由当前 shell eval，cm 无法得知结果。
func recordHistory(hist *history.Store, a *model.Alias, r resolved, start time.Time, code *int) {
	if hist == nil {
		return
	}
	err := hist.Add(history.Record{
		Alias:      a.Alias,
		Template:   a.Command,
		Command:    r.masked,
		Params:     r.params,
		ExitCode:   code,
		StartedAt:  start,
		DurationMs: time.Since(start).Milliseconds(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "cm: 记录执行历史失败: %v\n", err)
	}
}
