package form

import (
	"os"
	"path/filepath"
	"runtime"
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

// TestFormCompletionOnlyOnTab 回归：命令项的 PATH 补全只允许 tab 触发；
// enter / ↓ 只切换字段，不得改动命令内容，避免误把首词替换成建议。
// 注意：discover 的扫描结果进程内缓存（sync.Once），本测试用 t.Setenv 替换
// PATH 构造确定性场景，必须先于任何渲染含命令内容的测试执行。
func TestFormCompletionOnlyOnTab(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows 下补全入口行为相同，此处仅验证 unix 可执行文件识别")
	}
	dir := t.TempDir()
	name := "zzcmformcmd"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	newFormWithCmd := func(cmd string) formTui {
		f := newForm(nil, map[string]bool{}, false)
		f.inputs[fieldCommand].SetValue(cmd)
		return f
	}
	press := func(f formTui, key tea.KeyType) formTui {
		upd, _ := f.Update(tea.KeyMsg{Type: key})
		return upd.(formTui)
	}

	input := name[:len(name)-2] + " -avz" // 首词 zzcmformc 是 zzcmformcmd 的前缀且非精确匹配

	for _, key := range []tea.KeyType{tea.KeyEnter, tea.KeyDown} {
		f := press(newFormWithCmd(input), key)
		if got := f.inputs[fieldCommand].Value(); got != input {
			t.Errorf("%v 不应触发补全，命令被改为 %q", key, got)
		}
		if f.focus != fieldAlias {
			t.Errorf("%v 后焦点应在别名项，实际在 %d", key, f.focus)
		}
	}

	f := press(newFormWithCmd(input), tea.KeyTab)
	if got := f.inputs[fieldCommand].Value(); got != name+" -avz" {
		t.Errorf("tab 应补全首词为 %q，实际 %q", name+" -avz", got)
	}
	if f.focus != fieldCommand {
		t.Errorf("tab 补全后应停留在命令项，实际在 %d", f.focus)
	}

	// 首词已是精确命令时 tab 无可补全，切换字段
	f = press(newFormWithCmd(name+" -avz"), tea.KeyTab)
	if f.focus != fieldAlias {
		t.Errorf("首词已存在时 tab 应切换到别名项，实际在 %d", f.focus)
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

// TestFormParamHint 命令项下方常驻参数提示：无占位符时展示写法说明，
// 有占位符时列出识别到的参数名（去重）。
func TestFormParamHint(t *testing.T) {
	f := newForm(nil, map[string]bool{}, false)
	// 空命令：展示占位符写法（可发现性——用户不知道命令能带参数）
	view := f.View()
	if !strings.Contains(view, "{{名称:说明}}") || !strings.Contains(view, "支持参数") {
		t.Errorf("无占位符时应展示参数写法说明:\n%s", view)
	}

	// 识别到占位符：切换为参数列表（带内联说明）
	f.inputs[fieldCommand].SetValue("ssh {{user:用户名}}@{{host}}")
	view = f.View()
	if !strings.Contains(view, "参数: user（用户名）, host") {
		t.Errorf("应提示识别到的参数及说明:\n%s", view)
	}
	if strings.Contains(view, "支持参数") {
		t.Error("已有占位符时不应再显示写法说明")
	}

	// 同名占位符去重
	f.inputs[fieldCommand].SetValue("cp {{f}} {{f}}.bak")
	if !strings.Contains(f.View(), "参数: f") {
		t.Error("重复占位符应去重展示")
	}
}
