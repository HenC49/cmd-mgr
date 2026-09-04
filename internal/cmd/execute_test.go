package cmd

import (
	"errors"
	"strings"
	"testing"

	"cmd-mgr/internal/model"
	"cmd-mgr/internal/secret"
)

// stubSecrets 替换密钥读取与创建的间接层；fetch 定义各（类别,服务）的
// 返回值，创建调用记录在 created 中并按 store 里的值返回。
func stubSecrets(t *testing.T, fetch map[string]string, store map[string][2]string) *[]string {
	t.Helper()
	origFetch, origCreateCreds, origCreate := fetchSecret, createCreds, createSecret
	var created []string
	fetchSecret = func(kind, service string) (string, error) {
		if v, ok := fetch[kind+"@"+service]; ok {
			return v, nil
		}
		return "", errors.Join(secret.ErrNotFound, errors.New("item not found"))
	}
	createCreds = func(service string, needUser, interactive bool) (string, string, error) {
		if !interactive {
			return "", "", errors.New("不应在非交互模式走到创建")
		}
		created = append(created, service)
		creds := store[service]
		return creds[0], creds[1], nil
	}
	createSecret = func(service, account, password string) error { return nil }
	t.Cleanup(func() { fetchSecret, createCreds, createSecret = origFetch, origCreateCreds, origCreate })
	return &created
}

func TestResolveSecrets(t *testing.T) {
	params := model.ExtractParams("curl -u {{user@api}}:{{pass@api}} -H {{token}} {{pass@db}}")
	if len(params) != 4 {
		t.Fatalf("参数提取 = %v", params)
	}

	// 全部存在：按参数名返回对应类别
	stubSecrets(t, map[string]string{
		"user@api": "alice", "pass@api": "tok3n", "pass@db": "dbpw",
	}, nil)
	got, err := resolveSecrets(params, false)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"user@api": "alice", "pass@api": "tok3n", "pass@db": "dbpw"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, 期望 %q", k, got[k], v)
		}
	}
	if _, ok := got["token"]; ok {
		t.Error("普通参数不应出现在密钥结果里")
	}
}

func TestResolveSecretsCreatesOncePerService(t *testing.T) {
	// api 服务缺失（user@ 与 pass@ 只创建一次），db 存在
	params := model.ExtractParams("curl -u {{user@api}}:{{pass@api}} {{pass@db}}")
	created := stubSecrets(t,
		map[string]string{"pass@db": "dbpw"},
		map[string][2]string{"api": {"bob", "newpw"}},
	)
	got, err := resolveSecrets(params, true)
	if err != nil {
		t.Fatal(err)
	}
	if got["user@api"] != "bob" || got["pass@api"] != "newpw" {
		t.Errorf("新建凭据应回填 user/pass: %v", got)
	}
	if len(*created) != 1 || (*created)[0] != "api" {
		t.Errorf("api 服务应只创建一次: %v", *created)
	}
}

func TestResolveSecretsNonInteractiveMissing(t *testing.T) {
	params := model.ExtractParams("echo {{pass@nope}}")
	stubSecrets(t, nil, nil)
	_, err := resolveSecrets(params, false)
	if err == nil || !strings.Contains(err.Error(), "非交互") {
		t.Errorf("非交互缺失应报错提示: %v", err)
	}
}

func TestFinishResolvedMasksSecrets(t *testing.T) {
	params := model.ExtractParams("mysql -u {{user@db}} -p{{pass@db}} -h {{host}}")
	values := map[string]string{"user@db": "alice", "pass@db": "s3cret", "host": "10.0.0.1"}
	r := finishResolved("mysql -u {{user@db}} -p{{pass@db}} -h {{host}}", params, values)
	if r.command != "mysql -u alice -ps3cret -h 10.0.0.1" {
		t.Errorf("真实命令 = %q", r.command)
	}
	if strings.Contains(r.masked, "s3cret") || strings.Contains(r.masked, "alice") {
		t.Errorf("脱敏命令泄漏真实值: %q", r.masked)
	}
	if !strings.Contains(r.masked, model.SecretMask) {
		t.Errorf("脱敏命令应含 %q: %q", model.SecretMask, r.masked)
	}
	if r.params["pass@db"] != model.SecretMask || r.params["host"] != "10.0.0.1" {
		t.Errorf("历史参数应为脱敏值: %v", r.params)
	}
}
