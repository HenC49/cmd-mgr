// Package prompt 提供执行前的参数填写 TUI：命令含 {{占位符}} 时，
// 逐项输入参数值（预填最近一次的值），实时预览替换后的命令，
// tab 可循环复制历史参数。
//
// 表单采用内联渲染（fzf 风格，不进 altscreen）：上一个非 cm 命令的
// 输出仍然留在终端上方，可对照着复制参数值（鼠标选中复制 → 终端
// 粘贴键填入输入框）。altscreen 会覆盖整个终端且滚回内容无法读回，
// 因此只在光标处渲染表单本体。
//
//	enter      下一项；最后一项上为确认运行
//	↑/↓        切换字段（shift+tab 上一项）
//	tab        循环选择历史记录，把该次的全部参数填入输入框
//	esc/ctrl+c 取消
package prompt

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cmd-mgr/internal/history"
	"cmd-mgr/internal/model"
	"cmd-mgr/internal/ui"
)

// Config 参数表单配置。
type Config struct {
	Alias    string
	Template string           // 含 {{占位符}} 的原始命令
	Params   []model.Param    // 参数（按出现顺序，已按名称去重，可带说明）
	History  []history.Record // 该别名最近记录（新的在前）
}

// Result 参数表单结果。Confirm=false 表示用户取消。
type Result struct {
	Values  map[string]string
	Confirm bool
}

// Run 启动参数表单（内联渲染，保留终端上方内容）。output 为 nil 时渲染到
// stdout（shell 集成下传 stderr，避免污染被捕获的 stdout）。
func Run(cfg Config, output io.Writer) (Result, error) {
	m := newTui(cfg)
	var opts []tea.ProgramOption // 不用 altscreen：表单要保留上下文可见
	if output != nil {
		opts = append(opts, tea.WithOutput(output))
	}
	p := tea.NewProgram(m, opts...)
	out, err := p.Run()
	if err != nil {
		return Result{}, err
	}
	return out.(tui).result, nil
}

// maxHistLines 历史区最多展示条数。
const maxHistLines = 5

type tui struct {
	cfg     Config
	inputs  []textinput.Model
	focus   int
	histIdx int // 选中的历史记录下标（-1 = 未选中，值为手填）；tab 循环
	w, h    int
	result  Result
}

func newTui(cfg Config) tui {
	labelW := paramLabelW(cfg.Params)
	iw := inputWidthFor(0, labelW)
	inputs := make([]textinput.Model, len(cfg.Params))
	for i := range cfg.Params {
		in := textinput.New()
		in.Prompt = ""
		in.Width = iw // 先定宽再赋值，保证按宽度计算滚动窗口
		inputs[i] = in
	}
	t := tui{cfg: cfg, inputs: inputs, histIdx: -1}
	// 预填最近一次的参数值：重复上次执行只需连按 enter
	if len(cfg.History) > 0 {
		t.applyHistory(0)
	}
	if len(inputs) > 0 {
		inputs[0].Focus()
	}
	return t
}
func (t tui) Init() tea.Cmd { return textinput.Blink }

func (t tui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.w, t.h = msg.Width, msg.Height
		iw := inputWidthFor(t.w, paramLabelW(t.cfg.Params))
		for i := range t.inputs {
			// 直接改 Width 不会触发 textinput 内部重算滚动窗口，
			// 长值会整体渲染导致溢出折行；重新 SetValue 强制重算（同 form）
			val := t.inputs[i].Value()
			t.inputs[i].Width = iw
			t.inputs[i].SetValue(val)
		}
		return t, nil

	case tea.KeyMsg:
		if len(t.inputs) == 0 { // 理论上不会出现（无参数不会进入本表单）
			if msg.Type == tea.KeyEnter {
				t.result = Result{Values: map[string]string{}, Confirm: true}
				return t, tea.Quit
			}
			return t, nil
		}
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return t, tea.Quit

		case tea.KeyTab:
			if len(t.cfg.History) > 0 {
				t.applyHistory((t.histIdx + 1) % len(t.cfg.History))
				return t, nil
			}
			t.nextField()
			return t, nil
		case tea.KeyUp, tea.KeyShiftTab:
			t.focus = (t.focus + len(t.inputs) - 1) % len(t.inputs)
			t.syncFocus()
			return t, nil
		case tea.KeyDown:
			t.nextField()
			return t, nil

		case tea.KeyEnter:
			if t.focus == len(t.inputs)-1 {
				t.result = Result{Values: t.values(), Confirm: true}
				return t, tea.Quit
			}
			t.nextField()
			return t, nil

		default:
			t.histIdx = -1 // 手动输入后取消历史选中态（已填入的值保留）
		}
		var cmd tea.Cmd
		t.inputs[t.focus], cmd = t.inputs[t.focus].Update(msg)
		return t, cmd
	}
	return t, nil
}

func (t *tui) nextField() {
	if len(t.inputs) == 0 {
		return
	}
	t.focus = (t.focus + 1) % len(t.inputs)
	t.syncFocus()
}

func (t *tui) syncFocus() {
	for i := range t.inputs {
		if i == t.focus {
			t.inputs[i].Focus()
		} else {
			t.inputs[i].Blur()
		}
	}
}

