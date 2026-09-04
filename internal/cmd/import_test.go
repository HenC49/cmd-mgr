package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("无家目录环境")
	}
	if got := expandHome("~/x.json"); got != filepath.Join(home, "x.json") {
		t.Errorf("expandHome(~/x.json) = %q", got)
	}
	if got := expandHome("~"); got != home {
		t.Errorf("expandHome(~) = %q", got)
	}
	if got := expandHome("/abs/path.json"); got != "/abs/path.json" {
		t.Errorf("绝对路径应原样: %q", got)
	}
	if got := expandHome("rel.json"); got != "rel.json" {
		t.Errorf("相对路径应原样: %q", got)
	}
}

func TestParseImportAliases(t *testing.T) {
	// cm export 的完整形状
	wrapped := []byte(`{"version":1,"aliases":[{"alias":"gst","command":"git status"}]}`)
	list, err := parseImportAliases(wrapped)
	if err != nil || len(list) != 1 || list[0].Alias != "gst" {
		t.Fatalf("wrapped = %v, %v", list, err)
	}

	// 纯数组（手写/裁剪过的文件）
	arr := []byte(`[{"alias":"b","command":"echo b"}]`)
	list, err = parseImportAliases(arr)
	if err != nil || len(list) != 1 || list[0].Alias != "b" {
		t.Fatalf("array = %v, %v", list, err)
	}

	// 空库导出（aliases 为空数组）也应可解析
	empty := []byte(`{"version":1,"aliases":[]}`)
	if list, err = parseImportAliases(empty); err != nil || len(list) != 0 {
		t.Fatalf("empty = %v, %v", list, err)
	}

	// 无关 JSON 与坏 JSON 报错
	for _, bad := range []string{`{"foo":1}`, `not json`, ``} {
		if _, err := parseImportAliases([]byte(bad)); err == nil {
			t.Errorf("%q 应报错", bad)
		}
	}
}

func TestNameList(t *testing.T) {
	if got := nameList([]string{"a", "b"}); got != "（a、b）" {
		t.Errorf("nameList = %q", got)
	}
	if got := nameList([]string{"a"}); got != "（a）" {
		t.Errorf("nameList = %q", got)
	}
	// 超过 8 个折叠为 "等 N 个"
	names := make([]string, 10)
	for i := range names {
		names[i] = string(rune('a' + i))
	}
	if got := nameList(names); !strings.Contains(got, "等 10 个") || strings.Contains(got, "j、") {
		t.Errorf("超过上限应折叠: %q", got)
	}
}
