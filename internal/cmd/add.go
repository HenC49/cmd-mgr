package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"cmd-mgr/internal/model"
	"cmd-mgr/internal/ui"
)

var (
	addAlias string
	addDesc  string
	addTags  string
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "添加别名（无参数时进入交互表单）",
	Long: `添加别名。

交互模式（推荐）:
  cm add                          # 空表单
  cm add -- docker ps -a          # 预填命令后进入表单

非交互模式（脚本友好）:
  cm add -a dsync -d "同步源码" -t "deploy,rsync" -- rsync -avz ./ host:/srv/

命令参数（{{占位符}}）:
  命令中把需要执行时才确定的值写成 {{参数名}}，可附带说明 {{参数名:说明}}，如:
    cm add -a sshsrv -- 'ssh {{user:登录用户名}}@{{host:服务器 IP}}'
  执行该别名时 cm 会弹出参数表单（预填上次值，tab 可复制历史参数），
  说明展示在对应输入框下方，填完即运行。同名占位符只填一次。`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAdd(cmd, args)
	},
}

func init() {
	addCmd.Flags().StringVarP(&addAlias, "alias", "a", "", "别名（非交互模式必填）")
	addCmd.Flags().StringVarP(&addDesc, "desc", "d", "", "描述")
	addCmd.Flags().StringVarP(&addTags, "tags", "t", "", "标签，逗号分隔")
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}

	var command string
	if idx := cmd.ArgsLenAtDash(); idx >= 0 && idx < len(args) {
		command = strings.Join(args[idx:], " ")
	}

	// 非交互：别名与命令（-- 之后）都给全时直接保存
	if addAlias != "" && command != "" {
		a := &model.Alias{
			Alias:       addAlias,
			Command:     command,
			Description: addDesc,
			Tags:        model.ParseTags(addTags),
		}
		if err := st.Add(a); err != nil {
			return err
		}
		fmt.Println(ui.OKStyle.Render("✓ 已保存别名 " + a.Alias))
		return nil
	}
	if addAlias != "" && command == "" {
		return fmt.Errorf("已指定 --alias，但缺少命令；请在 -- 之后写完整命令，例如: cm add -a %s -- <命令>", addAlias)
	}

	// 交互表单
	if err := requireTTY(); err != nil {
		return fmt.Errorf("交互表单需要终端；非交互请用: cm add -a <别名> -- <命令>（%w）", err)
	}
	prefill := &model.Alias{}
	if command != "" {
		prefill.Command = command
	}
	return runAddForm(st, prefill, nil)
}
