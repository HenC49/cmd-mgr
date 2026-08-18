// Package browse 提供 PATH 命令浏览 TUI：模糊搜索系统中所有可执行命令，
// 选中后可直接进入"以此命令创建别名"的表单。
//
//	输入即过滤 · ↑/↓ 或 ctrl+k/j 移动 · enter 选中并创建别名 · esc 退出
package browse

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"cmd-mgr/internal/discover"
	"cmd-mgr/internal/ui"
)

type visEntry struct {
	e       discover.Entry
	matched []int
}

type browseTui struct {
	all   []discover.Entry
	vis   []visEntry
	query textinput.Model
	cursor, offset int
	w, h  int
	chosen *discover.Entry
}

// Run 启动浏览 TUI，返回用户选中的命令条目；未选中时第二个返回值为 false。
func Run(output io.Writer) (discover.Entry, bool, error) {
	b := newBrowseTui(discover.All())
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if output != nil {
		opts = append(opts, tea.WithOutput(output))
	}
	p := tea.NewProgram(b, opts...)
	out, err := p.Run()
	if err != nil {
		return discover.Entry{}, false, err
	}
	m := out.(browseTui)
	if m.chosen == nil {
		return discover.Entry{}, false, nil
	}
	return *m.chosen, true, nil
}

func newBrowseTui(entries []discover.Entry) browseTui {
	if entries == nil {
		entries = []discover.Entry{}
	}
	q := textinput.New()
	q.Prompt = ""
	q.Placeholder = "输入关键词过滤 PATH 中的命令…"
	q.Focus()
	b := browseTui{all: entries, query: q}
	b.applyFilter()
	return b
}

func (b browseTui) Init() tea.Cmd { return textinput.Blink }

func (b browseTui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		b.w, b.h = msg.Width, msg.Height
		b.query.Width = max(10, b.w-14)
		return b, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return b, tea.Quit
		case tea.KeyEnter:
			if b.cursor >= 0 && b.cursor < len(b.vis) {
				e := b.vis[b.cursor].e
				b.chosen = &e
			}
			return b, tea.Quit
		case tea.KeyUp, tea.KeyCtrlK:
			b.move(-1)
			return b, nil
		case tea.KeyDown, tea.KeyCtrlJ:
			b.move(1)
			return b, nil
		case tea.KeyPgUp:
			b.move(-(b.bodyHeight() - 1))
			return b, nil
		case tea.KeyPgDown:
			b.move(b.bodyHeight() - 1)
			return b, nil
		case tea.KeyHome:
			b.cursor, b.offset = 0, 0
			return b, nil
		case tea.KeyEnd:
			b.cursor = len(b.vis) - 1
			b.fixScroll()
			return b, nil
		}
		// 查询为空时 vim 风格 j/k
		if strings.TrimSpace(b.query.Value()) == "" && len(b.vis) > 0 {
			switch msg.String() {
			case "k":
				b.move(-1)
				return b, nil
			case "j":
				b.move(1)
				return b, nil
			}
		}
		var cmd tea.Cmd
		b.query, cmd = b.query.Update(msg)
		b.applyFilter()
		return b, cmd
	}
	return b, nil
}

func (b *browseTui) move(delta int) {
	if len(b.vis) == 0 {
		return
	}
	b.cursor = clamp(b.cursor+delta, 0, len(b.vis)-1)
	b.fixScroll()
}

func (b *browseTui) fixScroll() {
	h := b.bodyHeight()
	if b.cursor < b.offset {
		b.offset = b.cursor
	}
	if b.cursor >= b.offset+h {
		b.offset = b.cursor - h + 1
	}
	if b.offset < 0 {
		b.offset = 0
	}
}

func (b *browseTui) applyFilter() {
	q := strings.TrimSpace(b.query.Value())
	b.vis = b.vis[:0]
	if q == "" {
		for _, e := range b.all {
			b.vis = append(b.vis, visEntry{e: e})
		}
	} else {
		names := make([]string, len(b.all))
		for i, e := range b.all {
			names[i] = e.Name
		}
		for _, m := range fuzzy.Find(q, names) {
			b.vis = append(b.vis, visEntry{e: b.all[m.Index], matched: m.MatchedIndexes})
		}
	}
	b.cursor = clamp(b.cursor, 0, max(0, len(b.vis)-1))
	b.fixScroll()
}

