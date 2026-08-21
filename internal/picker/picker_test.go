package picker

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"cmd-mgr/internal/history"
	"cmd-mgr/internal/model"
	"cmd-mgr/internal/ui"
)

func testItems() []*model.Alias {
	return []*model.Alias{
		{Alias: "gst", Command: "git status -sb", Description: "查看 git 状态", UsedCount: 3, LastUsedAt: time.Now()},
		{Alias: "fail1", Command: "exit 42"},
		{Alias: "proj", Command: "cd /tmp/cm-smoke", Description: "进入项目目录"},
		{Alias: "dsync", Command: "rsync -avz --delete ./src/ host:/srv/app/", Description: "同步源码到服务器", Tags: []string{"deploy", "rsync"}},
	}
}

// fakeRecent 固定返回构造好的记录，验证预览面板的历史块。
type fakeRecent struct{ recs map[string][]history.Record }

func (f fakeRecent) Recent(alias string, n int) []history.Record {
	return f.recs[alias]
}

func resize(t *testing.T, m tui, w, h int) tui {
	t.Helper()
	upd, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return upd.(tui)
}

func TestViewLayoutExactWidth(t *testing.T) {
	m := resize(t, newTui(testItems(), nil), 120, 30)
	view := m.View()
	for i, line := range strings.Split(view, "\n") {
		if w := ui.Width(line); w != 120 {
			t.Errorf("第 %d 行宽度 = %d，应为 120: %q", i, w, line)
		}
	}
	// 行数不超过终端高度，避免 altscreen 滚动
	if n := len(strings.Split(view, "\n")); n > 30 {
		t.Errorf("总行数 = %d，超过 30", n)
	}
	// 内容抽查（首项 gst 的预览在右侧面板）
	for _, want := range []string{"cm · 命令别名 (4)", "dsync", "git status -sb", "enter 执行"} {
		if !strings.Contains(view, want) {
			t.Errorf("视图中应包含 %q", want)
		}
	}
}

func TestViewNarrowLayout(t *testing.T) {
	m := resize(t, newTui(testItems(), nil), 60, 20)
	view := m.View()
	for i, line := range strings.Split(view, "\n") {
		if w := ui.Width(line); w != 60 {
			t.Errorf("窄屏第 %d 行宽度 = %d，应为 60: %q", i, w, line)
		}
	}
}

func TestViewEmpty(t *testing.T) {
	m := resize(t, newTui(nil, nil), 100, 24)
	if !strings.Contains(m.View(), "还没有别名") {
		t.Error("空库应显示引导文案")
	}
}

// 预览面板应展示选中别名的最近执行（状态 + 相对时间 + 替换后命令）。
func TestPreviewShowsRecentHistory(t *testing.T) {
	code := 3
	recent := fakeRecent{recs: map[string][]history.Record{
		"dsync": {
			{Alias: "dsync", Command: "rsync -avz ./src/ 10.0.0.2:/srv/app/", ExitCode: nil, StartedAt: time.Now().Add(-time.Hour)},
			{Alias: "dsync", Command: "rsync -avz ./src/ 10.0.0.1:/srv/app/", ExitCode: &code, StartedAt: time.Now().Add(-26 * time.Hour)},
		},
	}}
	m := resize(t, newTui(testItems(), recent), 120, 30)
	// 选中 dsync
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("dsync")})
	view := m.View()
	for _, want := range []string{"最近执行:", "rsync -avz ./src/ 10.0.0.2", "✗3 1 天前"} {
		if !strings.Contains(view, want) {
			t.Errorf("预览面板应包含 %q\n---\n%s", want, view)
		}
	}
}

func TestFilterAndKeys(t *testing.T) {
	m := newTui(testItems(), nil)
	m = resize(t, m, 120, 30)

	// 打字即过滤：输入 rsyn 只剩 dsync
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("rsyn")})
	m = upd.(tui)
	if len(m.vis) != 1 || m.vis[0].a.Alias != "dsync" {
		t.Fatalf("过滤后应只剩 dsync，得到 %d 项", len(m.vis))
	}

	// enter → 执行动作
	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = upd.(tui)
	if m.result.Action != ActionExecute || m.result.Alias.Alias != "dsync" {
		t.Fatalf("enter 应执行 dsync，得到 %+v", m.result)
	}

	// esc → 退出
	m2 := newTui(testItems(), nil)
	upd, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if upd.(tui).result.Action != ActionQuit {
		t.Error("esc 应退出")
	}

	// ctrl+d → 确认态，y → 删除动作
	m3 := resize(t, newTui(testItems(), nil), 120, 30)
	upd, _ = m3.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m3 = upd.(tui)
	if !m3.confirm {
		t.Fatal("ctrl+d 应进入删除确认")
	}
	upd, _ = m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if got := upd.(tui).result.Action; got != ActionDelete {
		t.Errorf("y 应触发删除，得到 %v", got)
	}
	// n 取消确认
	upd, _ = m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if upd.(tui).confirm {
		t.Error("其他键应取消确认")
	}
}
