// Package form 提供新增/编辑别名的交互表单 TUI：
// 命令（PATH 实时补全与校验）→ 别名 → 描述 → 标签。
//
//	enter      下一项；最后一项（标签）上为保存（不触发补全）
//	↑/↓        切换字段（不触发补全）；shift+tab 上一项
//	tab        在命令项上补全首个建议（如输入 rsyn + tab → rsync），无可补全时切换字段
//	esc/ctrl+c 取消
package form

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cmd-mgr/internal/discover"
	"cmd-mgr/internal/model"
	"cmd-mgr/internal/ui"
)

// Outcome 表单结束方式。
type Outcome int

const (
	OutcomeCancelled Outcome = iota
	OutcomeSaved
)

// Run 启动表单。prefill 为预填值（nil 或部分填充均可），existing 为已占用别名
// （唯一性检查，编辑时外层应排除自身），isEdit 控制标题与统计字段保留；
// output 为 nil 时渲染到 stdout（shell 集成下传 stderr，避免污染捕获输出）。
func Run(prefill *model.Alias, existing []string, isEdit bool, output io.Writer) (*model.Alias, Outcome, error) {
	occupied := make(map[string]bool, len(existing))
	for _, n := range existing {
		occupied[n] = true
	}
	f := newForm(prefill, occupied, isEdit)
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if output != nil {
		opts = append(opts, tea.WithOutput(output))
	}
	p := tea.NewProgram(f, opts...)
	out, err := p.Run()
	if err != nil {
		return nil, OutcomeCancelled, err
	}
	m := out.(formTui)
	return m.saved, m.outcome, nil
}

const (
	fieldCommand = iota
	fieldAlias
	fieldDesc
	fieldTags
	fieldCount
)

var fieldLabels = [fieldCount]string{"命令", "别名", "描述", "标签"}

const fieldLabelW = 6 // 标签列宽（右侧补空格）

// formBoxWidth 表单盒的 lipgloss Width 参数（含左右 padding，不含边框）。
func formBoxWidth(w int) int {
	if w <= 0 {
		w = 80
	}
	return min(76, max(40, w-6))
}

// inputWidthFor 输入框可见宽度：盒内容宽 - 标签 - 括号，再留 4 列余量
// （bubbles textinput 的 placeholder 视图多占 1 列，长内容滚动窗口可宽出 1~2 列）。
func inputWidthFor(termW int) int { return formBoxWidth(termW) - 4 - fieldLabelW - 2 - 4 }

type formTui struct {
	isEdit   bool
	orig     *model.Alias // 编辑前的原值，保存时保留统计字段
	occupied map[string]bool
	inputs   [fieldCount]textinput.Model
	focus    int
	errMsg   string
	w, h     int
	saved    *model.Alias
	outcome  Outcome
}

func newForm(prefill *model.Alias, occupied map[string]bool, isEdit bool) formTui {
	if prefill == nil {
		prefill = &model.Alias{}
	}
	f := formTui{isEdit: isEdit, orig: prefill, occupied: occupied}
	values := [fieldCount]string{prefill.Command, prefill.Alias, prefill.Description, strings.Join(prefill.Tags, ", ")}
	placeholders := [fieldCount]string{
		"如: rsync -avz --delete ./src/ host:/srv/app/",
		"如: dsync",
		"这条命令是干什么的（搜索时也匹配这里）",
		"可选，逗号分隔，如: deploy, rsync",
	}
	for i := 0; i < fieldCount; i++ {
		in := textinput.New()
		in.Prompt = ""
		in.Placeholder = placeholders[i]
		in.Width = inputWidthFor(0) // 先定宽再赋值，保证按宽度计算滚动窗口
		in.SetValue(values[i])
		f.inputs[i] = in
	}
	f.inputs[fieldCommand].Focus()
	return f
}

func (f formTui) Init() tea.Cmd { return textinput.Blink }

func (f formTui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.w, f.h = msg.Width, msg.Height
		iw := inputWidthFor(f.w)
		for i := range f.inputs {
			// 直接改 Width 不会触发 textinput 内部重算滚动窗口，
			// 长命令会整体渲染导致溢出折行；重新 SetValue 强制重算
			val := f.inputs[i].Value()
			f.inputs[i].Width = iw
			f.inputs[i].SetValue(val)
		}
		return f, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			f.outcome = OutcomeCancelled
			return f, tea.Quit

		case tea.KeyTab:
			// 补全仅由 tab 触发：命中建议时消费按键，停留在命令项
			if f.focus == fieldCommand && f.completeCommand() {
				return f, nil
			}
			f.nextField()
			return f, nil
		case tea.KeyDown:
			f.nextField()
			return f, nil
		case tea.KeyUp, tea.KeyShiftTab:
			f.focus = (f.focus + fieldCount - 1) % fieldCount
			f.syncFocus()
			return f, nil

		case tea.KeyEnter:
			if f.focus == fieldTags {
				if err := f.trySave(); err != nil {
					f.errMsg = err.Error()
					return f, nil
				}
				f.outcome = OutcomeSaved
				return f, tea.Quit
			}
			f.nextField()
			return f, nil

		case tea.KeySpace:
			// 空格交给输入框（命令里需要空格），但需先清掉可能的错误提示
			f.errMsg = ""
		}

		var cmd tea.Cmd
		f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
		f.errMsg = ""
		return f, cmd
	}
	return f, nil
}

