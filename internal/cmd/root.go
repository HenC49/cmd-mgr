// Package cmd 定义 cm 的全部子命令。
package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"cmd-mgr/internal/form"
	"cmd-mgr/internal/model"
	"cmd-mgr/internal/picker"
	"cmd-mgr/internal/runner"
	"cmd-mgr/internal/store"
	"cmd-mgr/internal/ui"
)

var version = "0.1.0"

// ExitError 携带被执命令的退出码，由 main 转为 cm 自身的进程退出码。
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("命令退出码 %d", e.Code) }

// pickMode 对应隐藏的 --pick：TUI 选中后只打印命令（供 shell 集成 eval）。
var pickMode bool

var rootCmd = &cobra.Command{
	Use:     "cm",
	Short:   "cm - 命令别名管理器，记住那些记不清的命令",
	Version: version,
	Args:    cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRoot()
	},
}

// Execute 由 main 调用。
func Execute() error { return rootCmd.Execute() }

func init() {
	rootCmd.Flags().BoolVar(&pickMode, "pick", false, "选中后只打印命令（供 shell 集成 eval 使用）")
	_ = rootCmd.Flags().MarkHidden("pick")
}

// openStore 打开别名库。
func openStore() (*store.Store, error) {
	st, err := store.Open()
	if err != nil {
		return nil, err
	}
	return st, nil
}

// runRoot 裸 `cm` 的主循环：TUI 选择 → 执行/增删改 → 回到选择，直到退出或执行。
func runRoot() error {
	if !ui.IsTTY(os.Stdin) {
		return fmt.Errorf("需要交互式终端；非交互场景请用 cm list / cm search / cm run")
	}
	if !pickMode && !ui.IsTTY(os.Stdout) {
		return fmt.Errorf("stdout 不是终端；如需捕获结果请用 cm --pick 或 cm list")
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	for {
		items := st.List()
		// --pick 模式下 TUI 渲染到 stderr，把 stdout 留给选中的命令；
		// 同时把颜色检测改绑 stderr，避免 stdout 被捕获时样式被降级
		var out io.Writer
		if pickMode {
			out = os.Stderr
			ui.AdoptStderrColor()
		}
		res, err := picker.Run(items, out)
		if err != nil {
			return err
		}
		switch res.Action {
		case picker.ActionQuit:
			return nil
		case picker.ActionExecute:
			_ = st.RecordUse(res.Alias.Alias) // 统计失败不阻塞执行
			if pickMode {
				fmt.Println(res.Alias.Command)
				return nil
			}
			code := runner.Run(res.Alias.Command)
			if code == 127 { // command not found
				printShellFnHint(res.Alias.Command)
			}
			return &ExitError{Code: code}
		case picker.ActionAdd:
			if err := runAddForm(st, nil, out); err != nil {
				return err
			}
		case picker.ActionEdit:
			if err := runEditForm(st, res.Alias, out); err != nil {
				return err
			}
		case picker.ActionDelete:
			if _, err := st.Remove(res.Alias.Alias); err != nil {
				return err
			}
			notice("%s", ui.OKStyle.Render("✓ 已删除 "+res.Alias.Alias))
		}
	}
}

// notice 输出提示信息。--pick 模式下 stdout 被 shell 捕获用于 eval，
// 提示必须走 stderr，否则会被当成命令执行。
func notice(format string, a ...any) {
	w := io.Writer(os.Stdout)
	if pickMode {
		w = os.Stderr
	}
	fmt.Fprintf(w, format+"\n", a...)
}

// aliasNames 返回已占用别名列表，exclude 用于编辑时排除自身。
func aliasNames(st *store.Store, exclude string) []string {
	var names []string
	for _, a := range st.Aliases() {
		if a.Alias != exclude {
			names = append(names, a.Alias)
		}
	}
	return names
}

// runAddForm 打开新增表单并保存；output 同 picker 的输出目标（pick 模式为 stderr）。
func runAddForm(st *store.Store, prefill *model.Alias, output io.Writer) error {
	a, outcome, err := form.Run(prefill, aliasNames(st, ""), false, output)
	if err != nil {
		return err
	}
	if outcome == form.OutcomeSaved {
		if err := st.Add(a); err != nil {
			return err
		}
		notice("%s", ui.OKStyle.Render("✓ 已保存别名 "+a.Alias))
	}
	return nil
}

// runEditForm 打开编辑表单并保存（支持改名）。
func runEditForm(st *store.Store, orig *model.Alias, output io.Writer) error {
	prefill := *orig
	a, outcome, err := form.Run(&prefill, aliasNames(st, orig.Alias), true, output)
	if err != nil {
		return err
	}
	if outcome == form.OutcomeSaved {
		if err := st.Rename(orig.Alias, a); err != nil {
			return err
		}
		notice("%s", ui.OKStyle.Render("✓ 已更新别名 "+a.Alias))
	}
	return nil
}

// printShellFnHint 命令以 127（command not found）退出时，提示这可能是
// shell 函数或别名（如 SDKMAN! 的 sdk、nvm），子进程方式无法使用，
// 需安装 shell 集成由当前 shell 执行。
func printShellFnHint(command string) {
	name := command
	if words := strings.Fields(command); len(words) > 0 {
		name = words[0]
	}
	fmt.Fprintf(os.Stderr, "cm: 提示: %q 可能是 shell 函数或别名（如 SDKMAN! 的 sdk、nvm），\n", name)
	fmt.Fprintln(os.Stderr, "cm: 这类命令只在当前 shell 中存在，且常用于修改环境变量，子进程执行无效果。")
	fmt.Fprintln(os.Stderr, `cm: 安装 shell 集成后由当前 shell 执行即可: eval "$(cm init zsh)"（加入 ~/.zshrc）`)
}

// requireTTY 交互式子命令的入口检查。
func requireTTY() error {
	if !ui.IsTTY(os.Stdin) || !ui.IsTTY(os.Stdout) {
		return fmt.Errorf("该命令需要交互式终端")
	}
	return nil
}
