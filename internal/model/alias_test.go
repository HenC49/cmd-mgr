package model

import (
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		alias   string
		command string
		wantErr bool
	}{
		{"正常", "dsync", "rsync -avz ./ host:/srv", false},
		{"别名带空格", "bad name", "echo hi", true},
		{"别名为空", "  ", "echo hi", true},
		{"命令为空", "x", "  ", true},
		{"别名带管道符", "a|b", "echo hi", true},
		{"中文别名", "同步", "echo hi", false},
	}
	for _, c := range cases {
		a := &Alias{Alias: c.alias, Command: c.command}
		err := a.Validate()
		if (err != nil) != c.wantErr {
			t.Errorf("%s: Validate() err = %v, wantErr = %v", c.name, err, c.wantErr)
		}
	}
}

func TestParseTags(t *testing.T) {
	got := ParseTags("deploy, rsync 、同步 ,  deploy")
	want := []string{"deploy", "rsync", "同步"}
	if len(got) != len(want) {
		t.Fatalf("ParseTags() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseTags() = %v, want %v", got, want)
		}
	}
	if ParseTags("  ") != nil {
		t.Errorf("ParseTags(空) 应返回 nil")
	}
}

func TestSortByUsage(t *testing.T) {
	now := time.Now()
	items := []*Alias{
		{Alias: "old", UsedCount: 1, CreatedAt: now.Add(-48 * time.Hour), LastUsedAt: now.Add(-48 * time.Hour)},
		{Alias: "hot", UsedCount: 9, CreatedAt: now.Add(-24 * time.Hour), LastUsedAt: now.Add(-1 * time.Hour)},
		{Alias: "recent", UsedCount: 1, CreatedAt: now.Add(-24 * time.Hour), LastUsedAt: now},
	}
	SortByUsage(items)
	if items[0].Alias != "hot" {
		t.Errorf("使用最多的应排第一，得到 %s", items[0].Alias)
	}
	if items[1].Alias != "recent" {
		t.Errorf("次数相同时最近使用的应靠前，得到 %s", items[1].Alias)
	}
}
