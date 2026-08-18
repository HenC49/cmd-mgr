// Package store 负责别名库的 JSON 持久化（原子写）与 CRUD。
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
