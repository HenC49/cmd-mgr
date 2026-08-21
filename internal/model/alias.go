// Package model 定义别名数据模型与校验规则。
package model

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Alias 一条命令别名。
type Alias struct {
	Alias       string    `json:"alias"`       // 短名，如 dsync
	Command     string    `json:"command"`     // 完整命令
	Description string    `json:"description"` // 描述
	Tags        []string  `json:"tags,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UsedCount   int       `json:"used_count"` // 执行成功后 +1，用于排序
	LastUsedAt  time.Time `json:"last_used_at,omitempty"`
}

// Validate 修剪字段后校验合法性。
func (a *Alias) Validate() error {
	a.Alias = strings.TrimSpace(a.Alias)
	a.Command = strings.TrimSpace(a.Command)
	a.Description = strings.TrimSpace(a.Description)
	if a.Alias == "" {
		return fmt.Errorf("别名不能为空")
	}
	if strings.ContainsAny(a.Alias, " \t\n\"'`$;|&<>(){}") {
		return fmt.Errorf("别名 %q 含非法字符，请只使用字母、数字、-、_、. 等", a.Alias)
	}
	if a.Command == "" {
		return fmt.Errorf("命令不能为空")
	}
	if err := validatePlaceholders(a.Command); err != nil {
		return err
	}
	return nil
}

// SearchText 返回供模糊匹配使用的整合文本（别名+命令+描述+标签）。
func (a *Alias) SearchText() string {
	return strings.Join([]string{a.Alias, a.Command, a.Description, strings.Join(a.Tags, " ")}, " ")
}

// ParseTags 解析逗号或空格分隔的标签输入，去重去空。
func ParseTags(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '、' || r == '，'
	})
	seen := make(map[string]bool, len(fields))
	var tags []string
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" && !seen[f] {
			seen[f] = true
			tags = append(tags, f)
		}
	}
	return tags
}

// SortByUsage 按使用情况排序：使用次数降序 → 最近使用降序 → 创建时间降序 → 别名升序。
func SortByUsage(items []*Alias) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch {
		case a.UsedCount != b.UsedCount:
			return a.UsedCount > b.UsedCount
		case !a.LastUsedAt.Equal(b.LastUsedAt):
			return a.LastUsedAt.After(b.LastUsedAt)
		case !a.CreatedAt.Equal(b.CreatedAt):
			return a.CreatedAt.After(b.CreatedAt)
		default:
			return a.Alias < b.Alias
		}
	})
}
