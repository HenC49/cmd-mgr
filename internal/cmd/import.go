// cm import：从导出文件（或 stdin）合并导入别名。
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"cmd-mgr/internal/input"
	"cmd-mgr/internal/model"
	"cmd-mgr/internal/store"
	"cmd-mgr/internal/ui"
)

var (
	importOverwrite bool
	importDryRun    bool
)

var importCmd = &cobra.Command{
	Use:   "import <文件|->",
	Short: "导入别名（- 从 stdin 读）",
	Long: `从导出文件合并导入别名。

  cm import backup.json             # 重名默认跳过，保留本地版本
  cm import --overwrite team.json   # 重名覆盖为文件中的版本
  cm import --dry-run team.json     # 只预览结果，不落盘
  cm export | ssh b@srv 'cm import -'   # 机器间直接管道迁移

接受 cm export 的输出格式（{"version":..,"aliases":[..]}）与纯 JSON 数组；
文件内重名取后者；每条都会校验，非法条目跳过并逐条报告，不影响其余导入。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := readImportSource(args[0])
		if err != nil {
			return err
		}
		list, err := parseImportAliases(data)
		if err != nil {
			return fmt.Errorf("%s: %w", args[0], err)
		}
		st, err := openStore()
		if err != nil {
			return err
		}
		res, err := st.Import(list, importOverwrite, importDryRun)
		if err != nil {
			return err
		}
		reportImport(res, importDryRun)
		return nil
	},
}

func init() {
	importCmd.Flags().BoolVar(&importOverwrite, "overwrite", false, "重名别名覆盖本地版本（默认跳过）")
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "只预览导入结果，不写入")
	rootCmd.AddCommand(importCmd)
}

// readImportSource 读取导入源："-" 为 stdin，否则为文件路径。
func readImportSource(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 %s: %w", path, err)
	}
	return data, nil
}

// parseImportAliases 解析导入内容：优先按 cm export 的 {"aliases":[..]}
// 形状，失败再按纯数组 [..]——手写/裁剪过的文件也能导入。
func parseImportAliases(data []byte) ([]*model.Alias, error) {
	var f struct {
		Version int            `json:"version"`
		Aliases []*model.Alias `json:"aliases"`
	}
	if err := json.Unmarshal(data, &f); err == nil && f.Aliases != nil {
		return f.Aliases, nil
	}
	var list []*model.Alias
	if err := json.Unmarshal(data, &list); err == nil && list != nil {
		return list, nil
	}
	return nil, fmt.Errorf("无法识别的导入格式（期望 cm export 的输出或别名 JSON 数组）")
}

// nameList 渲染别名列示，超过 8 个折叠为 "等 N 个"。
func nameList(names []string) string {
	const cap = 8
	if len(names) <= cap {
		s := ""
		for i, n := range names {
			if i > 0 {
				s += "、"
			}
			s += n
		}
		return "（" + s + "）"
	}
	s := ""
	for i, n := range names[:cap] {
		if i > 0 {
			s += "、"
		}
		s += n
	}
	return fmt.Sprintf("（%s … 等 %d 个）", s, len(names))
}

// reportImport 打印导入结果摘要（CLI）。
func reportImport(res *store.ImportResult, dryRun bool) {
	total := len(res.Added) + len(res.Updated) + len(res.Skipped) + len(res.Invalid)
	if total == 0 {
		fmt.Println(ui.DimStyle.Render("导入文件中没有别名"))
		return
	}
	title := "导入完成"
	if dryRun {
		title = "导入预览（dry-run，未落盘）"
	}
	if s := importSummaryLine(res); s != "" {
		fmt.Println(ui.OKStyle.Render("✓ "+title+": ") + s)
	} else {
		fmt.Println(ui.OKStyle.Render("✓ " + title))
	}
	for _, inv := range res.Invalid {
		fmt.Println(ui.WarnStyle.Render("⚠ 无效条目 " + inv))
	}
	if len(res.Invalid) > 0 {
		fmt.Println(ui.WarnStyle.Render(fmt.Sprintf("⚠ %d 条无效被跳过", len(res.Invalid))))
	}
}

// importSummaryLine 一行式导入摘要（新增/覆盖/跳过），无内容时返回空串。
func importSummaryLine(res *store.ImportResult) string {
	var parts []string
	if n := len(res.Added); n > 0 {
		parts = append(parts, fmt.Sprintf("新增 %d%s", n, nameList(res.Added)))
	}
	if n := len(res.Updated); n > 0 {
		parts = append(parts, fmt.Sprintf("覆盖 %d%s", n, nameList(res.Updated)))
	}
	if n := len(res.Skipped); n > 0 {
		parts = append(parts, fmt.Sprintf("跳过已存在 %d%s", n, nameList(res.Skipped)))
	}
	if len(parts) == 0 {
		return ""
	}
	return joinZH(parts)
}

// runImportFlow TUI（主选择器 ctrl+o）里的导入流程：路径输入表单（enter 时
// 即校验文件存在与格式）→ 合并导入（重名跳过）→ 结果框展示。
func runImportFlow(st *store.Store, output io.Writer) error {
	var list []*model.Alias
	path, ok, err := input.Run(input.Config{
		Title:       "cm · 导入别名",
		Label:       "文件",
		Placeholder: "如: ~/team.json 或 /tmp/backup.json",
		Validate: func(v string) error {
			if v == "" {
				return fmt.Errorf("请填写导入文件路径")
			}
			data, err := os.ReadFile(expandHome(v))
			if err != nil {
				return fmt.Errorf("读取失败: %w", err)
			}
			if list, err = parseImportAliases(data); err != nil {
				return err
			}
			return nil
		},
	}, output)
	if err != nil || !ok {
		return err
	}
	res, err := st.Import(list, false, false)
	if err != nil {
		return input.Show("导入失败", []string{ui.ErrorStyle.Render(err.Error())}, output)
	}
	return input.Show("导入结果", importResultLines(res, path), output)
}

// importResultLines 导入结果的逐行展示（TUI 结果框用）。
func importResultLines(res *store.ImportResult, path string) []string {
	var lines []string
	if s := importSummaryLine(res); s != "" {
		lines = append(lines, s)
	} else {
		lines = append(lines, "导入文件中没有别名")
	}
	for _, inv := range res.Invalid {
		lines = append(lines, ui.WarnStyle.Render("⚠ 无效 "+inv))
	}
	if len(res.Skipped) > 0 {
		lines = append(lines, ui.DimStyle.Render("重名默认保留本地版本；如需覆盖请在终端运行: cm import --overwrite "+path))
	}
	return lines
}

// expandHome 展开路径开头的 ~ / ~/（其余原样返回）。
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// joinZH 中文顿号连接。
func joinZH(parts []string) string {
	s := ""
	for i, p := range parts {
		if i > 0 {
			s += "，"
		}
		s += p
	}
	return s
}
