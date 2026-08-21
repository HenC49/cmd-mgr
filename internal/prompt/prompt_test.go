package prompt

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"cmd-mgr/internal/history"
	"cmd-mgr/internal/model"
	"cmd-mgr/internal/ui"
)

func intp(i int) *int { return &i }

func testCfg() Config {
	return Config{
		Alias:    "sshsrv",
		Template: "ssh {{user}}@{{host}}",
		Params:   []model.Param{{Name: "user", Desc: "登录用户名"}, {Name: "host", Desc: "服务器 IP"}},
		History: []history.Record{
			{
				Alias: "sshsrv", Template: "ssh {{user}}@{{host}}", Command: "ssh root@10.0.0.2",
				Params:   map[string]string{"user": "root", "host": "10.0.0.2"},
				ExitCode: intp(0), StartedAt: time.Now().Add(-2 * time.Hour), DurationMs: 3200,
			},
			{
				Alias: "sshsrv", Template: "ssh {{user}}@{{host}}", Command: "ssh admin@10.0.0.1",
				Params:    map[string]string{"user": "admin", "host": "10.0.0.1"},
				StartedAt: time.Now().Add(-3 * 24 * time.Hour),
			},
		},
	}
}

func resize(t *testing.T, m tui, w, h int) tui {
	t.Helper()
	upd, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return upd.(tui)
}

func press(t *testing.T, m tui, key tea.KeyType) tui {
	t.Helper()
	upd, _ := m.Update(tea.KeyMsg{Type: key})
	return upd.(tui)
}

func pressRune(t *testing.T, m tui, runes string) tui {
	t.Helper()
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(runes)})
	return upd.(tui)
}

// 预填：打开表单时默认带入最近一次的参数值，重复上次执行只需连按 enter。
func TestPrefillFromMostRecent(t *testing.T) {
	m := newTui(testCfg())
	if got := m.inputs[0].Value(); got != "root" {
		t.Errorf("user 预填 = %q, 期望 root", got)
	}
	if got := m.inputs[1].Value(); got != "10.0.0.2" {
		t.Errorf("host 预填 = %q, 期望 10.0.0.2", got)
	}
	if m.histIdx != 0 {
		t.Errorf("应选中最近一条历史，histIdx = %d", m.histIdx)
	}
}

// tab 在历史记录间循环，并把该次全部参数填入输入框。
func TestTabCyclesHistory(t *testing.T) {
	m := newTui(testCfg())
	m = press(t, m, tea.KeyTab)
	if m.histIdx != 1 {
		t.Fatalf("tab 后 histIdx = %d, 期望 1", m.histIdx)
	}
	if m.inputs[0].Value() != "admin" || m.inputs[1].Value() != "10.0.0.1" {
		t.Fatalf("tab 应填入上一条历史参数，得到 %q / %q", m.inputs[0].Value(), m.inputs[1].Value())
	}
	m = press(t, m, tea.KeyTab)
	if m.histIdx != 0 || m.inputs[1].Value() != "10.0.0.2" {
		t.Fatalf("tab 应循环回最近一条，histIdx=%d host=%q", m.histIdx, m.inputs[1].Value())
	}

	// 手动输入后取消历史选中态（值保留）
	m = pressRune(t, m, "x")
	if m.histIdx != -1 {
		t.Errorf("手动输入后 histIdx 应为 -1，实际 %d", m.histIdx)
	}
	if m.inputs[0].Value() != "rootx" {
		t.Errorf("手动输入应保留并追加，得到 %q", m.inputs[0].Value())
	}
}

// enter 逐项推进，最后一项 enter 确认运行；esc 取消。
func TestEnterAndCancel(t *testing.T) {
	m := newTui(testCfg())
	m = press(t, m, tea.KeyEnter)
	if m.focus != 1 {
		t.Fatalf("第一个 enter 应切到下一项，focus = %d", m.focus)
	}
	if m.result.Confirm {
		t.Fatal("非最后一项 enter 不应确认")
	}
	m = press(t, m, tea.KeyEnter)
	if !m.result.Confirm {
		t.Fatal("最后一项 enter 应确认运行")
	}
	if m.result.Values["user"] != "root" || m.result.Values["host"] != "10.0.0.2" {
		t.Fatalf("确认值 = %v", m.result.Values)
	}

	// esc 取消
	m2 := press(t, newTui(testCfg()), tea.KeyEsc)
	if m2.result.Confirm {
		t.Error("esc 应取消")
	}
}

