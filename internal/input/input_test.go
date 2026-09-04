package input

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func press(t *testing.T, m inputTui, key tea.KeyType) inputTui {
	t.Helper()
	upd, _ := m.Update(tea.KeyMsg{Type: key})
	return upd.(inputTui)
}

func pressRune(t *testing.T, m inputTui, runes string) inputTui {
	t.Helper()
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(runes)})
	return upd.(inputTui)
}

// 输入 → enter 确认；Validate 通过才退出。
func TestRunConfirm(t *testing.T) {
	m := newInputTui(Config{Title: "t", Label: "文件", Default: "/tmp/a.json"})
	m = press(t, m, tea.KeyEnter)
	if !m.result.Confirm || m.result.Value != "/tmp/a.json" {
		t.Fatalf("enter 应确认默认值: %+v", m.result)
	}

	// 手动编辑后取编辑值（去首尾空白）
	m2 := newInputTui(Config{Title: "t", Label: "文件"})
	m2 = pressRune(t, m2, "  x.txt  ")
	m2 = press(t, m2, tea.KeyEnter)
	if m2.result.Value != "x.txt" {
		t.Fatalf("值应 TrimSpace: %q", m2.result.Value)
	}
}

// Validate 失败：enter 不退出，错误展示在表单内；重新输入清掉错误。
func TestValidateErrorShown(t *testing.T) {
	m := newInputTui(Config{Title: "t", Label: "文件", Validate: func(v string) error {
		if v == "" {
			return errors.New("不能为空")
		}
		return nil
	}})
	m = press(t, m, tea.KeyEnter) // 空值 → 校验失败，不退出
	if m.result.Confirm {
		t.Fatal("校验失败不应确认")
	}
	if !strings.Contains(m.View(), "✗ 不能为空") {
		t.Fatalf("表单应展示错误:\n%s", m.View())
	}
	m = pressRune(t, m, "v")
	m = press(t, m, tea.KeyEnter)
	if !m.result.Confirm || m.result.Value != "v" {
		t.Fatalf("修正后应确认: %+v", m.result)
	}
	if strings.Contains(m.View(), "✗") {
		t.Fatal("重新输入后错误应清除")
	}
}

// esc 取消。
func TestCancel(t *testing.T) {
	m := press(t, newInputTui(Config{Title: "t", Label: "文件"}), tea.KeyEsc)
	if m.result.Confirm {
		t.Error("esc 应取消")
	}
}

// 视图包含标题、标签、输入框与按键提示。
func TestViewElements(t *testing.T) {
	upd, _ := newInputTui(Config{Title: "cm · 导出别名", Label: "文件", Default: "~/b.json"}).Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m := upd.(inputTui)
	view := m.View()
	for _, want := range []string{"cm · 导出别名", "文件", "~/b.json", "enter 确认", "esc 取消"} {
		if !strings.Contains(view, want) {
			t.Errorf("视图中应包含 %q\n---\n%s", want, view)
		}
	}
}

// Show：enter 关闭，内容渲染。
func TestShowView(t *testing.T) {
	upd, _ := showTui{title: "导入结果", lines: []string{"新增 2（a、b）", "覆盖 1（c）"}}.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	s := upd.(showTui)
	view := s.View()
	for _, want := range []string{"导入结果", "新增 2（a、b）", "enter 返回"} {
		if !strings.Contains(view, want) {
			t.Errorf("Show 视图应包含 %q\n---\n%s", want, view)
		}
	}
	upd2, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, still := upd2.(showTui); !still {
		t.Fatal("Update 应返回模型")
	}
}
