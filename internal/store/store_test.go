package store

import (
	"errors"
	"os"
	"path/filepath"
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
