package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"cmd-mgr/internal/ui"
)

// shellIntegration zsh 与 bash 通用：裸 `cm` 与 `cm run <别名>` 选中/指定的命令
// 由当前 shell eval 执行，支持 cd / export 及 shell 函数（SDKMAN! 的 sdk、nvm 等）；
// 其余子命令原样透传给 cm 二进制。
const shellIntegration = `# cm (cmd-mgr) shell 集成
# 安装: 将下面这行加入 ~/.zshrc（bash 则是 ~/.bashrc），然后重启终端或 source：
#   eval "$(cm init zsh)"
cm() {
  local cm_sel
  if [ $# -eq 0 ]; then
    cm_sel="$(command cm --pick)" || return $?
  elif [ "${1:-}" = "run" ] && [ $# -ge 2 ]; then
    shift
    cm_sel="$(command cm run --print "$@")" || return $?
  else
    command cm "$@"
    return
  fi
  [ -n "$cm_sel" ] || return 0
  eval "$cm_sel"
}
`

var initInstall bool

var initCmd = &cobra.Command{
	Use:   "init <shell>",
	Short: "输出 shell 集成脚本（zsh / bash）",
	Long: `输出 shell 集成脚本。安装方式：

  eval "$(cm init zsh)"   # 加入 ~/.zshrc
  eval "$(cm init bash)"  # 加入 ~/.bashrc

集成后，裸 cm 与 cm run 选中的命令会由当前 shell 执行
（支持 cd / export / sdk / nvm 等）；cm 其余子命令行为不变。

加 --install 可自动检测 shell 并把集成行幂等写入对应 rc 文件
（make install 会自动调用）。`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := ""
		if len(args) > 0 {
			shell = args[0]
		} else {
			shell = detectShell()
			if shell != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "cm: 检测到当前 shell 为 %s\n", shell)
			}
		}
		switch shell {
		case "zsh", "bash":
		case "":
			return fmt.Errorf("未检测到 shell，请显式指定: cm init zsh 或 cm init bash")
		default:
			return fmt.Errorf("暂不支持 %q（当前支持: zsh、bash）", shell)
		}

		if initInstall {
			return installIntegration(shell)
		}
		fmt.Print(shellIntegration)
		return nil
	},
}

func init() {
	initCmd.Flags().BoolVar(&initInstall, "install", false, "自动把集成行写入对应 rc 文件（幂等）")
	rootCmd.AddCommand(initCmd)
}

// detectShell 按 显式参数 > $SHELL > rc 文件存在性 的顺序猜测 shell。
// make 等环境下 $SHELL 不可靠，需要 rc 文件兜底。
func detectShell() string {
	if base := filepath.Base(os.Getenv("SHELL")); base == "zsh" || base == "bash" {
		return base
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); err == nil {
		return "zsh"
	}
	if _, err := os.Stat(filepath.Join(home, ".bashrc")); err == nil {
		return "bash"
	}
	return ""
}

// installIntegration 把集成行幂等写入对应 rc 文件。
func installIntegration(shell string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("无法确定用户目录: %w", err)
	}
	rcName := ".zshrc"
	if shell == "bash" {
		rcName = ".bashrc"
	}
	rcPath := filepath.Join(home, rcName)

	added, err := appendIntegration(rcPath, shell)
	if err != nil {
		return fmt.Errorf("写入 %s 失败: %w", rcPath, err)
	}
	if added {
		fmt.Println(ui.OKStyle.Render("✓ 已添加 shell 集成到 ~/" + rcName))
		fmt.Println("  重启终端或执行 source ~/" + rcName + " 生效")
	} else {
		fmt.Println("✓ ~/" + rcName + " 中已有 cm 集成，跳过")
	}
	return nil
}

// appendIntegration 若 rc 中尚无 cm 集成则追加 eval 行，返回是否实际添加。
func appendIntegration(rcPath, shell string) (bool, error) {
	data, _ := os.ReadFile(rcPath)
	content := string(data)
	// 幂等：已有任何形式的 cm 集成痕迹就不再重复添加
	if strings.Contains(content, "cm init") || strings.Contains(content, "cmd-mgr") {
		return false, nil
	}
	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return false, err
		}
	}
	_, err = fmt.Fprintf(f, "\n# cm (cmd-mgr) shell 集成（cm init --install 添加）\neval \"$(cm init %s)\"\n", shell)
	return err == nil, err
}
