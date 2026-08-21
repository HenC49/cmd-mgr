package ui

import (
	"strings"
	"testing"
	"time"
)

func TestTruncate(t *testing.T) {
	if got := Truncate("hello", 10); got != "hello" {
		t.Errorf("短字符串不应截断: %q", got)
	}
	if got := Truncate("hello world", 8); got != "hello w…" {
		t.Errorf("Truncate = %q", got)
	}
	// 中文按显示宽度计
	if got := Truncate("同步源码到远程服务器", 9); Width(got) != 9 {
		t.Errorf("中文截断宽度 = %d (%q)", Width(got), got)
	}
}

func TestPadRight(t *testing.T) {
	if got := Width(PadRight("ab", 5)); got != 5 {
		t.Errorf("PadRight 宽度 = %d", got)
	}
	if got := Width(PadRight("你", 4)); got != 4 {
		t.Errorf("中文 PadRight 宽度 = %d", got)
	}
}

func TestWrap(t *testing.T) {
	// 空格断行
	got := Wrap("aaa bbb ccc", 7)
	if got != "aaa bbb\nccc" {
		t.Errorf("Wrap = %q", got)
	}
	// 超长词硬断
	got = Wrap("aaaaaaaaaa", 4)
	if got != "aaaa\naaaa\naa" {
		t.Errorf("Wrap 硬断 = %q", got)
	}
	// 中文无空格按字符硬断
	got = Wrap("同步源码到服务器", 8)
	lines := strings.Split(got, "\n")
	for i, l := range lines {
		if Width(l) > 8 {
			t.Errorf("第 %d 行宽度超限: %q (%d)", i, l, Width(l))
		}
	}
	if len(lines) != 2 { // 8 个中文字符共 16 显示宽，每行 8 → 2 行 × 4 字
		t.Errorf("中文 Wrap 行数 = %d (%q)", len(lines), got)
	}
}

func TestTimeAgo(t *testing.T) {
	if got := TimeAgo(time.Time{}); got != "未使用" {
		t.Errorf("零值 TimeAgo = %q", got)
	}
	if got := TimeAgo(time.Now().Add(-30 * time.Second)); got != "刚刚" {
		t.Errorf("TimeAgo = %q", got)
	}
	if got := TimeAgo(time.Now().Add(-2 * time.Hour)); got != "2 小时前" {
		t.Errorf("TimeAgo = %q", got)
	}
}

func TestDurationStr(t *testing.T) {
	cases := map[int64]string{
		0:     "",
		-5:    "",
		320:   "320ms",
		1400:  "1.4s",
		65000: "1m5s",
	}
	for ms, want := range cases {
		if got := DurationStr(ms); got != want {
			t.Errorf("DurationStr(%d) = %q, 期望 %q", ms, got, want)
		}
	}
}

func TestExitStatus(t *testing.T) {
	if got := ExitStatus(nil); got != DimStyle.Render("·") {
		t.Errorf("未知状态 = %q", got)
	}
	zero := 0
	if got := ExitStatus(&zero); got != OKStyle.Render("✓") {
		t.Errorf("成功状态 = %q", got)
	}
	three := 3
	if got := ExitStatus(&three); got != ErrorStyle.Render("✗3") {
		t.Errorf("失败状态 = %q", got)
	}
}
