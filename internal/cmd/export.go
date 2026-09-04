// cm export：导出全部别名为 JSON——备份、迁移机器、分享给同事。
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"cmd-mgr/internal/input"
	"cmd-mgr/internal/store"
	"cmd-mgr/internal/ui"
)

var exportCmd = &cobra.Command{
	Use:   "export [文件]",
	Short: "导出全部别名为 JSON（缺省输出到 stdout）",
	Long: `导出全部别名为 JSON。

  cm export > backup.json     # 导出到 stdout，重定向保存
  cm export backup.json       # 直接写入文件（权限 0600）
  cm export team.json         # 分享给同事：含 {{pass@服务}} 的命令只有引用、
                              # 不含真实密码，导出文件可安全外发

导出格式与别名库 aliases.json 同构：可作为备份直接放回原位，
也可在任何机器上用 cm import 导入。执行历史不包含在内。`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := openStore()
		if err != nil {
			return err
		}
		raw, err := st.ExportJSON()
		if err != nil {
			return fmt.Errorf("序列化别名库失败: %w", err)
		}
		if len(args) == 0 {
			os.Stdout.Write(raw)
			fmt.Println()
			return nil
		}
		if err := os.WriteFile(args[0], raw, 0o600); err != nil {
			return fmt.Errorf("写入 %s: %w", args[0], err)
		}
		fmt.Println(ui.OKStyle.Render(fmt.Sprintf("✓ 已导出 %d 条别名到 %s", len(st.Aliases()), args[0])))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(exportCmd)
}

// runExportFlow TUI（主选择器 ctrl+x）里的导出流程：路径输入表单（预填
// 带日期的默认名）→ 写文件 → 结果框展示。output 同 picker 的输出目标。
func runExportFlow(st *store.Store, output io.Writer) error {
	def := "~/cm-aliases-" + time.Now().Format("20060102") + ".json"
	path, ok, err := input.Run(input.Config{
		Title:       "cm · 导出别名",
		Label:       "文件",
		Default:     def,
		Placeholder: "导出文件路径（~ 按家目录展开）",
		Validate: func(v string) error {
			if v == "" {
				return fmt.Errorf("请填写导出文件路径")
			}
			if dir := filepath.Dir(expandHome(v)); dir == "" {
				return fmt.Errorf("路径不完整")
			} else if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				return fmt.Errorf("目录不存在: %s", dir)
			}
			return nil
		},
	}, output)
	if err != nil || !ok {
		return err
	}
	n, err := writeExportFile(st, path)
	if err != nil {
		// 路径已在表单里校验过，这里的失败多为权限问题；用结果框展示而非退出 cm
		return input.Show("导出失败", []string{ui.ErrorStyle.Render(err.Error())}, output)
	}
	return input.Show("导出完成", []string{
		"已导出 " + strconv.Itoa(n) + " 条别名到 " + path,
		"格式与别名库相同，可用 cm import 导入；密钥引用不含真实密码，可安全分享",
	}, output)
}

// writeExportFile 展开路径并写入导出内容，返回别名条数。
func writeExportFile(st *store.Store, path string) (int, error) {
	raw, err := st.ExportJSON()
	if err != nil {
		return 0, fmt.Errorf("序列化别名库失败: %w", err)
	}
	if err := os.WriteFile(expandHome(path), raw, 0o600); err != nil {
		return 0, fmt.Errorf("写入 %s: %w", path, err)
	}
	return len(st.Aliases()), nil
}
