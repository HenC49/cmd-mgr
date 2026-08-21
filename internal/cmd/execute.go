// 执行链路的共享逻辑：执行前解析 {{占位符}} 参数，执行后记录历史。
package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"cmd-mgr/internal/history"
	"cmd-mgr/internal/model"
	"cmd-mgr/internal/prompt"
	"cmd-mgr/internal/ui"
)

// resolved 待执行的命令（占位符已替换完成）。
type resolved struct {
	command string            // 替换参数后的命令
	params  map[string]string // 填写的参数值（无占位符时为 nil）
}

// openHistory 打开执行历史；历史是附属功能，失败不阻塞执行（返回 nil）。
func openHistory() *history.Store {
	hist, err := history.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cm: 打开执行历史失败（不影响执行）: %v\n", err)
		return nil
	}
	return hist
}

// resolveCommand 处理占位符：无占位符直接返回原命令；有则打开参数表单
// （预填最近一次的值，tab 可复制历史参数）。confirmed=false 表示用户取消。
func resolveCommand(a *model.Alias, hist *history.Store, output io.Writer) (r resolved, confirmed bool, err error) {
	params := model.ExtractParams(a.Command)
	if len(params) == 0 {
		return resolved{command: a.Command}, true, nil
	}
	if !ui.IsTTY(os.Stdin) {
		names := make([]string, len(params))
		for i, p := range params {
			names[i] = p.Name
		}
		return resolved{}, false, fmt.Errorf("命令 %s 含参数（%s），需交互式终端填写；历史参数可用 cm history %s 查看",
			a.Alias, strings.Join(names, ", "), a.Alias)
	}
	res, err := prompt.Run(prompt.Config{
		Alias:    a.Alias,
		Template: a.Command,
		Params:   params,
		History:  hist.Recent(a.Alias, 5),
	}, output)
	if err != nil {
		return resolved{}, false, err
	}
	if !res.Confirm {
		return resolved{}, false, nil
	}
	return resolved{command: model.RenderCommand(a.Command, res.Values), params: res.Values}, true, nil
}

// recordHistory 记录一次执行。code 为 nil 表示 shell 集成模式下由
// 当前 shell eval，cm 无法得知结果。
func recordHistory(hist *history.Store, a *model.Alias, r resolved, start time.Time, code *int) {
	if hist == nil {
		return
	}
	err := hist.Add(history.Record{
		Alias:      a.Alias,
		Template:   a.Command,
		Command:    r.command,
		Params:     r.params,
		ExitCode:   code,
		StartedAt:  start,
		DurationMs: time.Since(start).Milliseconds(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "cm: 记录执行历史失败: %v\n", err)
	}
}
