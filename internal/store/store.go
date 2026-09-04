// Package store 负责别名库的 JSON 持久化（原子写）与 CRUD。
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cmd-mgr/internal/model"
	"cmd-mgr/internal/platform"
)

const dbVersion = 1

// ErrNotFound 指定别名不存在。
var ErrNotFound = errors.New("别名不存在")

// ErrExists 别名已存在。
var ErrExists = errors.New("别名已存在")

type dbFile struct {
	Version int            `json:"version"`
	Aliases []*model.Alias `json:"aliases"`
}

// Store 别名库。加载后全量驻留内存，修改即时落盘。
type Store struct {
	path string
	data dbFile
}

// Open 打开默认位置的别名库；文件不存在时视为空库。
func Open() (*Store, error) {
	path, err := platform.DBPath()
	if err != nil {
		return nil, fmt.Errorf("确定存储位置失败: %w", err)
	}
	return openAt(path)
}

// OpenPath 打开指定 JSON 文件（主要用于测试）。
func OpenPath(path string) (*Store, error) {
	return openAt(path)
}

func openAt(path string) (*Store, error) {
	s := &Store{path: path, data: dbFile{Version: dbVersion}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 %s: %w", path, err)
	}
	if len(raw) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("解析 %s 失败（文件可能已损坏，可删除后重建）: %w", path, err)
	}
	if s.data.Version == 0 {
		s.data.Version = dbVersion
	}
	return s, nil
}

// Path 返回存储文件路径。
func (s *Store) Path() string { return s.path }

// List 返回按使用频率排序的全量别名副本。
func (s *Store) List() []*model.Alias {
	out := make([]*model.Alias, len(s.data.Aliases))
	copy(out, s.data.Aliases)
	model.SortByUsage(out)
	return out
}

// Aliases 返回未排序的全量别名（用于唯一性检查等）。
func (s *Store) Aliases() []*model.Alias {
	out := make([]*model.Alias, len(s.data.Aliases))
	copy(out, s.data.Aliases)
	return out
}

// Get 按别名精确查找。
func (s *Store) Get(alias string) (*model.Alias, bool) {
	for _, a := range s.data.Aliases {
		if a.Alias == alias {
			return a, true
		}
	}
	return nil, false
}

// Add 新增别名，重名时报 ErrExists。
func (s *Store) Add(a *model.Alias) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if _, ok := s.Get(a.Alias); ok {
		return fmt.Errorf("%w: %s", ErrExists, a.Alias)
	}
	a.CreatedAt = time.Now()
	s.data.Aliases = append(s.data.Aliases, a)
	return s.Save()
}

// Update 覆盖保存指定别名（别名不变），不存在时报 ErrNotFound。
func (s *Store) Update(a *model.Alias) error {
	return s.Rename(a.Alias, a)
}

// Rename 更新别名并允许改名：old 为原别名，a 为新内容。
func (s *Store) Rename(old string, a *model.Alias) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if old != a.Alias {
		if _, ok := s.Get(a.Alias); ok {
			return fmt.Errorf("%w: %s", ErrExists, a.Alias)
		}
	}
	for i, item := range s.data.Aliases {
		if item.Alias == old {
			s.data.Aliases[i] = a
			return s.Save()
		}
	}
	return fmt.Errorf("%w: %s", ErrNotFound, old)
}

// Remove 删除别名，返回是否确实删除了。
func (s *Store) Remove(alias string) (bool, error) {
	for i, a := range s.data.Aliases {
		if a.Alias == alias {
			s.data.Aliases = append(s.data.Aliases[:i], s.data.Aliases[i+1:]...)
			return true, s.Save()
		}
	}
	return false, nil
}

// RecordUse 记录一次使用（次数 +1、刷新时间）并落盘。
func (s *Store) RecordUse(alias string) error {
	a, ok := s.Get(alias)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, alias)
	}
	a.UsedCount++
	a.LastUsedAt = time.Now()
	return s.Save()
}

// ExportJSON 序列化整个别名库（与 aliases.json 同构），导出文件既可作
// 备份直接放回，也可用 cm import 导入。
func (s *Store) ExportJSON() ([]byte, error) {
	s.data.Version = dbVersion
	return json.MarshalIndent(&s.data, "", "  ")
}

// ImportResult 导入结果分类明细。
type ImportResult struct {
	Added   []string // 新增的别名
	Updated []string // 覆盖更新的别名
	Skipped []string // 与库中重名被跳过的别名
	Invalid []string // 校验失败的条目（"别名: 原因"）
}

// Import 合并导入一批别名：文件内重名后者胜出；与库中重名默认跳过，
// overwrite 为 true 时覆盖。条目保留导入文件中的全部字段（含创建时间与
// 使用统计），创建时间为零时补当前时间；非法条目跳过并记入 Invalid。
// dryRun 为 true 时只计算结果，不改动库也不落盘。
func (s *Store) Import(list []*model.Alias, overwrite, dryRun bool) (*ImportResult, error) {
	res := &ImportResult{}
	uniq := make(map[string]*model.Alias, len(list))
	order := make([]string, 0, len(list))
	for _, a := range list {
		if a == nil {
			continue
		}
		name := strings.TrimSpace(a.Alias)
		if name == "" {
			res.Invalid = append(res.Invalid, "<空别名>: 别名不能为空")
			continue
		}
		a.Alias = name
		if _, seen := uniq[name]; !seen {
			order = append(order, name)
		}
		uniq[name] = a // 文件内重名后者胜出
	}
	// 在副本上合并，dry-run 或中途出错都不污染内存中的库
	pos := make(map[string]int, len(s.data.Aliases)) // 别名 → 在 merged 中的下标
	merged := make([]*model.Alias, len(s.data.Aliases), len(s.data.Aliases)+len(order))
	copy(merged, s.data.Aliases)
	for i, a := range merged {
		pos[a.Alias] = i
	}
	for _, name := range order {
		a := uniq[name]
		if err := a.Validate(); err != nil {
			res.Invalid = append(res.Invalid, name+": "+err.Error())
			continue
		}
		if a.CreatedAt.IsZero() {
			a.CreatedAt = time.Now()
		}
		if i, ok := pos[a.Alias]; ok {
			if !overwrite {
				res.Skipped = append(res.Skipped, a.Alias)
				continue
			}
			merged[i] = a
			res.Updated = append(res.Updated, a.Alias)
			continue
		}
		pos[a.Alias] = len(merged)
		merged = append(merged, a)
		res.Added = append(res.Added, a.Alias)
	}
	if dryRun {
		return res, nil
	}
	s.data.Aliases = merged
	return res, s.Save()
}

// Save 原子写盘：先写同目录临时文件再 rename，避免写一半损坏。
func (s *Store) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建目录 %s: %w", dir, err)
	}
	s.data.Version = dbVersion
	raw, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".aliases-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // rename 成功后此调用为 no-op
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("写入临时文件: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}
