// Package input 提供轻量的单字段输入表单与只读结果展示 TUI（内联渲染，
// fzf 风格不进 altscreen）——供主选择器里的导出/导入等流程复用：
// picker 退出后终端已还原，表单在光标处渲染小框。
//
//	enter 确认（Validate 通过才退出，错误展示在表单内）
//	esc / ctrl+c 取消
package input

import (
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cmd-mgr/internal/ui"
)

// Config 单字段输入表单配置。
type Config struct {
	Title       string
	Label       string             // 输入框左侧标签
	Default     string             // 预填值
	Placeholder string             // 空值时的提示
	Validate    func(string) error // enter 时校验；返回错误展示在表单内，不退出
}

// Run 启动输入表单，返回 (值, 是否确认, 错误)。确认前值已 TrimSpace。
func Run(cfg Config, output io.Writer) (string, bool, error) {
	m := newInputTui(cfg)
	var opts []tea.ProgramOption // 内联渲染（同 prompt），保留上下文可见
	if output != nil {
		opts = append(opts, tea.WithOutput(output))
	}
	p := tea.NewProgram(m, opts...)
	out, err := p.Run()
	if err != nil {
		return "", false, err
	}
	r := out.(inputTui).result
	return r.Value, r.Confirm, nil
}

type result struct {
	Value   string
	Confirm bool
}

type inputTui struct {
	cfg    Config
	in     textinput.Model
	errMsg string
	w      int
	result result
}

func newInputTui(cfg Config) inputTui {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = cfg.Placeholder
	in.Width = inputWidthFor(0, labelW(cfg.Label))
	in.SetValue(cfg.Default)
	in.Focus()
	return inputTui{cfg: cfg, in: in}
}

func (t inputTui) Init() tea.Cmd { return textinput.Blink }

func (t inputTui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.w = msg.Width
		// 重新 SetValue 强制 textinput 重算滚动窗口（同 form/prompt）
		val := t.in.Value()
		t.in.Width = inputWidthFor(t.w, labelW(t.cfg.Label))
		t.in.SetValue(val)
		return t, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return t, tea.Quit
		case tea.KeyEnter:
			v := strings.TrimSpace(t.in.Value())
			if t.cfg.Validate != nil {
				if err := t.cfg.Validate(v); err != nil {
					t.errMsg = err.Error()
					return t, nil
				}
			}
			t.result = result{Value: v, Confirm: true}
			return t, tea.Quit
		}
		t.errMsg = "" // 任意编辑清掉错误提示
		var cmd tea.Cmd
		t.in, cmd = t.in.Update(msg)
		return t, cmd
	}
	return t, nil
}

// Show 展示只读结果框，enter/esc 关闭（用于流程结束的反馈——
// picker 重进 altscreen 后普通输出会被切走，结果框保证用户看得见）。
func Show(title string, lines []string, output io.Writer) error {
	m := showTui{title: title, lines: lines}
	var opts []tea.ProgramOption
	if output != nil {
		opts = append(opts, tea.WithOutput(output))
	}
	p := tea.NewProgram(m, opts...)
	_, err := p.Run()
	return err
}

type showTui struct {
	title string
	lines []string
	w     int
}

func (t showTui) Init() tea.Cmd { return nil }

func (t showTui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.Type {
		case tea.KeyEnter, tea.KeyEsc, tea.KeyCtrlC, tea.KeySpace:
			return t, tea.Quit
		}
	}
	if s, ok := msg.(tea.WindowSizeMsg); ok {
		t.w = s.Width
	}
	return t, nil
}

func (t showTui) View() string {
	w := t.w
	if w == 0 {
		w = 80
	}
	bw := boxWidth(w)
	inner := bw - 4
	var lines []string
	lines = append(lines, ui.TitleStyle.Render(t.title), "")
	for _, l := range t.lines {
		lines = append(lines, ui.Wrap(l, inner))
	}
	lines = append(lines, "", ui.DimStyle.Render("enter 返回"))
	body := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return ui.BorderStyle.Padding(1, 2).Width(bw).Render(body)
}

// ---------- 渲染尺寸（与 form/prompt 同一套约定） ----------

// boxWidth 表单盒的 lipgloss Width 参数（含左右 padding，不含边框）。
func boxWidth(w int) int {
	if w <= 0 {
		w = 80
	}
	return min(76, max(40, w-6))
}

// labelW 标签列宽（右侧补空格，最小 6 与 form 一致）。
func labelW(label string) int { return max(6, ui.Width(label)+1) }

// inputWidthFor 输入框可见宽度：盒内容宽 - 标签 - 括号，留 4 列余量。
func inputWidthFor(termW, lw int) int { return boxWidth(termW) - 4 - lw - 2 - 4 }

func (t inputTui) View() string {
	w := t.w
	if w == 0 {
		w = 80
	}
	bw := boxWidth(w)
	inner := bw - 4

	var lines []string
	lines = append(lines, ui.TitleStyle.Render(t.cfg.Title), "")
	lw := labelW(t.cfg.Label)
	lines = append(lines, ui.AliasStyle.Render(ui.PadRight(t.cfg.Label, lw))+"["+t.in.View()+"]")
	lines = append(lines, "")
	if t.errMsg != "" {
		lines = append(lines, ui.ErrorStyle.Render("✗ "+ui.Truncate(t.errMsg, inner-2)))
	} else {
		lines = append(lines, "")
	}
	lines = append(lines, ui.DimStyle.Render("enter 确认 · esc 取消"))

	body := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return ui.BorderStyle.Padding(1, 2).Width(bw).Render(body)
}
