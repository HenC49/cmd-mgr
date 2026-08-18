package cmd

import (
	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:   "edit <别名>",
	Short: "编辑指定别名（交互表单）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireTTY(); err != nil {
			return err
		}
		st, err := openStore()
		if err != nil {
			return err
		}
		name := args[0]
		a, ok := st.Get(name)
		if !ok {
			return notFoundError(st.List(), name)
		}
		orig := *a
		return runEditForm(st, &orig, nil)
	},
}

func init() {
	rootCmd.AddCommand(editCmd)
}
