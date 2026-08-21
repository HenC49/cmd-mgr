// Package picker 提供 cm 的主 TUI：即时模糊过滤的别名列表 + 预览面板。
//
// 交互约定（避免与"打字即过滤"冲突，动作键全部使用 ctrl 组合键）：
//
//	输入任意字符   过滤（匹配 别名/命令/描述/标签）
//	↑/↓ ctrl+k/j  移动光标（查询为空时 j/k 也可）
//	enter          执行选中项
//	ctrl+n         新增（返回 ActionAdd 由外层打开表单）
//	ctrl+e         编辑选中项
//	ctrl+d         删除选中项（y 确认）
//	esc / ctrl+c   退出
package picker

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"cmd-mgr/internal/history"
	"cmd-mgr/internal/model"
	"cmd-mgr/internal/ui"
)

// Action picker 结束后外层需要执行的动作。
type Action int

const (
	ActionQuit    Action = iota // 用户退出
	ActionExecute               // 执行选中别名
	ActionAdd                   // 打开新增表单
	ActionEdit                  // 打开编辑表单
	ActionDelete                // 删除选中别名
)

// Result picker 的返回结果。
type Result struct {
	Action Action
	Alias  *model.Alias // Execute/Edit/Delete 的目标
}

// RecentProvider 提供别名最近执行记录（预览面板展示），可为 nil。
type RecentProvider interface {
	Recent(alias string, n int) []history.Record
}

type visItem struct {
	a       *model.Alias
	matched []int // 别名中命中的字符下标（用于高亮）
}

type tui struct {
	items  []*model.Alias
	vis    []visItem
	query  textinput.Model
	cursor int
	offset int // 列表滚动窗口起点
	w, h   int
	recent RecentProvider // 执行历史（预览面板），可为 nil

	confirm bool // 删除确认中
	result  Result
}

// Run 启动选择 TUI。items 需已按使用频率排序；output 为 nil 时渲染到 stdout
// （--pick 模式传入 stderr，把 stdout 留给选中结果）；recent 提供执行历史，可为 nil。
func Run(items []*model.Alias, output io.Writer, recent RecentProvider) (Result, error) {
	t := newTui(items, recent)
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if output != nil {
		opts = append(opts, tea.WithOutput(output))
	}
	p := tea.NewProgram(t, opts...)
	out, err := p.Run()
	if err != nil {
		return Result{}, err
	}
	return out.(tui).result, nil
}

func newTui(items []*model.Alias, recent RecentProvider) tui {
	q := textinput.New()
	q.Prompt = ""
	q.Placeholder = "输入关键词过滤（别名 / 命令 / 描述 / 标签）…"
	q.Focus()
	t := tui{items: items, query: q, recent: recent}
	t.applyFilter()
	return t
}

func (t tui) Init() tea.Cmd { return textinput.Blink }

func (t tui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.w, t.h = msg.Width, msg.Height
		t.query.Width = max(10, t.w-14)
		return t, nil

	case tea.KeyMsg:
		if t.confirm {
			if msg.String() == "y" || msg.String() == "Y" {
				t.result = Result{Action: ActionDelete, Alias: t.current()}
				return t, tea.Quit
			}
			t.confirm = false
			return t, nil
		}

		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			t.result = Result{Action: ActionQuit}
			return t, tea.Quit

		case tea.KeyEnter:
			if cur := t.current(); cur != nil {
				t.result = Result{Action: ActionExecute, Alias: cur}
				return t, tea.Quit
			}

		case tea.KeyUp, tea.KeyCtrlK:
			t.move(-1)
			return t, nil
		case tea.KeyDown, tea.KeyCtrlJ:
			t.move(1)
			return t, nil
		case tea.KeyPgUp:
			t.move(-(t.listHeight() - 1))
			return t, nil
		case tea.KeyPgDown:
			t.move(t.listHeight() - 1)
			return t, nil
		case tea.KeyHome:
			t.cursor, t.offset = 0, 0
			return t, nil
		case tea.KeyEnd:
			t.cursor = len(t.vis) - 1
			t.fixScroll()
			return t, nil

		case tea.KeyCtrlN:
			t.result = Result{Action: ActionAdd}
			return t, tea.Quit
		case tea.KeyCtrlE:
			if cur := t.current(); cur != nil {
				t.result = Result{Action: ActionEdit, Alias: cur}
				return t, tea.Quit
			}
		case tea.KeyCtrlD:
			if cur := t.current(); cur != nil {
				t.confirm = true
			}
			return t, nil
		}

		// 查询为空时支持 vim 风格 j/k 移动
		if s := msg.String(); strings.TrimSpace(t.query.Value()) == "" && len(t.vis) > 0 {
			if s == "k" {
				t.move(-1)
				return t, nil
			}
			if s == "j" {
				t.move(1)
				return t, nil
			}
		}

		var cmd tea.Cmd
		t.query, cmd = t.query.Update(msg)
		t.applyFilter()
		return t, cmd
	}
	return t, nil
}

