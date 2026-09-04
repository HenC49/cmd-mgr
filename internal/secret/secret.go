// Package secret 从系统密码管理器读取/创建凭据，支撑密钥占位符
// {{user@服务}} / {{pass@服务}}：
//
//	macOS:   钥匙串（security CLI，读账号 = 项的 acct 属性，读密码 = 项内容）
//	Linux:   libsecret（secret-tool，账号存为 username 属性）
//	Windows: 暂不支持（返回 ErrUnsupported）
//
// 密码创建走 `security -i` 的 stdin 管道而非命令行参数——避免密码出现在
// ps 进程列表中；引号转义已实测与 security 交互解析器往返一致。
package secret

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// Kind 密钥类别：user（账号）/ pass（密码）。
const (
	KindUser = "user"
	KindPass = "pass"
)

var (
	// ErrNotFound 服务对应的钥匙串项不存在（调用方可据此引导创建）。
	ErrNotFound = errors.New("未找到")
	// ErrUnsupported 当前系统没有可用的系统密码管理器后端。
	ErrUnsupported = errors.New("当前系统暂不支持系统密码管理器（macOS 钥匙串 / Linux libsecret）")
)

// Fetch 读取服务的账号（kind=user）或密码（kind=pass）。
func Fetch(kind, service string) (string, error) {
	switch kind {
	case KindUser, KindPass:
	default:
		return "", fmt.Errorf("未知密钥类别 %q（应为 user 或 pass）", kind)
	}
	switch runtime.GOOS {
	case "darwin":
		return fetchDarwin(kind, service)
	case "linux":
		return fetchLinux(kind, service)
	default:
		return "", ErrUnsupported
	}
}

// Create 在系统密码管理器中创建服务的凭据（账号 + 密码）。
func Create(service, account, password string) error {
	switch runtime.GOOS {
	case "darwin":
		return createDarwin(service, account, password)
	case "linux":
		return createLinux(service, account, password)
	default:
		return ErrUnsupported
	}
}

// ---------- 可注入的命令执行（测试替换 runCmd） ----------

// runner 执行外部命令：stdin 非空时接入子进程标准输入，返回 stdout。
type runner func(name string, args []string, stdin string) ([]byte, error)

var runCmd runner = func(name string, args []string, stdin string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("%s: %w", strings.TrimSpace(errBuf.String()), err)
		}
		return out, err
	}
	return out, nil
}

// ---------- macOS：钥匙串（security CLI） ----------

// acctRe 从 find-generic-password 的属性输出中提取账号（acct）。
var acctRe = regexp.MustCompile(`"acct"<blob>="([^"]*)"`)

func fetchDarwin(kind, service string) (string, error) {
	args := []string{"find-generic-password", "-s", service}
	if kind == KindPass {
		args = append(args, "-w") // -w 只输出密码本身
	}
	out, err := runCmd("/usr/bin/security", args, "")
	if err != nil {
		return "", fmt.Errorf("钥匙串中没有 %q 对应的项（或访问被拒绝）: %w", service, ErrNotFound)
	}
	if kind == KindPass {
		return strings.TrimRight(string(out), "\r\n"), nil
	}
	m := acctRe.FindSubmatch(out)
	if m == nil {
		return "", fmt.Errorf("钥匙串项 %q 没有账号（acct）属性，请在钥匙串访问中补全", service)
	}
	return string(m[1]), nil
}

// createDarwin 经 `security -i` 的 stdin 创建钥匙串项。
// 转义规则按 security 交互解析器（同 shell：双引号内 \" 与 \\），
// 已实测含空格/引号/反斜杠/$&; 的密码可完整往返。
func createDarwin(service, account, password string) error {
	line, err := buildAddLine(service, account, password)
	if err != nil {
		return err
	}
	if _, err := runCmd("/usr/bin/security", []string{"-i"}, line+"\n"); err != nil {
		return fmt.Errorf("写入钥匙串失败: %w", err)
	}
	return nil
}

// buildAddLine 构造 `security -i` 的 add-generic-password 命令行；
// 三个字段均含换行时报错——交互模式按行解析，换行无法转义。
func buildAddLine(service, account, password string) (string, error) {
	for name, v := range map[string]string{"服务名": service, "账号": account, "密码": password} {
		if strings.ContainsAny(v, "\n\r") {
			return "", fmt.Errorf("%s不能包含换行符", name)
		}
	}
	return fmt.Sprintf("add-generic-password -s %s -a %s -w %s",
		quoteItem(service), quoteItem(account), quoteItem(password)), nil
}

// quoteItem security 交互解析器的双引号转义（只处理 " 与 \，其余原样）。
func quoteItem(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

// ---------- Linux：libsecret（secret-tool） ----------

func fetchLinux(kind, service string) (string, error) {
	if kind == KindPass {
		out, err := runCmd("secret-tool", []string{"lookup", "service", service}, "")
		if err != nil {
			return "", fmt.Errorf("libsecret 中没有 %q 对应的项: %w", service, ErrNotFound)
		}
		return strings.TrimRight(string(out), "\r\n"), nil
	}
	// 账号存为 username 属性，从 search 的属性输出中读取
	out, err := runCmd("secret-tool", []string{"search", "service", service}, "")
	if err != nil {
		return "", fmt.Errorf("libsecret 中没有 %q 对应的项: %w", service, ErrNotFound)
	}
	re := regexp.MustCompile(`(?m)^attribute: username = (.*)$`)
	m := re.FindSubmatch(out)
	if m == nil {
		return "", fmt.Errorf("libsecret 项 %q 没有账号（username 属性）；可重新用 cm 创建", service)
	}
	return string(m[1]), nil
}

func createLinux(service, account, password string) error {
	if strings.ContainsAny(service, "\n\r") || strings.ContainsAny(account, "\n\r") {
		return fmt.Errorf("服务名/账号不能包含换行符")
	}
	// 密码经 stdin 传入，不进命令行参数（同 macOS 的考量）
	if _, err := runCmd("secret-tool",
		[]string{"store", "--label=" + service, "service", service, "username", account},
		password+"\n"); err != nil {
		return fmt.Errorf("写入 libsecret 失败: %w", err)
	}
	return nil
}
