package cmd

import (
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"
	"github.com/spf13/cobra"

	"cmd-mgr/internal/model"
)

var searchCmd = &cobra.Command{
	Use:   "search <关键词>",
	Short: "按关键词模糊搜索别名（匹配 别名/命令/描述/标签）",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := openStore()
		if err != nil {
			return err
		}
		q := strings.Join(args, " ")
		items := st.List()
		texts := make([]string, len(items))
		for i, a := range items {
			texts[i] = a.SearchText()
		}
		var matched []*model.Alias
		for _, m := range fuzzy.Find(q, texts) {
			matched = append(matched, items[m.Index])
		}
		if len(matched) == 0 {
			fmt.Printf("未找到匹配 %q 的别名\n", q)
			return nil
		}
		printItems(matched, false)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
}
