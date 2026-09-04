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
		{"密钥引用", "mysql -u {{user@mydb}} -p{{pass@mydb}}", []Param{{Name: "user@mydb", Secret: true}, {Name: "pass@mydb", Secret: true}}},
		{"密钥引用可带说明", "PGPASSWORD={{pass@mydb:数据库密码}} psql", []Param{{Name: "pass@mydb", Desc: "数据库密码", Secret: true}}},
		{"普通参数名含 @ 不误判", "mail {{addr@example.com}}", []Param{{Name: "addr@example.com"}}},
		{"密钥服务名去重", "{{pass@db}} {{pass@db}}", []Param{{Name: "pass@db", Secret: true}}},
		{"user 与 pass 同服务是两个参数", "curl -u {{user@api}}:{{pass@api}}", []Param{{Name: "user@api", Secret: true}, {Name: "pass@api", Secret: true}}},
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

func TestParseSecretName(t *testing.T) {
	cases := []struct {
		name      string
		kind, svc string
		ok        bool
	}{
		{"pass@mydb", "pass", "mydb", true},
		{"user@a.b.c", "user", "a.b.c", true},
		{"host@svc", "", "", false}, // 只有 user/pass 前缀是密钥
		{"user", "", "", false},     // 无 @
		{"user@", "", "", false},    // 服务名为空
		{"x@user@y", "", "", false}, // 前缀必须从名称开头
	}
	for _, c := range cases {
		kind, svc, ok := ParseSecretName(c.name)
		if ok != c.ok || kind != c.kind || svc != c.svc {
			t.Errorf("ParseSecretName(%q) = (%q, %q, %v), 期望 (%q, %q, %v)",
				c.name, kind, svc, ok, c.kind, c.svc, c.ok)
		}
	}
}

func TestMaskedValues(t *testing.T) {
	params := []Param{{Name: "host"}, {Name: "user@db", Secret: true}, {Name: "pass@db", Secret: true}}
	values := map[string]string{"host": "10.0.0.1", "user@db": "alice", "pass@db": "s3cret"}
	got := MaskedValues(params, values)
	if got["host"] != "10.0.0.1" {
		t.Errorf("普通参数不应脱敏: %v", got)
	}
	for _, k := range []string{"user@db", "pass@db"} {
		if got[k] != SecretMask {
			t.Errorf("密钥 %s 应脱敏为 %q, 得到 %q", k, SecretMask, got[k])
		}
	}
	// 值缺失时（执行前预览场景）补脱敏占位
	got2 := MaskedValues(params, map[string]string{"host": "h"})
	if got2["pass@db"] != SecretMask {
		t.Errorf("缺失的密钥值应补脱敏占位: %v", got2)
	}
	// 原映射不被修改
	if values["pass@db"] != "s3cret" {
		t.Error("MaskedValues 不应改动传入的 values")
	}
}

func TestAliasValidateSecretRef(t *testing.T) {
	ok := &Alias{Alias: "db", Command: "mysql -u {{user@mydb}} -p{{pass@mydb}} -h db.local"}
	if err := ok.Validate(); err != nil {
		t.Errorf("合法密钥占位符不应报错: %v", err)
	}
	bad := &Alias{Alias: "db", Command: "mysql -p{{pass@}}"}
	if err := bad.Validate(); err == nil {
		t.Error("{{pass@}} 缺服务名应报错")
	}
}
