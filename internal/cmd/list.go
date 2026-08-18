package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"cmd-mgr/internal/model"
	"cmd-mgr/internal/ui"
)

var listVerbose bool

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "列出所有别名",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := openStore()
		if err != nil {
			return err
		}
		items := st.List()
		if len(items) == 0 {
			fmt.Println("暂无别名，用 cm add 添加第一条")
			return nil
		}
		printItems(items, listVerbose)
		return nil
	},
}

func init() {
	listCmd.Flags().BoolVarP(&listVerbose, "verbose", "v", false, "显示标签与使用统计")
	rootCmd.AddCommand(listCmd)
}

// printItems 以对齐的表格输出别名列表（非终端时 lipgloss 自动去色）。
func printItems(items []*model.Alias, verbose bool) {
	width := 80
	if ui.IsTTY(os.Stdout) {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
			width = w
		}
	}

	aliasW := 6
	for _, a := range items {
		if w := ui.Width(a.Alias); w > aliasW {
			aliasW = w
		}
	}
	aliasW = min(aliasW+2, 26)

	cmdW := min(44, max(20, width-aliasW-20))
	descW := max(10, width-aliasW-cmdW-4)

	header := ui.PadRight("别名", aliasW) + ui.PadRight("命令", cmdW) + "描述"
	if verbose {
		header += "  标签  使用"
	}
	fmt.Println(ui.DimStyle.Render(header))
	for _, a := range items {
		row := ui.AliasStyle.Render(ui.PadRight(a.Alias, aliasW)) +
			ui.CmdStyle.Render(ui.PadRight(ui.Truncate(a.Command, cmdW-1), cmdW)) +
			ui.Truncate(a.Description, descW)
		if verbose {
			tags := strings.Join(a.Tags, ",")
			row += "  " + ui.Truncate(tags, 16) + "  " + fmt.Sprintf("%d次 %s", a.UsedCount, ui.TimeAgo(a.LastUsedAt))
		}
		fmt.Println(row)
	}
}
