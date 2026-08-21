package model

import "testing"

func TestExtractParams(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want []Param
	}{
		{"无占位符", "git status -sb", nil},
		{"单个", "ssh {{user}}@1.2.3.4", []Param{{Name: "user"}}},
		{"多个按出现顺序", "rsync -avz {{src}} {{user}}@{{host}}:{{dst}}", []Param{{Name: "src"}, {Name: "user"}, {Name: "host"}, {Name: "dst"}}},
		{"同名去重", "cp {{f}} {{f}}.bak", []Param{{Name: "f"}}},
		{"允许空白填充", "scp {{ file }} host:/tmp", []Param{{Name: "file"}}},
		{"名称可含中文与符号", "kubectl logs {{pod-名.字}}", []Param{{Name: "pod-名.字"}}},
		{"不误伤 shell 变量", "echo ${HOME} $USER", nil},
		{"不误伤单个花括号", "awk '{print $1}' foo.txt", nil},
		{"带说明", "ssh {{user:用户名}}@{{host:服务器 IP}}", []Param{{Name: "user", Desc: "用户名"}, {Name: "host", Desc: "服务器 IP"}}},
		{"说明可含冒号与空格", "curl {{url:格式: http 或 https}}", []Param{{Name: "url", Desc: "格式: http 或 https"}}},
		{"说明为空视为无说明", "ping {{host:}}", []Param{{Name: "host"}}},
		{"同名去重取首个说明", "cp {{f:源文件}} {{f}}.bak", []Param{{Name: "f", Desc: "源文件"}}},
	}
	for _, c := range cases {
		got := ExtractParams(c.cmd)
		if len(got) != len(c.want) {
			t.Errorf("%s: ExtractParams(%q) = %v, 期望 %v", c.name, c.cmd, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: ExtractParams(%q)[%d] = %+v, 期望 %+v", c.name, c.cmd, i, got[i], c.want[i])
			}
		}
	}
}

func TestRenderCommand(t *testing.T) {
	cases := []struct {
		cmd    string
		values map[string]string
		want   string
	}{
		{"ssh {{user}}@{{host}}", map[string]string{"user": "root", "host": "10.0.0.1"}, "ssh root@10.0.0.1"},
		{"ssh {{user:用户名}}@{{host}}", map[string]string{"user": "root", "host": "10.0.0.1"}, "ssh root@10.0.0.1"},
		{"cp {{f}} {{f}}.bak", map[string]string{"f": "a.txt"}, "cp a.txt a.txt.bak"},
		{"echo hi", map[string]string{}, "echo hi"},
		{"echo {{a}} {{ b }}", map[string]string{"a": "x", "b": "y"}, "echo x y"},
		{"curl {{url:地址}}:8080", map[string]string{"url": "h.io"}, "curl h.io:8080"},
	}
	for _, c := range cases {
		if got := RenderCommand(c.cmd, c.values); got != c.want {
			t.Errorf("RenderCommand(%q, %v) = %q, 期望 %q", c.cmd, c.values, got, c.want)
		}
	}
}

func TestValidatePlaceholders(t *testing.T) {
	ok := []string{
		"",
		"ssh {{user}}@{{host}}",
		"ssh {{user:用户名}}@{{host:服务器 IP}}",
		"echo ${HOME}",
		"awk '{print $1}'",
		"cp {{ f }} {{f}}.bak",
		"{{a:}}", // 空说明
	}
	for _, cmd := range ok {
		if err := validatePlaceholders(cmd); err != nil {
			t.Errorf("validatePlaceholders(%q) 不应报错: %v", cmd, err)
		}
	}
	bad := map[string]string{
		"{{}}":          "空名称",
		"{{ }}":         "空白名称",
		"echo {{a}} {{": "未闭合",
		"echo {{a b}}":  "名称含空格",
		"echo {{a{b}}}": "名称含花括号",
		"{{:说明}}":       "名称为空只有说明",
		"{{a:b}c}}":     "说明含花括号",
	}
	for cmd := range bad {
		if err := validatePlaceholders(cmd); err == nil {
			t.Errorf("validatePlaceholders(%q) 应报错", cmd)
		}
	}
}

func TestAliasValidateRejectsBadPlaceholder(t *testing.T) {
	a := &Alias{Alias: "x", Command: "echo {{a}} {{}"}
	if err := a.Validate(); err == nil {
		t.Error("含非法占位符的命令应校验失败")
	}
}
