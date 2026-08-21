package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"cmd-mgr/internal/history"
	"cmd-mgr/internal/ui"
)

var historyLimit int

var historyCmd = &cobra.Command{
	Use:   "history [别名]",
	Short: "查看执行历史（命令与结果）",
	Long: `查看执行历史：每次执行记录替换参数后的实际命令、参数值与结果。

  cm history           # 最近 20 条
  cm history dsync     # 指定别名的最近记录
  cm history -n 50     # 条数上限

历史保存在配置目录 history.json（上限 500 条，自动淘汰最旧），
供 TUI 中的"最近执行"展示与参数表单 tab 复制参数使用。`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		hist, err := history.Open()
		if err != nil {
			return err
		}
		var recs []history.Record
		if len(args) > 0 {
			recs = hist.Recent(args[0], historyLimit)
		} else {
			recs = hist.List(historyLimit)
		}
		if len(recs) == 0 {
			if len(args) > 0 {
				fmt.Printf("别名 %q 暂无执行历史\n", args[0])
			} else {
				fmt.Println("暂无执行历史，执行一次别名后即有记录")
			}
			return nil
		}
		printHistory(recs)
		return nil
	},
}

func init() {
	historyCmd.Flags().IntVarP(&historyLimit, "number", "n", 20, "最多显示条数")
	rootCmd.AddCommand(historyCmd)
}

// statusText 状态 + 耗时，如 "✓ 3.2s"、"✗3"、"·"（shell eval 模式结果未知）。
func statusText(r history.Record) string {
	s := ui.ExitStatus(r.ExitCode)
	if d := ui.DurationStr(r.DurationMs); d != "" {
		s += " " + ui.DimStyle.Render(d)
	}
	return s
}

// printHistory 以对齐表格输出历史（新的在前；非终端时 lipgloss 自动去色）。
func printHistory(recs []history.Record) {
	width := 80
	if ui.IsTTY(os.Stdout) {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
			width = w
		}
	}

	aliasW := 6
	for _, r := range recs {
		if w := ui.Width(r.Alias); w > aliasW {
			aliasW = w
		}
	}
	aliasW = min(aliasW+2, 26)
	cmdW := min(48, max(20, width-aliasW-28))

	fmt.Println(ui.DimStyle.Render(ui.PadRight("时间", 13) + ui.PadRight("别名", aliasW) + ui.PadRight("命令", cmdW) + "结果"))
	for _, r := range recs {
		row := ui.DimStyle.Render(ui.PadRight(r.StartedAt.Format("01-02 15:04"), 13)) +
			ui.AliasStyle.Render(ui.PadRight(r.Alias, aliasW)) +
			ui.CmdStyle.Render(ui.PadRight(ui.Truncate(r.Command, cmdW-1), cmdW)) +
			statusText(r)
		fmt.Println(row)
	}
}
