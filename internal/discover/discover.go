// Package discover 扫描 $PATH 列出可用命令，用于添加时补全与 browse 浏览。
package discover

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Entry 一个命令名及其出现的全部路径。
type Entry struct {
	Name  string   `json:"name"`
	Paths []string `json:"paths"`
}

var (
	once    sync.Once
	entries []Entry
	byName  map[string]*Entry
)

// All 扫描 $PATH（目录去重），返回按名称排序的命令列表，结果进程内缓存。
func All() []Entry {
	load()
	return entries
}

// Exists 判断命令名是否出现在 PATH 中。
func Exists(name string) bool {
	if name == "" {
		return false
	}
	load()
	_, ok := byName[name]
	return ok
}

// Suggestions 返回前缀匹配的前 n 个命令名（大小写不敏感），
// 不足时用包含匹配补齐。entries 已按名称排序，前缀结果自然有序。
func Suggestions(prefix string, n int) []string {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" || n <= 0 {
		return nil
	}
	load()
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(strings.ToLower(e.Name), prefix) {
			out = append(out, e.Name)
			if len(out) >= n {
				return out
			}
		}
	}
	for _, e := range entries {
		if len(out) >= n {
			break
		}
		if strings.Contains(strings.ToLower(e.Name), prefix) && !contains(out, e.Name) {
			out = append(out, e.Name)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func load() {
	once.Do(func() {
		entries = scan(os.Getenv("PATH"))
		byName = make(map[string]*Entry, len(entries))
		for i := range entries {
			byName[entries[i].Name] = &entries[i]
		}
	})
}

// scan 扫描一组 PATH（分隔符随平台），返回按名称排序的命令列表。
func scan(pathEnv string) []Entry {
	byName := map[string]*Entry{}
	seenDir := map[string]bool{}
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" || seenDir[dir] {
			continue
		}
		seenDir[dir] = true
		dirents, err := os.ReadDir(dir)
		if err != nil {
			continue // 目录不存在或无权限，跳过
		}
		for _, de := range dirents {
			if !isExecutable(de) {
				continue
			}
			if e, ok := byName[de.Name()]; ok {
				e.Paths = append(e.Paths, dir)
			} else {
				byName[de.Name()] = &Entry{Name: de.Name(), Paths: []string{dir}}
			}
		}
	}
	list := make([]Entry, 0, len(byName))
	for _, e := range byName {
		list = append(list, *e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list
}

func isExecutable(de os.DirEntry) bool {
	name := de.Name()
	if strings.HasPrefix(name, ".") || de.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		lower := strings.ToLower(name)
		return strings.HasSuffix(lower, ".exe") || strings.HasSuffix(lower, ".bat") || strings.HasSuffix(lower, ".cmd")
	}
	info, err := de.Info()
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}
