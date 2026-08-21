package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func rec(alias, cmd string, code *int) Record {
	c := cmd
	return Record{
		Alias:     alias,
		Template:  c,
		Command:   c,
		ExitCode:  code,
		StartedAt: time.Now(),
	}
}

func intp(i int) *int { return &i }

func TestAddListRecent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	st, err := OpenPath(path)
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	zero := 0
	if err := st.Add(rec("gst", "git status", &zero)); err != nil {
		t.Fatalf("Add: %v", err)
	}
	c42 := 42
	_ = st.Add(rec("dsync", "rsync a b", &c42))
	_ = st.Add(rec("dsync", "rsync a c", nil))

	// List 新的在前
	all := st.List(10)
	if len(all) != 3 || all[0].Command != "rsync a c" || all[2].Command != "git status" {
		t.Fatalf("List 顺序不符: %+v", all)
	}
	// 按别名过滤 + 限量
	ds := st.Recent("dsync", 1)
	if len(ds) != 1 || ds[0].Command != "rsync a c" {
		t.Fatalf("Recent(dsync,1) = %+v", ds)
	}
	if got := st.Recent("none", 5); len(got) != 0 {
		t.Fatalf("不存在的别名应返回空，得到 %d 条", len(got))
	}
	// 退出码还原
	if all[1].ExitCode == nil || *all[1].ExitCode != 42 {
		t.Fatalf("ExitCode 持久化不符: %+v", all[1].ExitCode)
	}
	if all[0].ExitCode != nil {
		t.Fatal("eval 模式的记录 ExitCode 应为 nil")
	}
}

func TestPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	st, _ := OpenPath(path)
	params := map[string]string{"host": "10.0.0.1"}
	c := 0
	err := st.Add(Record{
		Alias: "ssh-srv", Template: "ssh {{host}}", Command: "ssh 10.0.0.1",
		Params: params, ExitCode: &c, StartedAt: time.Now(), DurationMs: 1200,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	st2, err := OpenPath(path)
	if err != nil {
		t.Fatalf("重开: %v", err)
	}
	got := st2.List(10)
	if len(got) != 1 {
		t.Fatalf("应有 1 条记录，得到 %d", len(got))
	}
	r := got[0]
	if r.Alias != "ssh-srv" || r.Template != "ssh {{host}}" || r.Command != "ssh 10.0.0.1" ||
		r.Params["host"] != "10.0.0.1" || r.DurationMs != 1200 || r.ExitCode == nil || *r.ExitCode != 0 {
		t.Fatalf("持久化数据不完整: %+v", r)
	}
}

func TestTrimToMax(t *testing.T) {
	st, _ := OpenPath(filepath.Join(t.TempDir(), "history.json"))
	for i := 0; i < MaxRecords+50; i++ {
		if err := st.Add(rec("a", time.Now().Format("15:04:05.000000000"), nil)); err != nil {
			t.Fatalf("Add #%d: %v", i, err)
		}
	}
	if got := len(st.List(MaxRecords + 100)); got != MaxRecords {
		t.Fatalf("应自动淘汰到 %d 条，得到 %d", MaxRecords, got)
	}
}

func TestNilStoreSafe(t *testing.T) {
	var st *Store
	if got := st.List(5); got != nil {
		t.Error("nil store List 应返回 nil")
	}
	if got := st.Recent("x", 3); got != nil {
		t.Error("nil store Recent 应返回 nil")
	}
	if err := st.Add(rec("x", "y", nil)); err != nil {
		t.Errorf("nil store Add 应为 no-op: %v", err)
	}
}

func TestCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenPath(path); err == nil {
		t.Fatal("损坏文件应报错")
	}
}

func TestAtomicWriteNoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	st, _ := OpenPath(path)
	_ = st.Add(rec("x", "echo", nil))
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("原子写后不应残留临时文件: %s", e.Name())
		}
	}
}
