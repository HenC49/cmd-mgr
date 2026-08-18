// Package runner 负责执行选中的命令：通过用户 shell 直接执行，stdio 直通。
package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"cmd-mgr/internal/platform"
)

// Run 用用户 shell 执行命令，stdin/stdout/stderr 直通当前终端，
// 返回被执命令的退出码。
func Run(command string) int {
	sh := platform.DefaultShell()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command(sh, "-NoLogo", "-Command", command)
	} else {
		cmd = exec.Command(sh, "-c", command)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	fmt.Fprintf(os.Stderr, "cm: 执行失败: %v\n", err)
	return 1
}
