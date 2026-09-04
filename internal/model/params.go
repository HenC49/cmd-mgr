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
// Secret 为 true 时是密钥引用（{{user@服务}} / {{pass@服务}}），值不手填，
// 执行时从系统密码管理器读取（macOS 钥匙串 / Linux libsecret）。
type Param struct {
	Name   string
	Desc   string
	Secret bool
}

// secretNameRe 密钥引用形式的参数名：user@服务 / pass@服务。
// 选用 名称@服务 而非新括号语法：仍落在 {{}} 语法内（存储与校验零改动），
// 读作"mydb 的账号/密码"；@ 前缀形式也不与 bash 任何语法冲突。
var secretNameRe = regexp.MustCompile(`^(user|pass)@(.+)$`)

// ParseSecretName 解析密钥参数名，返回类别（user/pass）与服务名。
func ParseSecretName(name string) (kind, service string, ok bool) {
	m := secretNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
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
			p := Param{Name: m[1], Desc: strings.TrimSpace(m[2])}
			_, _, p.Secret = ParseSecretName(p.Name)
			params = append(params, p)
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

// SecretMask 预览与历史中密钥值的统一脱敏占位。
const SecretMask = "••••••"

// MaskedValues 返回 values 的副本，其中密钥参数的值替换为 SecretMask
// （值缺失时补上）——用于执行前预览与历史记录：真实密钥只在最终执行的
// 命令里出现，不上屏、不落盘。
func MaskedValues(params []Param, values map[string]string) map[string]string {
	out := make(map[string]string, len(values)+len(params))
	for k, v := range values {
		out[k] = v
	}
	for _, p := range params {
		if p.Secret {
			out[p.Name] = SecretMask
		}
	}
	return out
}
