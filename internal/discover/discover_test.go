package discover

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// makePath 构造两个临时目录：binA 与 binB，其中 binA 含 foo(可执行)、bar(不可执行)、
// .hidden(可执行)；binB 含另一个 foo(可执行)。返回 PATH 字符串。
func makePath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	binA := filepath.Join(dir, "binA")
	binB := filepath.Join(dir, "binB")
	for _, d := range []string{binA, binB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path string, mode os.FileMode) {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(binA, "foo"), 0o755)
	write(filepath.Join(binA, "bar"), 0o644)
	write(filepath.Join(binA, ".hidden"), 0o755)
	write(filepath.Join(binB, "foo"), 0o755)
	return binA + string(os.PathListSeparator) + binB
}

func TestScan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix 可执行位判定测试")
	}
	entries := scan(makePath(t))
	if len(entries) != 1 || entries[0].Name != "foo" {
		t.Fatalf("应只发现 foo，得到 %v", entries)
	}
	if len(entries[0].Paths) != 2 {
		t.Fatalf("foo 应出现在两个目录，得到 %v", entries[0].Paths)
	}
}

func TestScanEmptyPath(t *testing.T) {
	if got := scan(""); len(got) != 0 {
		t.Fatalf("空 PATH 应返回空，得到 %d 项", len(got))
	}
}

func TestScanDedupDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix 可执行位判定测试")
	}
	p := makePath(t)
	doubled := p + string(os.PathListSeparator) + p
	entries := scan(doubled)
	for _, e := range entries {
		if e.Name == "foo" && len(e.Paths) != 2 {
			t.Fatalf("PATH 目录去重后 foo 仍应只有两个路径，得到 %v", e.Paths)
		}
	}
}