func (t *tui) move(delta int) {
	if len(t.vis) == 0 {
		return
	}
	t.cursor = clamp(t.cursor+delta, 0, len(t.vis)-1)
	t.fixScroll()
}

func (t *tui) fixScroll() {
	h := t.listHeight()
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+h {
		t.offset = t.cursor - h + 1
	}
	if t.offset < 0 {
		t.offset = 0
	}
}

func (t *tui) current() *model.Alias {
	if t.cursor < 0 || t.cursor >= len(t.vis) {
		return nil
	}
	return t.vis[t.cursor].a
}

// applyFilter 依据查询词重建可见列表，并保持光标在范围内。
func (t *tui) applyFilter() {
	q := strings.TrimSpace(t.query.Value())
	t.vis = t.vis[:0]
	if q == "" {
		for _, a := range t.items {
			t.vis = append(t.vis, visItem{a: a})
		}
	} else {
		texts := make([]string, len(t.items))
		for i, a := range t.items {
			texts[i] = a.SearchText()
		}
		for _, m := range fuzzy.Find(q, texts) {
			vi := visItem{a: t.items[m.Index]}
			if fm := fuzzy.Find(q, []string{vi.a.Alias}); len(fm) > 0 {
				vi.matched = fm[0].MatchedIndexes
			}
			t.vis = append(t.vis, vi)
		}
	}
	t.cursor = clamp(t.cursor, 0, max(0, len(t.vis)-1))
	t.fixScroll()
}

// ---------- 渲染 ----------