// applyHistory 选中第 i 条历史并把该次的参数值填入对应输入框；
// 模板变更后历史中缺失的参数保持当前值不动。
func (t *tui) applyHistory(i int) {
	t.histIdx = i
	for j, p := range t.cfg.Params {
		if v, ok := t.cfg.History[i].Params[p.Name]; ok {
			t.inputs[j].SetValue(v)
		}
	}
}

// values 收集当前输入值（去首尾空白，保留内部空格——参数可能是带空格的短语）。
func (t *tui) values() map[string]string {
	v := make(map[string]string, len(t.inputs))
	for i, p := range t.cfg.Params {
		v[p.Name] = strings.TrimSpace(t.inputs[i].Value())
	}
	return v
}

// preview 替换参数后的完整命令，随输入实时变化。
func (t *tui) preview() string {
	return model.RenderCommand(t.cfg.Template, t.values())
}

// ---------- 渲染 ----------

// 盒宽（lipgloss Width 参数，含左右 padding 不含边框），与 form 一致。
func boxWidth(w int) int {
	if w <= 0 {
		w = 80
	}
	return min(76, max(40, w-6))
}

func paramLabelW(params []model.Param) int {
	w := 6
	for _, p := range params {
		if x := ui.Width(p.Name); x > w {
			w = x
		}
	}
	return w + 1
}

// inputWidthFor 输入框可见宽度：盒内容宽 - 标签 - 括号，再留 4 列余量（同 form）。
func inputWidthFor(termW, labelW int) int {
	return boxWidth(termW) - 4 - labelW - 2 - 4
}

func (t tui) View() string {
	if t.w == 0 {
		t.w, t.h = 80, 24
	}
	bw := boxWidth(t.w)
	inner := bw - 4 // Padding(1,2) 下的内容宽

	var lines []string
	lines = append(lines, ui.TitleStyle.Render("cm · 执行 "+t.cfg.Alias), "")
	lines = append(lines, ui.DimStyle.Render(ui.Truncate("$ "+t.cfg.Template, inner)), "")

	labelW := paramLabelW(t.cfg.Params)
	descRows := 0
	for i, p := range t.cfg.Params {
		style := ui.DimStyle
		if i == t.focus {
			style = ui.AliasStyle
		}
		lines = append(lines, style.Render(ui.PadRight(p.Name, labelW))+"["+t.inputs[i].View()+"]")
		if p.Desc != "" { // 参数说明：展示在输入框下方，与输入内容对齐
			descRows++
			lines = append(lines, ui.DimStyle.Render(strings.Repeat(" ", labelW+1)+"└ "+ui.Truncate(p.Desc, inner-labelW-3)))
		}
	}

	lines = append(lines, "", ui.DimStyle.Render("运行命令:"))
	prevBox := ui.BorderStyle.Width(inner - 2).Render(ui.CmdStyle.Render("$ " + ui.Wrap(t.preview(), inner-6)))
	lines = append(lines, prevBox)

	// 历史区按剩余高度收敛展示条数，避免小终端下溢出
	help := "enter 下一项/运行 · ↑↓ 切换 · tab 复制历史参数 · esc 取消"
	if n := t.histCount(inner, help); n > 0 {
		lines = append(lines, "", ui.DimStyle.Render(fmt.Sprintf("最近执行（tab 填入）:")))
		for i := 0; i < n; i++ {
			lines = append(lines, t.histLine(i, inner))
		}
		lines = append(lines, "", ui.DimStyle.Render(help))
	} else {
		lines = append(lines, "", ui.DimStyle.Render(help))
	}

	body := lipgloss.JoinVertical(lipgloss.Left, lines...)
	// 内联渲染：不做 Place 居中/撑满，表单按自身尺寸渲染在光标处，
	// 终端上方的历史输出保持可见
	return ui.BorderStyle.Padding(1, 2).Width(bw).Render(body)
}

// histCount 计算历史区可展示的条数：受历史总量、单页上限与剩余高度三方约束。
func (t tui) histCount(inner int, help string) int {
	n := min(len(t.cfg.History), maxHistLines)
	if n == 0 || t.h <= 0 {
		return n
	}
	descRows := 0
	for _, p := range t.cfg.Params {
		if p.Desc != "" {
			descRows++
		}
	}
	// 无历史区时的行数（含盒 padding 2 行）
	fixed := 7 + len(t.inputs) + descRows + 4 + lipgloss.Height(ui.Wrap(t.preview(), inner-6)) - 1
	avail := t.h - fixed - len(strings.Split(help, "\n")) - 2
	return max(0, min(n, avail))
}

// histLine 渲染一条历史：选中标记 + 相对时间 + 状态/耗时 + 解析后命令。
func (t tui) histLine(i, inner int) string {
	r := t.cfg.History[i]
	marker := "  "
	if i == t.histIdx {
		marker = ui.SelectedStyle.Render("▸ ")
	}
	status := ui.ExitStatus(r.ExitCode)
	if d := ui.DurationStr(r.DurationMs); d != "" {
		status += ui.DimStyle.Render(" " + d)
	}
	head := marker + ui.DimStyle.Render(ui.PadRight(ui.TimeAgo(r.StartedAt), 10)) + " " + ui.PadRight(status, 8) + " "
	return head + ui.CmdStyle.Render(ui.Truncate(r.Command, inner-ui.Width(head)))
}
