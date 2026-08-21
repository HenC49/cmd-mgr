// Package platform 收敛各平台差异（macOS / Linux / Windows 预留）：
// 配置目录、默认 shell。
package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

// ConfigDir 返回配置目录。优先级：$CM_HOME > 各平台用户配置目录。
//
//	macOS:   ~/Library/Application Support/cmd-mgr
//	Linux:   ~/.config/cmd-mgr
//	Windows: %AppData%\cmd-mgr
func ConfigDir() (string, error) {
	if dir := os.Getenv("CM_HOME"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "cmd-mgr"), nil
}

// DBPath 返回别名库 JSON 文件路径。
func DBPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "aliases.json"), nil
}

// HistoryPath 返回执行历史 JSON 文件路径。
func HistoryPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.json"), nil
}

// DefaultShell 返回执行命令所用 shell：优先 $SHELL，其次按平台兜底。
func DefaultShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	switch runtime.GOOS {
	case "windows":
		return "powershell.exe"
	case "darwin":
		return "/bin/zsh"
	default:
		return "/bin/bash"
	}
}

// ShellName 返回当前 shell 的短名（如 zsh/bash），用于 cm init 默认值。
func ShellName() string {
	s := os.Getenv("SHELL")
	if s == "" {
		return ""
	}
	return filepath.Base(s)
}
