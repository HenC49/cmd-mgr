package form

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cmd-mgr/internal/model"
	"cmd-mgr/internal/ui"
)

// resize 构造指定终端尺寸下的表单模型。
func resize(f formTui, w, h int) formTui {
	upd, _ := f.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return upd.(formTui)
}

// TestFormNoBracketWrap 回归：任何终端宽度下，字段行的 "[" 与 "]" 必须在同一行，
// 且所有行都不超过盒子的总宽（边框内），否则说明宽度计算不一致导致折行。
func TestFormNoBracketWrap(t *testing.T) {
	for _, w := range []int{200, 110, 100, 80, 60, 50, 45} {
		f := resize(newForm(nil, map[string]bool{}, false), w, 24)
		view := f.View()

		fieldRows := 0
		for i, line := range strings.Split(view, "\n") {
			// Place 会把整个视图补齐到终端宽，因此行宽不得超过终端宽
			if lw := ui.Width(line); lw > w {
				t.Errorf("w=%d: 第 %d 行宽 %d 超过终端宽 %d: %q", w, i, lw, w, line)
			}
			if strings.Contains(line, "[") {
				fieldRows++
				if !strings.Contains(line, "]") {
					t.Errorf("w=%d: 括号被折行: %q", w, line)
				}
			}
		}
		if fieldRows != fieldCount {
			t.Errorf("w=%d: 应有 %d 个字段行，得到 %d", w, fieldCount, fieldRows)
		}
	}
}

// TestFormPrefilledLongCommand 预填长命令时同样不得折断括号。
func TestFormPrefilledLongCommand(t *testing.T) {
	prefill := &model.Alias{
		Command:     "rsync -avz --delete ./src/ host:/srv/app/ --exclude node_modules --exclude .git",
		Alias:       "dsync",
		Description: "同步源码到服务器",
	}
	f := resize(newForm(prefill, map[string]bool{}, false), 100, 24)
	for i, line := range strings.Split(f.View(), "\n") {
		if strings.Contains(line, "[") && !strings.Contains(line, "]") {
			t.Errorf("第 %d 行括号被折行: %q", i, line)
		}
	}
}
