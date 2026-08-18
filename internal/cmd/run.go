package cmd

import (
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"
	"github.com/spf13/cobra"

	"cmd-mgr/internal/model"
	"cmd-mgr/internal/runner"
)

var (
	runPrint bool // 配合 shell 集成：只打印命令由当前 shell eval
)

var runCmd = &cobra.Command{
	Use:   "run <别名>",
	Short: "跳过 TUI，直接执行指定别名",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := openStore()
		if err != nil {
			return err
		}
		name := args[0]
		a, ok := st.Get(name)
		if !ok {
			return notFoundError(st.List(), name)
		}
		_ = st.RecordUse(name)
		if runPrint {
			fmt.Println(a.Command)
			return nil
		}
		code := runner.Run(a.Command)
		if code == 127 { // command not found
			printShellFnHint(a.Command)
		}
		return &ExitError{Code: code}
	},
}

// notFoundError 构造"别名不存在"错误，并附带相近建议。
func notFoundError(items []*model.Alias, name string) error {
	names := make([]string, len(items))
	for i, a := range items {
		names[i] = a.Alias
	}
	var sugg []string
	for _, m := range fuzzy.Find(name, names) {
		sugg = append(sugg, names[m.Index])
		if len(sugg) >= 3 {
			break
		}
	}
	if len(sugg) > 0 {
		return fmt.Errorf("未找到别名 %q，相似: %s", name, strings.Join(sugg, ", "))
	}
	return fmt.Errorf("未找到别名 %q，运行 cm list 查看全部", name)
}

func init() {
	runCmd.Flags().BoolVar(&runPrint, "print", false, "只打印命令不执行（供 shell 集成 eval）")
	_ = runCmd.Flags().MarkHidden("print")
	rootCmd.AddCommand(runCmd)
}
