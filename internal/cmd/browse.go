package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"cmd-mgr/internal/browse"
	"cmd-mgr/internal/model"
)

var browseCmd = &cobra.Command{
	Use:     "browse",
	Aliases: []string{"b"},
	Short:   "TUI 浏览 PATH 中所有可用命令，选中可直接创建别名",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireTTY(); err != nil {
			return err
		}
		st, err := openStore()
		if err != nil {
			return err
		}
		entry, ok, err := browse.Run(os.Stdout)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		prefill := &model.Alias{Command: entry.Name + " "}
		return runAddForm(st, prefill, nil)
	},
}

func init() {
	rootCmd.AddCommand(browseCmd)
}
