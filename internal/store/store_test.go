package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"cmd-mgr/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aliases.json")
	st, err := OpenPath(path)
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	return st
}

func TestAddGetRemove(t *testing.T) {
	st := newTestStore(t)
	a := &model.Alias{Alias: "dsync", Command: "rsync -avz ./ h:/s", Description: "同步"}
	if err := st.Add(a); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if a.CreatedAt.IsZero() {
		t.Error("Add 应设置 CreatedAt")
	}
	if got, ok := st.Get("dsync"); !ok || got.Command != "rsync -avz ./ h:/s" {
		t.Fatalf("Get(dsync) = %v, %v", got, ok)
	}

	// 重名拒绝
	err := st.Add(&model.Alias{Alias: "dsync", Command: "x"})
	if !errors.Is(err, ErrExists) {
		t.Fatalf("重名 Add 应返回 ErrExists，得到 %v", err)
	}

	// 删除
	if _, err := st.Remove("dsync"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := st.Get("dsync"); ok {
		t.Error("删除后不应再查到")
	}
	if _, err := st.Remove("dsync"); err != nil {
		t.Fatalf("重复删除应静默返回: %v", err)
	}
}

func TestPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aliases.json")
	st, err := OpenPath(path)
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	if err := st.Add(&model.Alias{Alias: "a1", Command: "echo 1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := st.RecordUse("a1"); err != nil {
		t.Fatalf("RecordUse: %v", err)
	}

	// 重新打开验证持久化与统计
	st2, err := OpenPath(path)
	if err != nil {
		t.Fatalf("重开: %v", err)
	}
	a, ok := st2.Get("a1")
	if !ok || a.UsedCount != 1 || a.LastUsedAt.IsZero() {
		t.Fatalf("持久化数据不完整: %+v", a)
	}
}

func TestRenameKeepsOthers(t *testing.T) {
	st := newTestStore(t)
	_ = st.Add(&model.Alias{Alias: "a", Command: "echo a"})
	_ = st.Add(&model.Alias{Alias: "b", Command: "echo b"})

	renamed := &model.Alias{Alias: "c", Command: "echo c", CreatedAt: time.Now()}
	if err := st.Rename("a", renamed); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, ok := st.Get("a"); ok {
		t.Error("改名后旧别名不应存在")
	}
	if _, ok := st.Get("c"); !ok {
		t.Error("新别名应存在")
	}
	if _, ok := st.Get("b"); !ok {
		t.Error("改名不应影响其他别名")
	}
	// 改成已存在的名字应拒绝
	if err := st.Rename("c", &model.Alias{Alias: "b", Command: "x"}); !errors.Is(err, ErrExists) {
		t.Fatalf("改成重名应报 ErrExists，得到 %v", err)
	}
}

func TestCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aliases.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenPath(path); err == nil {
		t.Fatal("损坏文件应报错")
	}
}

func TestAtomicWriteNoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aliases.json")
	st, _ := OpenPath(path)
	if err := st.Add(&model.Alias{Alias: "x", Command: "echo x"}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("原子写后不应残留临时文件: %s", e.Name())
		}
	}
}

func TestImportMerge(t *testing.T) {
	st := newTestStore(t)
	_ = st.Add(&model.Alias{Alias: "gst", Command: "git status -sb"})
	_ = st.RecordUse("gst")

	list := []*model.Alias{
		{Alias: "gst", Command: "git status --from-import"}, // 与本地重名
		{Alias: "dsync", Command: "rsync -avz ./ {{host}}:/srv/", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Alias: "bad", Command: "echo {{}"},      // 非法占位符
		nil,                                      // null 条目
		{Alias: "  spaced  ", Command: "echo s"}, // 名称被修剪
	}
	res, err := st.Import(list, false, false)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(res.Added) != 2 || !slices.Contains(res.Added, "dsync") || !slices.Contains(res.Added, "spaced") {
		t.Fatalf("应新增 dsync 与 spaced, got %v", res.Added)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "gst" {
		t.Errorf("gst 应跳过, got %v", res.Skipped)
	}
	if len(res.Invalid) != 1 {
		t.Errorf("非法条目应记 1 条, got %v", res.Invalid)
	}
	// 跳过的保留本地版本
	if a, _ := st.Get("gst"); a.Command != "git status -sb" || a.UsedCount != 1 {
		t.Errorf("跳过不应改动本地版本: %+v", a)
	}
	// 保留导入文件的创建时间；零值补当前时间
	if a, _ := st.Get("dsync"); !a.CreatedAt.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("导入应保留文件中的创建时间: %+v", a)
	}
	if a, _ := st.Get("spaced"); a.Alias != "spaced" || a.CreatedAt.IsZero() {
		t.Errorf("别名应修剪、零创建时间应补当前: %+v", a)
	}
}

func TestImportOverwrite(t *testing.T) {
	st := newTestStore(t)
	_ = st.Add(&model.Alias{Alias: "gst", Command: "local version"})

	res, err := st.Import([]*model.Alias{{Alias: "gst", Command: "file version"}}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 1 || len(res.Skipped) != 0 {
		t.Fatalf("应覆盖 1 条: %+v", res)
	}
	if a, _ := st.Get("gst"); a.Command != "file version" {
		t.Errorf("覆盖后应为文件版本, got %q", a.Command)
	}

	// 重新打开验证落盘
	st2, _ := OpenPath(st.Path())
	if a, _ := st2.Get("gst"); a.Command != "file version" {
		t.Errorf("覆盖应已落盘, got %q", a.Command)
	}
}

func TestImportDedupInFileAndDryRun(t *testing.T) {
	st := newTestStore(t)
	list := []*model.Alias{
		{Alias: "dup", Command: "echo first"},
		{Alias: "dup", Command: "echo second"}, // 文件内重名后者胜出
	}
	res, err := st.Import(list, false, true) // dry-run
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Added) != 1 || res.Added[0] != "dup" {
		t.Fatalf("文件内重名应合并为 1 条: %v", res.Added)
	}
	if a, ok := st.Get("dup"); ok {
		t.Fatalf("dry-run 不应落盘: %+v", a)
	}
	// 真正导入后取后者
	if _, err := st.Import(list, false, false); err != nil {
		t.Fatal(err)
	}
	if a, _ := st.Get("dup"); a.Command != "echo second" {
		t.Errorf("文件内重名应取后者, got %q", a.Command)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aliases.json")
	st, _ := OpenPath(path)
	_ = st.Add(&model.Alias{Alias: "a", Command: "echo a", Tags: []string{"t1"}})
	_ = st.Add(&model.Alias{Alias: "b", Command: "mysql -u {{user@db}} -p{{pass@db}}"})

	raw, err := st.ExportJSON()
	if err != nil {
		t.Fatal(err)
	}

	// 导出到新库再比对
	st2, err := OpenPath(filepath.Join(t.TempDir(), "aliases2.json"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := parseRoundTrip(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st2.Import(list, false, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b"} {
		x, _ := st.Get(name)
		y, _ := st2.Get(name)
		if x.Command != y.Command || x.CreatedAt.Format(time.RFC3339Nano) != y.CreatedAt.Format(time.RFC3339Nano) {
			t.Errorf("%s 往返不一致: %+v vs %+v", name, x, y)
		}
	}
}

// parseRoundTrip 测试侧按 {"aliases":[..]} 形状解析（与 cmd.parseImportAliases 对应）。
func parseRoundTrip(data []byte) ([]*model.Alias, error) {
	var f struct {
		Aliases []*model.Alias `json:"aliases"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return f.Aliases, nil
}