// 单参数命令：enter 直接确认。
func TestSingleParamEnterRuns(t *testing.T) {
	cfg := Config{Alias: "p", Template: "ping {{host}}", Params: []model.Param{{Name: "host"}}}
	m := newTui(cfg)
	m = pressRune(t, m, "1.1.1.1")
	m = press(t, m, tea.KeyEnter)
	if !m.result.Confirm || m.result.Values["host"] != "1.1.1.1" {
		t.Fatalf("单参数 enter 应直接确认: %+v", m.result)
	}
}

// 视图包含模板、实时预览与历史区（含状态与解析后命令）。
func TestViewElements(t *testing.T) {
	m := resize(t, newTui(testCfg()), 100, 30)
	view := m.View()
	for _, want := range []string{
		"cm · 执行 sshsrv",
		"ssh {{user}}@{{host}}", // 模板
		"ssh root@10.0.0.2",     // 实时预览（预填值）
		"ssh admin@10.0.0.1",    // 历史命令
		"10.0.0.2",              // 输入框预填值
		"tab 复制历史参数",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("视图中应包含 %q\n---\n%s", want, view)
		}
	}
}

// 无历史时也有基本布局（无历史区），字段行完整。
func TestViewNoHistory(t *testing.T) {
	cfg := Config{Alias: "p", Template: "ping {{host}}", Params: []model.Param{{Name: "host"}}}
	m := resize(t, newTui(cfg), 80, 24)
	view := m.View()
	if strings.Contains(view, "最近执行") {
		t.Error("无历史不应渲染历史区")
	}
	if !strings.Contains(view, "ping {{host}}") {
		t.Error("应显示命令模板")
	}
}

// 回归：任何宽度下字段行 "[" 与 "]" 必须同行，且行宽不超过终端宽。
func TestNoBracketWrap(t *testing.T) {
	for _, w := range []int{200, 110, 100, 80, 60, 50, 45} {
		m := resize(t, newTui(testCfg()), w, 30)
		view := m.View()
		fieldRows := 0
		for i, line := range strings.Split(view, "\n") {
			if lw := ui.Width(line); lw > w {
				t.Errorf("w=%d: 第 %d 行宽 %d 超过终端宽: %q", w, i, lw, line)
			}
			if strings.Contains(line, "[") {
				fieldRows++
				if !strings.Contains(line, "]") {
					t.Errorf("w=%d: 括号被折行: %q", w, line)
				}
			}
		}
		if fieldRows != 2 {
			t.Errorf("w=%d: 应有 2 个字段行，得到 %d", w, fieldRows)
		}
	}
}

// 内联渲染回归：表单不得用 Place 居中/撑满终端（那会把上方的历史输出
// 挤出屏幕）；宽终端下行宽应远小于终端宽。
func TestInlineRenderNotFullScreen(t *testing.T) {
	m := resize(t, newTui(testCfg()), 160, 40)
	max := 0
	for _, line := range strings.Split(m.View(), "\n") {
		if w := ui.Width(line); w > max {
			max = w
		}
	}
	if max >= 160 {
		t.Errorf("内联表单最大行宽 = %d，不应撑满 160 列终端", max)
	}
	if max > 80 {
		t.Errorf("内联表单最大行宽 = %d，盒宽上限 76+边框应不超过 80", max)
	}
}

// 参数说明展示：配置 {{名称:说明}} 后，说明渲染在对应输入框下方。
func TestParamDescShown(t *testing.T) {
	m := resize(t, newTui(testCfg()), 100, 30) // testCfg 的参数带说明
	view := m.View()
	for _, want := range []string{"└ 登录用户名", "└ 服务器 IP"} {
		if !strings.Contains(view, want) {
			t.Errorf("参数表单应展示说明 %q\n---\n%s", want, view)
		}
	}
	// 无说明的参数不渲染说明行
	cfg := Config{Alias: "p", Template: "ping {{host}}", Params: []model.Param{{Name: "host"}}}
	if view := resize(t, newTui(cfg), 80, 24).View(); strings.Contains(view, "└") {
		t.Errorf("无说明不应渲染说明行:\n%s", view)
	}
}
