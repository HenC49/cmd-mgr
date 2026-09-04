package secret

import (
	"errors"
	"strings"
	"testing"
)

// withRunner 测试内替换命令执行器，返回捕获到的调用。
func withRunner(t *testing.T, fn runner) *[]struct {
	name  string
	args  []string
	stdin string
} {
	t.Helper()
	var calls []struct {
		name  string
		args  []string
		stdin string
	}
	orig := runCmd
	runCmd = func(name string, args []string, stdin string) ([]byte, error) {
		calls = append(calls, struct {
			name  string
			args  []string
			stdin string
		}{name, args, stdin})
		return fn(name, args, stdin)
	}
	t.Cleanup(func() { runCmd = orig })
	return &calls
}

const darwinAttrs = `keychain: "/Users/u/Library/Keychains/login.keychain-db"
version: 512
class: "genp"
attributes:
    0x00000007 <blob>="mydb"
    "acct"<blob>="alice@example.com"
    "svce"<blob>="mydb"
`

func TestFetchDarwin(t *testing.T) {
	// 密码：-w 只输出密码本身，去掉尾部换行
	withRunner(t, func(name string, args []string, stdin string) ([]byte, error) {
		if args[len(args)-1] != "-w" {
			t.Errorf("pass 类别应带 -w: %v", args)
		}
		return []byte("s3cret pa\"ss\n"), nil
	})
	got, err := fetchDarwin(KindPass, "mydb")
	if err != nil || got != `s3cret pa"ss` {
		t.Fatalf("fetchDarwin(pass) = %q, %v", got, err)
	}

	// 账号：从属性输出解析 acct
	withRunner(t, func(name string, args []string, stdin string) ([]byte, error) {
		for _, a := range args {
			if a == "-w" {
				t.Errorf("user 类别不应带 -w: %v", args)
			}
		}
		return []byte(darwinAttrs), nil
	})
	got, err = fetchDarwin(KindUser, "mydb")
	if err != nil || got != "alice@example.com" {
		t.Fatalf("fetchDarwin(user) = %q, %v", got, err)
	}
}

func TestFetchDarwinNotFound(t *testing.T) {
	withRunner(t, func(name string, args []string, stdin string) ([]byte, error) {
		return nil, errors.New("security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain.: exit status 44")
	})
	_, err := fetchDarwin(KindPass, "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("查找失败应归为 ErrNotFound（供上层引导创建）: %v", err)
	}
}

func TestFetchDarwinNoAccount(t *testing.T) {
	// 项存在但无 acct 属性：明确报错而不是静默空值
	withRunner(t, func(name string, args []string, stdin string) ([]byte, error) {
		return []byte(`class: "genp"
attributes:
    "svce"<blob>="mydb"
`), nil
	})
	if _, err := fetchDarwin(KindUser, "mydb"); err == nil {
		t.Fatal("无 acct 属性应报错")
	}
}

func TestCreateDarwin(t *testing.T) {
	calls := withRunner(t, func(name string, args []string, stdin string) ([]byte, error) {
		return nil, nil
	})
	if err := createDarwin(`my db`, `al"ice`, `p@\ "ss`); err != nil {
		t.Fatalf("createDarwin: %v", err)
	}
	c := (*calls)[0]
	if c.name != "/usr/bin/security" || strings.Join(c.args, " ") != "-i" {
		t.Fatalf("应经 security -i 创建: %v %v", c.name, c.args)
	}
	want := `add-generic-password -s "my db" -a "al\"ice" -w "p@\\ \"ss"` + "\n"
	if c.stdin != want {
		t.Fatalf("stdin = %q, 期望 %q", c.stdin, want)
	}
}

func TestBuildAddLineRejectsNewline(t *testing.T) {
	if _, err := buildAddLine("s", "a", "p\nw"); err == nil {
		t.Error("密码含换行应报错")
	}
	if _, err := buildAddLine("s\n", "a", "p"); err == nil {
		t.Error("服务名含换行应报错")
	}
}

func TestQuoteItem(t *testing.T) {
	cases := map[string]string{
		`plain`:       `"plain"`,
		`has space`:   `"has space"`,
		`has"quote`:   `"has\"quote"`,
		`has\back`:    `"has\\back"`,
		`$var;rm -rf`: `"$var;rm -rf"`,
	}
	for in, want := range cases {
		if got := quoteItem(in); got != want {
			t.Errorf("quoteItem(%q) = %s, 期望 %s", in, got, want)
		}
	}
}

func TestFetchLinux(t *testing.T) {
	withRunner(t, func(name string, args []string, stdin string) ([]byte, error) {
		return []byte("ln-pass\n"), nil
	})
	if got, err := fetchLinux(KindPass, "svc"); err != nil || got != "ln-pass" {
		t.Fatalf("fetchLinux(pass) = %q, %v", got, err)
	}

	withRunner(t, func(name string, args []string, stdin string) ([]byte, error) {
		return []byte("/org/freedesktop/secrets/aliases/default/42\nattribute: service = svc\nattribute: username = bob\n"), nil
	})
	if got, err := fetchLinux(KindUser, "svc"); err != nil || got != "bob" {
		t.Fatalf("fetchLinux(user) = %q, %v", got, err)
	}
}

func TestCreateLinux(t *testing.T) {
	calls := withRunner(t, func(name string, args []string, stdin string) ([]byte, error) {
		return nil, nil
	})
	if err := createLinux("svc", "bob", "pw"); err != nil {
		t.Fatalf("createLinux: %v", err)
	}
	c := (*calls)[0]
	// 密码走 stdin 不进 argv；账号存为 username 属性供 user@ 读取
	if c.stdin != "pw\n" {
		t.Errorf("密码应经 stdin 传入, stdin = %q", c.stdin)
	}
	joined := strings.Join(c.args, " ")
	if !strings.Contains(joined, "username bob") || !strings.Contains(joined, "service svc") {
		t.Errorf("应携带 service 与 username 属性: %v", c.args)
	}
}

func TestFetchBadKind(t *testing.T) {
	if _, err := Fetch("token", "svc"); err == nil {
		t.Error("未知类别应报错")
	}
}
