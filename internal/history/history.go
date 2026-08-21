// Package history 记录别名执行历史：替换参数后的实际命令、参数值、
// 退出码与耗时，供 TUI 展示与"复制前序命令参数"使用。
//
// 历史是附属功能：打开或写入失败不应阻塞命令执行，调用方拿到的
// error 只用于提示；nil *Store 的所有方法均为安全 no-op。
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"cmd-mgr/internal/platform"
)

// MaxRecords 历史条数上限，超出自动淘汰最旧记录。
const MaxRecords = 500

const dbVersion = 1

// Record 一次别名执行的记录。ExitCode 为 nil 表示命令交由当前 shell
// eval 执行（shell 集成模式），cm 无法得知结果。
type Record struct {
	Alias      string            `json:"alias"`
	Template   string            `json:"template"`            // 含 {{占位符}} 的原始命令
	Command    string            `json:"command"`             // 替换参数后实际执行的命令
	Params     map[string]string `json:"params,omitempty"`    // 参数名 → 填写值
	ExitCode   *int              `json:"exit_code,omitempty"` // nil = 结果未知（shell eval）
	StartedAt  time.Time         `json:"started_at"`
	DurationMs int64             `json:"duration_ms,omitempty"`
}

type dbFile struct {
	Version int      `json:"version"`
	Records []Record `json:"records"`
}

// Store 执行历史。加载后全量驻留内存，追加即落盘。
type Store struct {
	path    string
	records []Record // 按时间升序
}

// Open 打开默认位置的历史文件；文件不存在时视为空历史。
func Open() (*Store, error) {
	path, err := platform.HistoryPath()
	if err != nil {
		return nil, fmt.Errorf("确定历史存储位置失败: %w", err)
	}
	return OpenPath(path)
}

// OpenPath 打开指定 JSON 文件（主要用于测试）。
func OpenPath(path string) (*Store, error) {
	s := &Store{path: path}
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
	var f dbFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("解析 %s 失败（文件可能已损坏，可删除后重建）: %w", path, err)
	}
	if f.Version == 0 {
		f.Version = dbVersion
	}
	s.records = f.Records
	return s, nil
}

// Path 返回存储文件路径。
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// List 返回最近 n 条记录（新的在前）。
func (s *Store) List(n int) []Record {
	if s == nil || n <= 0 || len(s.records) == 0 {
		return nil
	}
	out := make([]Record, 0, min(n, len(s.records)))
	for i := len(s.records) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, s.records[i])
	}
	return out
}

// Recent 返回指定别名最近 n 条记录（新的在前）。
func (s *Store) Recent(alias string, n int) []Record {
	if s == nil || n <= 0 {
		return nil
	}
	var out []Record
	for i := len(s.records) - 1; i >= 0 && len(out) < n; i-- {
		if s.records[i].Alias == alias {
			out = append(out, s.records[i])
		}
	}
	return out
}

// Add 追加一条记录并落盘（超出上限时淘汰最旧）。
func (s *Store) Add(r Record) error {
	if s == nil {
		return nil
	}
	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now()
	}
	s.records = append(s.records, r)
	if over := len(s.records) - MaxRecords; over > 0 {
		s.records = append([]Record(nil), s.records[over:]...)
	}
	return s.Save()
}

// Save 原子写盘：先写同目录临时文件再 rename（与别名库同一套模式）。
func (s *Store) Save() error {
	if s == nil {
		return nil
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建目录 %s: %w", dir, err)
	}
	raw, err := json.MarshalIndent(&dbFile{Version: dbVersion, Records: s.records}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".history-*.tmp")
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
