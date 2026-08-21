// 命令参数占位符：配置时写 {{名称}} 或 {{名称:说明}}，执行前由用户填写
// 参数值替换。选用 {{}} 而非 ${} 是为了不与 shell 变量/位置参数语法冲突——
// 存入的 ${HOME}、$1 等仍由 shell 在运行时展开，cm 不拦截。
package model

import (
	"fmt"
	"regexp"
	"strings"
)

// placeholderRe 合法占位符：{{名称}} 或 {{名称:说明}}。名称非空、不含空格、
// 花括号与冒号（允许中英文、数字、-_. 等）；说明可含空格与冒号，不可含花括号。
var placeholderRe = regexp.MustCompile(`\{\{\s*([^{}\s:]+)\s*(?::\s*([^{}]*?)\s*)?\}\}`)

// Param 一个命令参数：名称 + 可选说明（说明展示在执行时的参数表单中）。
type Param struct {
	Name string
	Desc string
}

// ExtractParams 按出现顺序返回命令中的参数（按名称去重；同名多次出现时
// 以首次出现的说明为准）。
func ExtractParams(command string) []Param {
	matches := placeholderRe.FindAllStringSubmatch(command, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	var params []Param
	for _, m := range matches {
		if !seen[m[1]] {
			seen[m[1]] = true
			params = append(params, Param{Name: m[1], Desc: strings.TrimSpace(m[2])})
		}
	}
	return params
}

// RenderCommand 用填写的参数值替换全部占位符（含说明的整个 {{...}} 一并
// 替换；纯文本替换不做转义——预览中看到的就是实际执行的命令）。
// 缺失的参数替换为空串。
func RenderCommand(command string, values map[string]string) string {
	return placeholderRe.ReplaceAllStringFunc(command, func(m string) string {
		return values[placeholderRe.FindStringSubmatch(m)[1]]
	})
}

// validatePlaceholders 校验命令中所有 {{ 均构成合法占位符，
// 防止 {{a}} {{} 这类笔误被存入库后在执行时才暴露。
func validatePlaceholders(command string) error {
	rest := command
	for {
		i := strings.Index(rest, "{{")
		if i < 0 {
			return nil
		}
		seg := rest[i:]
		loc := placeholderRe.FindStringIndex(seg)
		// 合法占位符必须紧贴当前 {{ 开始（loc[0]==0），否则说明此处是笔误
		if loc == nil || loc[0] != 0 {
			return fmt.Errorf("命令中的 {{ 需构成合法占位符 {{名称}} 或 {{名称:说明}}，如: ssh {{user:用户名}}@{{host}}")
		}
		rest = seg[loc[1]:]
	}
}
