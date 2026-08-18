# cm (cmd-mgr)

通用的命令别名管理器——**记住那些记不清的命令**。把长命令、复杂参数的命令存成带描述的别名，用的时候一个 `cm` 打开交互选择界面，打字模糊搜索、回车即执行。

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)

- Go 实现，编译为**单个二进制**，无运行时依赖
- macOS 全量支持；Linux / Windows 已预留（平台差异收敛在 `internal/platform`，Makefile 一键交叉编译）
- TUI 基于 [bubbletea](https://github.com/charmbracelet/bubbletea) + [lipgloss](https://github.com/charmbracelet/lipgloss)

```
 cm · 命令别名 (4)
 搜索: rsyn
 ──────────────────────────────────────────────────────────────────────
 gst       查看 git 状态                          │ gst
 dsync     同步源码到服务器                       │ 同步源码到服务器
                                               │ ╭────────────────────────────╮
                                               │ │ $ rsync -avz --delete ... │
                                               │ ╰────────────────────────────╯
                                               │ #deploy  #rsync
                                               │ 使用 12 次 · 3 天前
 ──────────────────────────────────────────────────────────────────────
 ↑/↓ 移动 · 输入即过滤 · enter 执行 · ctrl+n 新增 · ctrl+e 编辑 · ctrl+d 删除 · esc 退出
```

## 功能

| 需求 | 实现 |
|---|---|
| 输入命令设置别名 + 描述 | `cm add` 交互表单（命令/别名/描述/标签），或非交互 `cm add -a xx -d "描述" -- <命令>` |
| 交互模式选择命令 | 裸 `cm` 打开 TUI：打字即模糊过滤（匹配别名/命令/描述/标签），回车执行 |
| 别名持久化 | JSON 存储，原子写盘；按使用频率自动排序，常用命令浮顶 |
| 统一命令列出可选项 | `cm`（TUI 双栏：列表 + 命令预览）/ `cm list`（表格）/ `cm search <词>` |
| 自动列出可用命令 | `cm add` 时实时从 PATH 补全命令名（tab 补全）并校验；`cm browse` TUI 浏览 PATH 全部可执行命令，选中可直接建别名 |

## 安装

**一键安装（macOS / Linux）**

```bash
curl -fsSL https://raw.githubusercontent.com/HenC49/cmd-mgr/main/scripts/install.sh | bash
```

脚本自动识别系统与架构，从 GitHub Releases 下载对应二进制并校验 sha256，安装到 `/usr/local/bin`（不可写且无 sudo 时退回 `~/.local/bin` 并写入 PATH），最后自动安装 shell 集成。可选环境变量：`CM_VERSION=v0.2.0` 指定版本、`CM_INSTALL_DIR=<目录>` 指定安装位置、`CM_SKIP_INTEGRATE=1` 跳过集成。

**Windows（PowerShell）**

```powershell
irm https://raw.githubusercontent.com/HenC49/cmd-mgr/main/scripts/install.ps1 | iex
```

安装到 `%LOCALAPPDATA%\Programs\cm` 并自动加入用户 PATH。也可从 [Releases](https://github.com/HenC49/cmd-mgr/releases) 手动下载 zip 解压。

**源码安装**

```bash
make            # 编译出 ./cm
make install    # 安装到 /usr/local/bin，并自动把 shell 集成写入 ~/.zshrc / ~/.bashrc（幂等）
make integrate  # 仅安装 shell 集成（install 已自动执行）
```

`make install` 的集成步骤会自动检测 shell（zsh → `~/.zshrc`，bash → `~/.bashrc`），在 rc 末尾追加：

```zsh
# cm (cmd-mgr) shell 集成（cm init --install 添加）
eval "$(cm init zsh)"
```

重复安装不会产生重复条目；也可以手动执行 `cm init --install`。

## 快速开始

```bash
# 1. 添加别名（交互表单）
cm add

# 2. 非交互添加
cm add -a dsync -d "同步源码到服务器" -t "deploy,rsync" -- rsync -avz --delete ./src/ host:/srv/app/

# 3. 使用：裸 cm 打开选择器，打字过滤，回车执行
cm

# 4. 其他入口
cm list            # 表格列出全部
cm search docker   # 模糊搜索
cm run dsync       # 跳过 TUI 直接执行
cm edit dsync      # 编辑
cm rm dsync        # 删除（-f 跳过确认；-i 多选删除）
cm browse          # 浏览 PATH 中所有可用命令
```

## shell 集成（推荐：支持 cd / export / shell 函数）

默认模式下，选中的命令由 cm 起子进程执行。子进程是非交互 shell，**不加载 `~/.zshrc`**，因此用不了 shell 函数和别名（如 SDKMAN! 的 `sdk`、`nvm`），也无法改变当前 shell 的目录和环境变量。
安装集成后，选中的命令交给当前 shell `eval` 执行，`cd`、`export`、`sdk use` 等都能生效：

```bash
# zsh：把这行加入 ~/.zshrc（bash 则是 ~/.bashrc，然后重启终端）
eval "$(cm init zsh)"
```

集成后的行为：

- 裸 `cm`（无参数）→ TUI 选择 → 由**当前 shell** 执行（支持 cd / export / sdk / nvm 等）
- `cm run <别名>` → 同样由当前 shell 执行
- `cm <其他子命令>` → 原样透传给 cm 二进制，行为不变
- 未安装集成时，裸 `cm` 依然是 TUI 选择 + 子进程直接执行（适合 git/docker/ssh 等）

## 常见问题

**执行别名报 `zsh:1: command not found: sdk`（或 nvm、pyenv 等）**

`sdk`、`nvm` 这类命令是 `.zshrc` 里定义的 **shell 函数**，PATH 中并没有对应的可执行文件，cm 的子进程里自然找不到；而且它们通常用于修改当前 shell 的环境（JAVA_HOME、PATH），在子进程里执行即使成功也不会留下任何效果。

解决：安装上面的 shell 集成，然后通过裸 `cm` 选择或 `cm run <别名>` 执行——命令会在你当前的 shell 里运行。直接执行模式遇到 127 退出码时，cm 也会打印这条提示。

## 存储位置

| 平台 | 路径 |
|---|---|
| macOS | `~/Library/Application Support/cmd-mgr/aliases.json` |
| Linux | `~/.config/cmd-mgr/aliases.json` |
| Windows | `%AppData%\cmd-mgr\aliases.json` |

设置 `CM_HOME` 环境变量可覆盖目录（测试/便携场景）。文件为可读 JSON，也可手工编辑（写入采用临时文件 + rename 原子替换）。

## TUI 按键

| 按键 | 作用 |
|---|---|
| 直接打字 | 模糊过滤（查询为空时 `j`/`k` 可上下移动） |
| `↑` `↓` / `ctrl+k` `ctrl+j` | 移动光标（`PgUp`/`PgDown`/`Home`/`End` 亦可） |
| `enter` | 执行选中项（退出码透传） |
| `ctrl+n` | 新增别名（保存后回到选择器） |
| `ctrl+e` | 编辑选中项 |
| `ctrl+d` | 删除选中项（`y` 确认） |
| `esc` / `ctrl+c` | 退出 |

## 开发

```bash
make test    # 单元测试（model/store/discover/ui/picker）
make vet     # go vet
make build-all   # mac + linux(amd64/arm64) + windows(amd64/arm64)
make release     # 全平台编译并打包 dist/（tar.gz / zip / checksums.txt），用于上传 GitHub Release
```

目录结构：

```
internal/
├── cmd/        # cobra 子命令：root(picker)/add/list/rm/edit/run/search/browse/init
├── model/      # Alias 数据模型、校验、按使用排序
├── store/      # JSON 持久化（原子写）、CRUD、使用统计
├── platform/   # 平台差异收敛：配置目录、默认 shell
├── picker/     # 主选择 TUI（即时过滤 + 双栏预览）
├── form/       # 新增/编辑表单 TUI（PATH 补全）
├── browse/     # PATH 命令浏览 TUI
├── discover/   # $PATH 扫描
├── runner/     # 命令执行（shell 选择、退出码透传）
└── ui/         # 共享样式与文本工具（宽度感知截断/换行）
```

## Windows / Linux 预留说明

- 路径、配置目录、shell 选择全部收敛在 `internal/platform`，其余代码平台无关
- Windows 下执行走 `powershell -NoLogo -Command`，`cm init` 目前仅输出 zsh/bash 脚本
- `make build-linux` / `make build-windows` 交叉编译开箱即用（实际运行行为未在对应平台验证）

## License

[MIT](./LICENSE)
