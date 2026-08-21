// Package ui 提供各 TUI 与 CLI 输出共用的样式和文本工具。
// 样式颜色由 lipgloss 自动检测：输出非终端时自动降级为纯文本。
package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// 共享 lipgloss 样式。
var (
	TitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	AliasStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	DescStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	DimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	CmdStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	TagStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("135"))
	OKStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("71"))
	WarnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	ErrorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	CursorStyle   = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("61")).Foreground(lipgloss.Color("231"))
	SelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	BorderStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
)

// IsTTY 判断文件描述符是否连接到终端。
func IsTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// AdoptStderrColor 把 lipgloss 颜色检测改按 stderr 进行。
// 适用场景：shell 集成下 cm 以 --pick 运行，stdout 被 $(...) 捕获而非终端，
// 默认渲染器按 stdout 检测会把所有样式降级为纯文本（选中高亮消失）；
// TUI 实际渲染到 stderr，此时应按 stderr 检测。其余情况不做任何改变。
// 注意：直接替换默认渲染器对已创建的样式无效（样式在创建时绑定渲染器实例），
// 因此这里是在默认渲染器上原地修改 profile。
func AdoptStderrColor() {
	if !IsTTY(os.Stdout) && IsTTY(os.Stderr) {
		lipgloss.DefaultRenderer().SetColorProfile(lipgloss.NewRenderer(os.Stderr).ColorProfile())
	}
}

// Width 返回字符串的显示宽度（中文等宽字符按 2 计）。
func Width(s string) int { return lipgloss.Width(s) }

// Truncate 按显示宽度截断字符串，超出部分以 … 结尾。
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if w+rw > max-1 {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String() + "…"
}

// PadRight 在字符串右侧补空格至指定显示宽度。
func PadRight(s string, w int) string {
	d := lipgloss.Width(s)
	if d >= w {
		return s
	}
	return s + strings.Repeat(" ", w-d)
}

// PadLeft 在字符串左侧补空格至指定显示宽度（右对齐）。
func PadLeft(s string, w int) string {
	d := lipgloss.Width(s)
	if d >= w {
		return s
	}
	return strings.Repeat(" ", w-d) + s
}

// Wrap 按显示宽度换行：优先在空格处断行，超长词（如长命令、中文无空格）按字符硬断。
func Wrap(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		var line strings.Builder
		appendWord := func(w string) {
			if line.Len() == 0 {
				line.WriteString(w)
			} else if lipgloss.Width(line.String())+1+lipgloss.Width(w) <= max {
				line.WriteString(" ")
				line.WriteString(w)
			} else {
				lines = append(lines, line.String())
				line.Reset()
				line.WriteString(w)
			}
		}
		for _, word := range strings.Split(para, " ") {
			// 单个词超宽时先硬断
			for lipgloss.Width(word) > max {
				lines = append(lines, hardCut(word, max))
				word = word[len(cutBytes(word, max)):]
			}
			appendWord(word)
		}
		lines = append(lines, line.String())
	}
	return strings.Join(lines, "\n")
}

// hardCut 取字符串开头不超过 max 显示宽度的部分。
func hardCut(s string, max int) string { return cutBytes(s, max) }

func cutBytes(s string, max int) string {
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if w+rw > max {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String()
}

// TimeAgo 返回人类可读的相对时间。
func TimeAgo(t time.Time) string {
	if t.IsZero() {
		return "未使用"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}

// DurationStr 把毫秒数渲染为简短时长（如 320ms、1.4s、2m5s）；非正值返回空串。
func DurationStr(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case ms <= 0:
		return ""
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}

// ExitStatus 渲染一次执行的状态：✓ 成功、✗ 退出码、· 结果未知（shell eval 模式）。
func ExitStatus(code *int) string {
	switch {
	case code == nil:
		return DimStyle.Render("·")
	case *code == 0:
		return OKStyle.Render("✓")
	default:
		return ErrorStyle.Render(fmt.Sprintf("✗%d", *code))
	}
}