// nextField 移到下一字段，不做补全——补全只由 tab 显式触发，
// enter/↓ 切换时误改命令首词会静默产生错误命令。
func (f *formTui) nextField() {
	f.focus = (f.focus + 1) % fieldCount
	f.syncFocus()
}

func (f *formTui) syncFocus() {
	for i := range f.inputs {
		if i == f.focus {
			f.inputs[i].Focus()
		} else {
			f.inputs[i].Blur()
		}
	}
}

// completeCommand 把命令首词补全为第一个 PATH 建议；首词已精确存在时返回 false（视为切换字段）。
func (f *formTui) completeCommand() bool {
	cmdStr := strings.TrimSpace(f.inputs[fieldCommand].Value())
	words := strings.Fields(cmdStr)
	if len(words) == 0 {
		return false
	}
	first := words[0]
	if discover.Exists(first) {
		return false
	}
	sugg := discover.Suggestions(first, 1)
	if len(sugg) == 0 {
		return false
	}
	words[0] = sugg[0]
	f.inputs[fieldCommand].SetValue(strings.Join(words, " "))
	return true
}

// trySave 校验并构造别名；出错时返回错误信息展示在表单内。
func (f *formTui) trySave() error {
	a := &model.Alias{
		Alias:       f.inputs[fieldAlias].Value(),
		Command:     f.inputs[fieldCommand].Value(),
		Description: f.inputs[fieldDesc].Value(),
		Tags:        model.ParseTags(f.inputs[fieldTags].Value()),
	}
	if err := a.Validate(); err != nil {
		return err
	}
	if f.occupied[a.Alias] && !(f.isEdit && f.orig != nil && a.Alias == f.orig.Alias) {
		return fmt.Errorf("别名 %s 已存在", a.Alias)
	}
	if f.isEdit && f.orig != nil {
		a.CreatedAt = f.orig.CreatedAt
		a.UsedCount = f.orig.UsedCount
		a.LastUsedAt = f.orig.LastUsedAt
	}
	f.saved = a
	return nil
}

// ---------- 渲染 ----------

func (f formTui) View() string {
	title := "新增别名"
	if f.isEdit && f.orig != nil {
		title = "编辑别名: " + f.orig.Alias
	}

	var lines []string
	lines = append(lines, ui.TitleStyle.Render(title), "")
	for i := 0; i < fieldCount; i++ {
		label := fieldLabels[i]
		if i == fieldTags {
			label = "标签*" // 可选标记
		}
		lines = append(lines, f.renderField(i, label))
		if i == fieldCommand {
			if hint := f.commandHint(); hint != "" {
				// 与输入框内容对齐（标签 6 列 + "[" 1 列 + 空格 1 列）
				lines = append(lines, strings.Repeat(" ", fieldLabelW+2)+hint)
			}
		}
	}
	lines = append(lines, "")
	if f.errMsg != "" {
		lines = append(lines, ui.ErrorStyle.Render("✗ "+f.errMsg))
	} else {
		lines = append(lines, "")
	}
	lines = append(lines, ui.DimStyle.Render("enter 下一项/保存 · ↑/↓ 切换字段 · tab 补全（仅命令项）或下一项 · esc 取消"))

	body := lipgloss.JoinVertical(lipgloss.Left, lines...)
	box := ui.BorderStyle.Padding(1, 2).Width(formBoxWidth(f.w))
	if f.w > 0 && f.h > 0 {
		return lipgloss.Place(f.w, f.h, lipgloss.Center, lipgloss.Center, box.Render(body))
	}
	return box.Render(body)
}

func (f formTui) renderField(i int, label string) string {
	style := ui.DimStyle
	if i == f.focus {
		style = ui.AliasStyle
	}
	return style.Render(ui.PadRight(label, fieldLabelW)) + "[" + f.inputs[i].View() + "]"
}

// commandHint 命令首词的 PATH 校验提示与建议列表。
func (f formTui) commandHint() string {
	words := strings.Fields(strings.TrimSpace(f.inputs[fieldCommand].Value()))
	if len(words) == 0 {
		return ""
	}
	first := words[0]
	var b strings.Builder
	if discover.Exists(first) {
		b.WriteString(ui.OKStyle.Render("✓ " + first + " 在 PATH 中"))
	} else {
		b.WriteString(ui.WarnStyle.Render("⚠ " + first + " 不在 PATH 中（shell 函数如 sdk/nvm 需 shell 集成执行）"))
	}
	sugg := discover.Suggestions(first, 5)
	others := make([]string, 0, len(sugg))
	for _, s := range sugg {
		if s != first {
			others = append(others, s)
		}
	}
	if len(others) > 0 {
		b.WriteString(ui.DimStyle.Render(" · 建议: ") + ui.DimStyle.Render(strings.Join(others, "  ")))
	}
	return b.String()
}