// ---------- 渲染 ----------

func (b browseTui) View() string {
	if b.w == 0 {
		b.w, b.h = 80, 24 // 拿不到终端尺寸时的兜底
	}
	title := ui.TitleStyle.Render(fmt.Sprintf("cm · 浏览 PATH 命令 (%d)", len(b.all)))
	search := ui.DimStyle.Render("搜索: ") + b.query.View()
	divider := ui.DimStyle.Render(strings.Repeat("─", b.w))
	help := ui.DimStyle.Render(ui.Truncate("输入即过滤 · ↑/↓ 移动 · enter 以此命令创建别名 · esc 退出", b.w))

	bodyH := max(1, b.h-4) // title + search + divider + footer
	var body string
	if len(b.vis) == 0 {
		body = lipgloss.Place(b.w, bodyH, lipgloss.Center, lipgloss.Center,
			ui.DimStyle.Render(fmt.Sprintf("无匹配 %q 的命令", b.query.Value())))
	} else {
		listW := b.w * 3 / 5
		prevW := b.w - listW - 1
		list := b.renderList(listW, bodyH)
		prev := b.renderPreview(prevW, bodyH)
		sep := strings.TrimSuffix(strings.Repeat("│\n", bodyH), "\n")
		body = lipgloss.JoinHorizontal(lipgloss.Top, list,
			ui.DimStyle.Render(sep), prev)
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, search, divider, body, help)
}

func (b browseTui) bodyHeight() int {
	h := b.h
	if h == 0 {
		h = 24
	}
	return max(1, h-4)
}

func (b browseTui) renderList(width, height int) string {
	var rows []string
	end := min(b.offset+height, len(b.vis))
	for i := b.offset; i < end; i++ {
		v := b.vis[i]
		name := v.e.Name
		firstPath := ""
		if len(v.e.Paths) > 0 {
			firstPath = v.e.Paths[0]
		}
		if i == b.cursor {
			row := ui.PadRight(name, 24) + ui.Truncate(firstPath, width-24)
			rows = append(rows, ui.CursorStyle.Render(ui.PadRight(ui.Truncate(row, width), width)))
		} else {
			name = highlight(name, v.matched)
			row := ui.PadRight(name, 24) + ui.Truncate(firstPath, width-24)
			rows = append(rows, ui.PadRight(row, width))
		}
	}
	for len(rows) < height {
		rows = append(rows, strings.Repeat(" ", width))
	}
	if len(b.vis) > height {
		pos := fmt.Sprintf(" %d/%d ", b.cursor+1, len(b.vis))
		rows[len(rows)-1] = overlayRight(rows[len(rows)-1], ui.DimStyle.Render(pos), width)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func overlayRight(row, right string, width int) string {
	row = ui.PadRight(row, width)
	keep := width - ui.Width(right)
	if keep < 0 {
		return right
	}
	return ui.Truncate(row, keep) + right
}

func highlight(name string, matched []int) string {
	base := ui.AliasStyle
	if len(matched) == 0 {
		return base.Render(name)
	}
	hit := make(map[int]bool, len(matched))
	for _, i := range matched {
		hit[i] = true
	}
	var sb strings.Builder
	for i, r := range name {
		if hit[i] {
			sb.WriteString(base.Underline(true).Render(string(r)))
		} else {
			sb.WriteString(base.Render(string(r)))
		}
	}
	return sb.String()
}

func (b browseTui) renderPreview(width, height int) string {
	if width < 16 || b.cursor >= len(b.vis) {
		return ""
	}
	e := b.vis[b.cursor].e
	var lines []string
	lines = append(lines, ui.AliasStyle.Render(e.Name))
	lines = append(lines, ui.DimStyle.Render("位于:"))
	for _, p := range e.Paths {
		lines = append(lines, "  "+ui.CmdStyle.Render(p))
	}
	lines = append(lines, "", ui.DimStyle.Render("按 enter 以此命令创建别名"))
	body := strings.Join(lines, "\n")
	if n := len(lines); n > height {
		body = strings.Join(lines[:height], "\n")
	}
	return body
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
