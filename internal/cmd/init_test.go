package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendIntegration(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")

	// 首次添加
	added, err := appendIntegration(rc, "zsh")
	if err != nil || !added {
		t.Fatalf("首次添加失败: added=%v err=%v", added, err)
	}
	data, _ := os.ReadFile(rc)
	if !strings.Contains(string(data), `eval "$(cm init zsh)"`) {
		t.Fatalf("rc 中应有集成行:\n%s", data)
	}

	// 幂等：重复执行不再添加
	added, err = appendIntegration(rc, "zsh")
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Fatal("已存在集成时应跳过")
	}
	data2, _ := os.ReadFile(rc)
	if len(data2) != len(data) {
		t.Fatalf("重复执行不应改变文件: %d != %d", len(data2), len(data))
	}
}

func TestAppendIntegrationExistingManualMarker(t *testing.T) {
	// 用户手工贴过脚本（含 cmd-mgr 标记）也算已安装
	rc := filepath.Join(t.TempDir(), ".zshrc")
	os.WriteFile(rc, []byte("# cm (cmd-mgr) shell 集成\ncm() { command cm \"$@\"; }\n"), 0o644)
	added, err := appendIntegration(rc, "zsh")
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Fatal("已有手工集成时应跳过")
	}
}

func TestAppendIntegrationNoTrailingNewline(t *testing.T) {
	// 原文件末尾无换行时先补换行，避免拼到上一行
	rc := filepath.Join(t.TempDir(), ".bashrc")
	os.WriteFile(rc, []byte("export FOO=1"), 0o644)
	added, err := appendIntegration(rc, "bash")
	if err != nil || !added {
		t.Fatalf("添加失败: %v", err)
	}
	data, _ := os.ReadFile(rc)
	content := string(data)
	if !strings.Contains(content, "FOO=1\n") {
		t.Fatalf("FOO=1 后应有换行:\n%s", content)
	}
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "export FOO=1#") || strings.HasPrefix(line, "export FOO=1# cm") {
			t.Fatalf("集成内容拼到了上一行:\n%s", content)
		}
	}
}

func TestDetectShellFallback(t *testing.T) {
	// $SHELL 可用时优先用
	t.Setenv("SHELL", "/bin/zsh")
	if got := detectShell(); got != "zsh" {
		t.Errorf("detectShell = %q, want zsh", got)
	}
	// $SHELL 不可用时回落到 rc 文件存在性（依赖真实 HOME，仅在无 shell 环境下有意义）
	t.Setenv("SHELL", "/bin/sh")
	got := detectShell()
	if got != "zsh" && got != "bash" && got != "" {
		t.Errorf("detectShell 兜底结果异常: %q", got)
	}
}
