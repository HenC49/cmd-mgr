# 设计：密钥占位符（用户名/密码 引用系统密码管理器）

日期：2026-09-04
状态：已实现（自主模式下按本设计直接实施，用户可复查后要求调整）

## 需求

命令中可使用**用户名/密码占位符**：配置时不写明文，执行时从系统密码管理器
（macOS 钥匙串）读取后替换再执行。

## 设计决策

### 语法：`{{user@服务名}}` / `{{pass@服务名}}`

在既有 `{{}}` 语法内做前缀约定，不引入新括号：

- `[[user:svc]]` 之类新括号会与 bash 的 `[[ ]]` 测试语法冲突，无法做严格校验；
- `{{user:svc}}`（复用冒号）与既有 `{{名称:说明}}` 冲突——升级会把老别名的
  `{{user:用户名}}` 静默改成密钥引用；
- `名称@服务` 仍落在原占位符正则内（存储零改动），读作"mydb 的账号/密码"，
  说明语法照常可用（`{{pass@mydb:数据库密码}}`）。

判定规则：参数名匹配 `^(user|pass)@(.+)$` 即为密钥引用；其余含 `@` 的名字
（如 `{{addr@example.com}}`）仍是普通参数。`{{user@}}`（空服务名）在
`Alias.Validate` 报错。

同一服务名的 user/pass 指向**同一条钥匙串项**（macOS generic password 的
svce=服务名；user 取 acct 属性，pass 取项内容）。

### 读取时机：表单确认后、执行前

密钥在参数表单退出后再读取（`resolveSecrets`）：

- 读取其他 App 创建的钥匙串项时 macOS 会弹授权框（"调起系统密码管理器"），
  缺失项的交互创建需要普通终端——两者都不宜发生在 bubbletea 的 raw mode 里；
- 取值失败时执行中止，回到 picker（cm 主循环）。

### 缺失项：现场创建

钥匙串中没有该服务时（`security find-generic-password` 退出码 44）：

- 交互模式：提示输入账号（仅当命令引用了 `user@`）与密码（`x/term.ReadPassword`
  隐藏输入），经 `security -i` 的 **stdin 管道**写入（密码不进 argv，避免 `ps`
  可见；引号转义实测与 security 交互解析器往返一致，含空格/引号/反斜杠/$&;）；
- 非交互模式：明确报错（先在终端交互执行一次，或用钥匙串访问手动添加）。

### 脱敏：密码不上屏、不落盘

真实密钥只出现在最终交给 shell 的命令里：

- 参数表单预览：密钥值渲染为 `••••••`（表单此时甚至还没取值）；
- 执行历史：`history.json` 中的 `command` 与 `params` 均存脱敏版本
  （`model.MaskedValues`）；
- `--pick` / `--print` 输出的被 eval 命令 necessarily 含真实值（eval 需要），
  README 提示不要重定向到文件。

### 非交互可用性

只含密钥引用（无手填参数）的命令**不需要终端**：脚本/管道中直接读取钥匙串执行
（`cm run mydb < /dev/null`）。混合普通参数的命令仍要求交互终端（与原行为一致）。

### 平台

| 平台 | 实现 |
|---|---|
| macOS | `/usr/bin/security`：pass=`find-generic-password -s S -w`；user=解析属性输出中的 `"acct"<blob>="…"`；创建=`security -i` + stdin |
| Linux | `secret-tool`：pass=`lookup service S`；user=解析 `search` 输出的 `attribute: username = …`（创建时把账号存为 username 属性）；密码经 stdin 传入 |
| Windows | 返回 ErrUnsupported（暂无标准 CLI） |

Linux 分支未在真实环境验证（与仓库既有交叉编译立场一致，README 已注明）。

## 改动清单

| 位置 | 改动 |
|---|---|
| `internal/model/params.go` | Param.Secret、ParseSecretName、MaskedValues/SecretMask |
| `internal/model/alias.go` | Validate 拒绝 `{{user@}}` 空服务名 |
| `internal/secret` 新增 | Fetch/Create（darwin/linux/windows 分派，runner 可注入测试） |
| `internal/cmd/execute.go` | resolveCommand 接入密钥：非交互纯密钥直读、表单后 resolveSecrets、createInteractive、历史/预览走脱敏版 |
| `internal/prompt` | 密钥引用渲染为 🔑 只读行；预览脱敏；纯密钥命令（0 输入框）enter 直接确认 |
| `internal/form` | paramHint 区分"参数/密钥"两行；命令项 placeholder 提及密钥写法 |
| `README.md` | 功能表、密钥占位符章节、按键说明、目录结构 |

## 已验证（真实环境，darwin）

- `security -i` 创建 + 读回：空格/引号/反斜杠/$&; 密码往返一致；
- 非交互 `cm run`（纯密钥命令）：钥匙串取值替换后执行，输出正确；
- `cm history` / history.json：命令与参数均为 `••••••`；
- `cm run --print`：stdout 输出真实命令（供 eval）；
- 缺失服务非交互：报错文案正确；`{{pass@}}` 校验拒绝；
- 单元测试：model（提取/脱敏/校验）、secret（darwin/linux 命令构造与解析、
  转义、换行拒绝）、prompt（密钥行/脱敏预览/纯密钥确认）、cmd（按服务去重
  创建一次、脱敏渲染）。

## 明确不做（YAGNI）

- 不做 `{{pass@服务#账号}}` 多账号服务定位（首个匹配即用，需要时再加）；
- 不做 Windows 凭据管理器支持（无标准 CLI，等真实需求）；
- 不把密钥值写入剪贴板或二次暴露；
- 不做 cm 自己的密码库——系统密码管理器是唯一存储。
