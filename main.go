// cm - 命令别名管理器。记住那些记不清的命令。
package main

import (
	"errors"
	"fmt"
	"os"

	"cmd-mgr/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		var ec *cmd.ExitError
		if errors.As(err, &ec) {
			// 被执命令的退出码透传为 cm 的退出码
			os.Exit(ec.Code)
		}
		fmt.Fprintln(os.Stderr, "cm:", err)
		os.Exit(1)
	}
}