func (t tui) View() string {
	if t.w == 0 {
		// 某些环境下拿不到终端尺寸（如尺寸为 0 的 pty），用兜底尺寸渲染
		t.w, t.h = 80, 24
	}

	title := ui.TitleStyle.Render(fmt.Sprintf("cm · 命令别名 (%d)", len(t.items)))
	search := ui.DimStyle.Render("搜索: ") + t.query.View()
	divider := ui.DimStyle.Render(strings.Repeat("─", t.w))
	footer := t.footer()

	mainH := max(1, t.h-5) // title + search + divider + divider + footer
	var main string
	switch {
	case len(t.items) == 0:
		main = lipgloss.Place(t.w, mainH, lipgloss.Center, lipgloss.Center,
			ui.DimStyle.Render("还没有别名\n\n按 ctrl+n 添加第一条\n或退出后运行 cm add"))
	case len(t.vis) == 0:
		main = lipgloss.Place(t.w, mainH, lipgloss.Center, lipgloss.Center,
			ui.DimStyle.Render(fmt.Sprintf("无匹配 %q 的别名", t.query.Value())))
	case t.w >= 80:
		listW := t.w * 3 / 5
		previewW := t.w - listW - 1
		list := t.renderList(listW, mainH)
		preview := t.renderPreview(t.current(), previewW, mainH)
		sep := strings.TrimSuffix(strings.Repeat("│\n", mainH), "\n")
		main = lipgloss.JoinHorizontal(lipgloss.Top,
			list,
			ui.DimStyle.Render(sep),
			preview,
		)
	default:
		listH := max(2, mainH*3/5)
		list := t.renderList(t.w, listH)
		preview := t.renderPreview(t.current(), t.w, mainH-listH-1)
		main = lipgloss.JoinVertical(lipgloss.Left, list,
			ui.DimStyle.Render(strings.Repeat("─", min(t.w, ui.Width(firstLine(list))))),
			preview)
	}

	return lipgloss.JoinVertical(lipgloss.Left, title, search, divider, main, divider, footer)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func (t tui) footer() string {
	if t.confirm {
		if cur := t.current(); cur != nil {
			return ui.ErrorStyle.Render(fmt.Sprintf("确认删除 %q？y 确认 / 其他键取消", cur.Alias))
		}
	}
	help := "↑/↓ 移动 · 输入即过滤 · enter 执行 · ctrl+n 新增 · ctrl+e 编辑 · ctrl+d 删除 · esc 退出"
	return ui.DimStyle.Render(ui.Truncate(help, t.w))
}

func (t tui) listHeight() int {
	h := t.h
	if h == 0 {
		h = 24
	}
	h = max(1, h-5)
	if t.w >= 80 {
		return h
	}
	return max(2, h*3/5)
}

// renderList 渲染左栏列表（含滚动窗口与光标高亮）。
func (t tui) renderList(width, height int) string {
	aliasW := 8
	for _, v := range t.vis { // 取可见项的最长别名宽（限定范围避免大列表全扫）
		if w := ui.Width(v.a.Alias); w > aliasW {
			aliasW = w
		}
	}
	aliasW = clamp(aliasW+2, 8, 24)

	var rows []string
	end := min(t.offset+height, len(t.vis))
	for i := t.offset; i < end; i++ {
		rows = append(rows, t.renderRow(t.vis[i], i == t.cursor, width, aliasW))
	}
	// 用空行填满高度，保证布局稳定
	for len(rows) < height {
		rows = append(rows, strings.Repeat(" ", width))
	}
	if len(t.vis) > height { // 滚动指示
		pos := fmt.Sprintf(" %d/%d ", t.cursor+1, len(t.vis))
		row := rows[len(rows)-1]
		rows[len(rows)-1] = overlayRight(row, ui.DimStyle.Render(pos), width)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// overlayRight 将 right 覆盖到按显示宽度补齐的行末尾。
func overlayRight(row, right string, width int) string {
	row = ui.PadRight(row, width)
	keep := width - ui.Width(right)
	if keep < 0 {
		return right
	}
	return ui.Truncate(row, keep) + right
}

func (t tui) renderRow(v visItem, selected bool, width, aliasW int) string {
	var alias string
	if selected {
		// 选中行整行反色，别名不再单独着色，避免嵌套样式冲突
		alias = v.a.Alias
	} else {
		alias = highlightAlias(v.a.Alias, v.matched)
	}
	desc := ui.Truncate(v.a.Description, width-aliasW-2)
	row := ui.PadRight(alias, aliasW) + desc
	row = ui.Truncate(row, width)
	if selected {
		return ui.CursorStyle.Render(ui.PadRight(row, width))
	}
	return ui.PadRight(row, width)
}

// highlightAlias 高亮别名中被模糊命中的字符。
func highlightAlias(name string, matched []int) string {
	base := ui.AliasStyle
	if len(matched) == 0 {
		return base.Render(name)
	}
	hit := make(map[int]bool, len(matched))
	for _, i := range matched {
		hit[i] = true
	}
	var b strings.Builder
	for i, r := range name {
		if hit[i] {
			b.WriteString(base.Underline(true).Render(string(r)))
		} else {
			b.WriteString(base.Render(string(r)))
		}
	}
	return b.String()
}

// renderPreview 渲染右栏（宽屏）或底部（窄屏）的详情预览。
func (t tui) renderPreview(a *model.Alias, width, height int) string {
	if a == nil || width < 16 {
		return strings.Repeat("\n", max(0, height-1))
	}
	inner := width - 2

	var blocks []string
	blocks = append(blocks, ui.AliasStyle.Render(a.Alias))
	if a.Description != "" {
		blocks = append(blocks, ui.DescStyle.Render(ui.Wrap(a.Description, inner)))
	}
	cmdBox := ui.BorderStyle.Width(inner).Render(ui.CmdStyle.Render("$ " + ui.Wrap(a.Command, inner-4)))
	blocks = append(blocks, cmdBox)
	if len(a.Tags) > 0 {
		blocks = append(blocks, ui.TagStyle.Render("#"+strings.Join(a.Tags, "  #")))
	}
	blocks = append(blocks, ui.DimStyle.Render(fmt.Sprintf("使用 %d 次 · %s", a.UsedCount, ui.TimeAgo(a.LastUsedAt))))
	if hist := t.renderRecent(a, inner); hist != "" {
		blocks = append(blocks, hist)
	}

	body := lipgloss.JoinVertical(lipgloss.Left, blocks...)
	// 按行截断到预览区高度，避免撑破布局
	lines := strings.Split(body, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

// renderRecent 渲染预览面板中的"最近执行"块（最多 3 条），
// 供查看上次执行的实际命令与结果、复制参数；无历史时返回空串。
func (t tui) renderRecent(a *model.Alias, inner int) string {
	if t.recent == nil {
		return ""
	}
	recs := t.recent.Recent(a.Alias, 3)
	if len(recs) == 0 {
		return ""
	}
	lines := []string{ui.DimStyle.Render("最近执行:")}
	for _, r := range recs {
		head := ui.ExitStatus(r.ExitCode) + " " + ui.DimStyle.Render(ui.PadRight(ui.TimeAgo(r.StartedAt), 9)) + " "
		lines = append(lines, head+ui.Truncate(r.Command, inner-ui.Width(head)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
