package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"cmd-mgr/internal/model"
	"cmd-mgr/internal/store"
	"cmd-mgr/internal/ui"
)

var (
	rmForce       bool
	rmInteractive bool
)

var rmCmd = &cobra.Command{
	Use:   "rm <别名>...",
	Short: "删除别名（-i 进入多选删除）",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := openStore()
		if err != nil {
			return err
		}
		if rmInteractive {
			return runRmInteractive(st)
		}
		if len(args) == 0 {
			return fmt.Errorf("请指定要删除的别名，或用 cm rm -i 多选删除")
		}
		return runRmDirect(st, args)
	},
}

func init() {
	rmCmd.Flags().BoolVarP(&rmForce, "force", "f", false, "跳过确认")
	rmCmd.Flags().BoolVarP(&rmInteractive, "interactive", "i", false, "TUI 多选删除")
	rootCmd.AddCommand(rmCmd)
}

func runRmDirect(st *store.Store, names []string) error {
	reader := bufio.NewReader(os.Stdin)
	for _, name := range names {
		a, ok := st.Get(name)
		if !ok {
			return notFoundError(st.List(), name)
		}
		if !rmForce {
			if !ui.IsTTY(os.Stdin) {
				return fmt.Errorf("非交互环境删除需加 -f")
			}
			fmt.Printf("删除 %s（%s）？[y/N] ", ui.AliasStyle.Render(a.Alias), ui.Truncate(a.Command, 40))
			line, _ := reader.ReadString('\n')
			if ans := strings.ToLower(strings.TrimSpace(line)); ans != "y" && ans != "yes" {
				fmt.Println("已跳过 " + a.Alias)
				continue
			}
		}
		if _, err := st.Remove(name); err != nil {
			return err
		}
		fmt.Println(ui.OKStyle.Render("✓ 已删除 " + a.Alias))
	}
	return nil
}

func runRmInteractive(st *store.Store) error {
	if err := requireTTY(); err != nil {
		return err
	}
	items := st.List()
	if len(items) == 0 {
		fmt.Println("暂无别名")
		return nil
	}
	names, err := runMultiSelect(items)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	for _, name := range names {
		if _, err := st.Remove(name); err != nil {
			return err
		}
	}
	fmt.Println(ui.OKStyle.Render(fmt.Sprintf("✓ 已删除 %d 个别名: %s", len(names), strings.Join(names, ", "))))
	return nil
}

// ---------- 多选删除 TUI ----------

type multiSelectTui struct {
	items   []*model.Alias
	checked map[string]bool
	cursor  int
	offset  int
	w, h    int
}

// runMultiSelect 启动多选 TUI，返回勾选的别名列表（esc 取消时为空）。
func runMultiSelect(items []*model.Alias) ([]string, error) {
	m := multiSelectTui{items: items, checked: map[string]bool{}}
	p := tea.NewProgram(m, tea.WithAltScreen())
	out, err := p.Run()
	if err != nil {
		return nil, err
	}
	final := out.(multiSelectTui)
	names := make([]string, 0, len(final.checked))
	for _, a := range items {
		if final.checked[a.Alias] {
			names = append(names, a.Alias)
		}
	}
	return names, nil
}

func (t multiSelectTui) Init() tea.Cmd { return nil }

func (t multiSelectTui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.w, t.h = msg.Width, msg.Height
		return t, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			t.checked = map[string]bool{}
			return t, tea.Quit
		case tea.KeyUp, tea.KeyCtrlK:
			if t.cursor > 0 {
				t.cursor--
				t.fixScroll()
			}
			return t, nil
		case tea.KeyDown, tea.KeyCtrlJ:
			if t.cursor < len(t.items)-1 {
				t.cursor++
				t.fixScroll()
			}
			return t, nil
		case tea.KeySpace:
			if t.cursor < len(t.items) {
				name := t.items[t.cursor].Alias
				if t.checked[name] {
					delete(t.checked, name)
				} else {
					t.checked[name] = true
				}
			}
			return t, nil
		case tea.KeyEnter:
			return t, tea.Quit
		}
	}
	return t, nil
}

func (t *multiSelectTui) fixScroll() {
	h := t.height()
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+h {
		t.offset = t.cursor - h + 1
	}
}

func (t multiSelectTui) height() int { return max(1, t.h-4) }

func (t multiSelectTui) View() string {
	if t.w == 0 {
		t.w, t.h = 80, 24 // 拿不到终端尺寸时的兜底
	}
	title := ui.TitleStyle.Render(fmt.Sprintf("cm · 删除别名（已勾选 %d）", len(t.checked)))
	divider := ui.DimStyle.Render(strings.Repeat("─", t.w))
	help := ui.DimStyle.Render("space 勾选 · ↑/↓ 移动 · enter 删除勾选项 · esc 取消")

	h := t.height()
	var rows []string
	end := min(t.offset+h, len(t.items))
	for i := t.offset; i < end; i++ {
		a := t.items[i]
		mark := " "
		if t.checked[a.Alias] {
			mark = "x"
		}
		prefix := "[" + mark + "] "
		row := prefix + ui.PadRight(a.Alias, 16) + ui.Truncate(a.Description, t.w-ui.Width(prefix)-16-2)
		row = ui.Truncate(row, t.w)
		if i == t.cursor {
			rows = append(rows, ui.CursorStyle.Render(ui.PadRight(row, t.w)))
		} else {
			rows = append(rows, ui.PadRight(row, t.w))
		}
	}
	for len(rows) < h {
		rows = append(rows, strings.Repeat(" ", t.w))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return lipgloss.JoinVertical(lipgloss.Left, title, divider, body, divider, help)
}
