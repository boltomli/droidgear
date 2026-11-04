# Wails 版 CLI（先行）与 TUI（延后决策）设计与实现

本文件是 ADR-001「TUI/CLI 命运」的**执行细则**：把「Go 共享 service 层 → `cmd/dg` CLI →
Go TUI」这条路线拆成可验收的步骤，并固定 CLI/TUI 的对外契约。

- **状态**：**期 0 与期 1-B1 已落地**（`internal/version`、分层门禁、`service/{paths,channels,export}`、`cmd/dg` 骨架 + 测试），分期与缺口见 §10
- **适用分支**：`spike/wails-v3-minimal`（**Wails v3 单一运行时**，见 §0）
- **关联**：
  - [`adr-001-tui-fate.zh.md`](./adr-001-tui-fate.zh.md) — 决策本体与分期
  - [`wails-port-guidelines.zh.md`](./wails-port-guidelines.zh.md) §5 — 能力评估与分层建议
  - [`tui-design.md`](./tui-design.md) — ChangeSet（Plan → Preview → Apply）的历史设计，仍是契约来源
  - [`../../wails/PORTING.md`](../../wails/PORTING.md)「CLI contract」— GUI 侧的移植规则
  - [`../agents/wails-port.md`](../agents/wails-port.md) — 面向 agent 的 Wails 速查

---

## 0. 本分支的运行时边界：Wails only

从本分支起，**GUI 的唯一目标是 Wails v3 + Go**。Tauri 壳与 Rust TUI 只作只读历史资料，
不再是需要同步的目标：

| 端                         | 本分支状态                                   | 允许的改动                           |
| -------------------------- | -------------------------------------------- | ------------------------------------ |
| `wails/`（Go + React GUI） | **唯一的活跃实现**                           | 正常开发                             |
| `src/`（共享前端）         | 双壳历史遗留——只保留 Wails 壳需要的部分      | 新代码不得为「兼容 Tauri」让步       |
| `src-tauri/`（Tauri 壳）   | **冻结**：不修 bug、不补功能、不加命令/插件  | 仅「为 Go 侧提供参考」时可读，不改码 |
| `droidgear-tui`（Rust）    | **冻结**（ADR-001 已定），由 Go CLI/TUI 取代 | 不改码                               |

冻结的硬规则：

1. **不新增 Tauri command / plugin / shim 映射**。`scripts/check-command-surface.mjs` 的基线只允许收缩。
2. **新增能力先落 Go `internal/service`**，再决定落到 GUI（`internal/app` 转发）与 CLI/TUI。
   反过来（先写 Tauri 再补 Go）在本分支一律拒绝。
3. **Tauri 侧的缺陷不再修**。遇到 `src-tauri` 与 Go 行为不一致，以 Go 为准，并把差异写进
   `wails/PORTING.md` 的契约表，而不是改 Rust。
4. **彻底删除 `src-tauri` 是独立任务**（含 `package.json` 的 `tauri:*`/`rust:*`/`tui:*` 脚本、
   `.github/workflows/release.yml` 的 `publish-tauri` 与 `droidgear-tui` 打包、`AGENTS.md` 规则），
   不在本文件范围内；在此之前「冻结」即满足本分支定义。

> 结论：本分支的「非 GUI 消费者」只有 `cmd/dg`（当前）与未来的 Go TUI。
> 规则 9 的 parity 对象从「Tauri + Wails + TUI」收窄为「Wails GUI ↔ `dg`」。

---

## 1. 目标与非目标

### 目标

| #   | 目标                                                                   | 验收形态                               |
| --- | ---------------------------------------------------------------------- | -------------------------------------- |
| G1  | SSH / 容器 / 无桌面环境下可完成配置读、改、应用                        | `dg` 单文件二进制，不链接 WebKit/Cocoa |
| G2  | 写盘逻辑只有一份（`internal/service`），GUI 与 CLI 不各自实现          | 代码审查 + 分层门禁（§7）              |
| G3  | `--json` 是脚本可依赖的机器契约，字段与前端 bindings 同名（camelCase） | golden 测试固定输出                    |
| G4  | 每个 CLI 动作都对应一个 GUI 动作（或显式豁免），规则 9 可执行          | §4 对照表 + `PORTING.md` 豁免记录      |
| G5  | 危险动作有 dry-run / 冲突检测 / secrets 默认打码                       | §5 的退出码与冲突规则                  |

### 非目标

- 桌面专有能力：窗口、菜单、通知、更新、字体枚举、PTY 终端页签（GUI-only，CLI 不提供）。
- 在 CLI 阶段做交互式全屏 UI（TUI 另行决策，见 §6）。
- 让 CLI 变成第二个「配置系统」：它只是 service 的薄壳。
- 兼容 Rust TUI 的 `--home` 之外的旧参数形态。

---

## 2. 分层与依赖方向

```
   wails/internal/core      纯文件/格式 + 通用工具层：路径解析、atomic write、JSON5/YAML 往返、
                            registry、FileExists / NewHTTPClient / RedactSecret（service 与 app 共用）
            ▲
            │  只被引用，不引用上层
   wails/internal/service   用例层：读 → 计算 ChangeSet → 校验 → 应用；**零 Wails 依赖**
            ▲                        ▲                        ▲
            │                        │                        │
   wails/internal/app      wails/cmd/dg                （未来）wails/internal/tui
   Wails service（薄转发）  CLI（标准库 flag）          bubbletea，延后决策
   含 application.* 适配    不 import app，无 WebKit
```

三条强制规则（可被门禁校验，见 §7）：

| #   | 规则                                                                                       | 违反示例                                       |
| --- | ------------------------------------------------------------------------------------------ | ---------------------------------------------- |
| R1  | `internal/service` 不得 `import "github.com/wailsapp/wails/v3/..."`                        | service 里 `application.Get().Event.Emit(...)` |
| R2  | `internal/app` 的方法体只做「编组参数 → 调 service → 原样返回」，不直接 `os.WriteFile`     | 在 app 里再拼一份写盘逻辑                      |
| R3  | `cmd/dg` 不得 `import "droidgear-wails/internal/app"`（否则 CLI 会拖进 Wails/WebKit 依赖） | CLI 复用 `app.App{}` 的方法                    |

### GUI-only 适配点（必须留在 `internal/app`）

| 能力                                           | 现状文件                                                     | service 侧形态                                    |
| ---------------------------------------------- | ------------------------------------------------------------ | ------------------------------------------------- |
| 事件推送（`application.Get() != nil` 才 emit） | `pty.go`、`sessions.go`、`specs.go`、`codex_sessions.go`     | service 定义 `Emitter` 接口，GUI 注入、CLI 不注入 |
| 菜单 / 通知 / 窗口 / 窗口状态                  | `menu.go`、`notifications.go`、`window.go`、`windowstate.go` | 不迁（GUI 专属）                                  |
| 字体枚举、外部打开、PTY 生命周期               | `fonts*.go`、`opener.go`、`pty.go`                           | 不迁（GUI 专属）                                  |
| 单实例 / 主题底色 / 启动画面                   | `main.go`、`shutdown.go`                                     | 不迁（GUI 专属）                                  |

`Emitter` 的语义要求（延续「no no-op shims」原则）：

- GUI 注入真实实现；**CLI 不实现 `Emitter`（传 `nil`）**，且 CLI 不提供 `--watch` 一类声明——
  宁可没有该 flag，也不要一个永远不触发的假选项。
- service 在 `Emitter == nil` 时必须照常工作（事件只是通知，不是数据通路）。

---

## 3. 期 1：`internal/service` 下沉

### 3.1 包布局

```
wails/internal/service/
├── paths/        配置路径覆盖（对应 app/paths.go + core/paths*.go）
├── channels/     渠道/凭据/token（app/channels.go、export.go 的渠道部分）
├── export/       导出模板与执行（app/export.go）
├── codex/        profile + auth profile + 当前配置（app/codex*.go）
├── pi/  hermes/  omp/  opencode/  openclaw/
├── claude/  copilot/  droid/  dsh/  factory/  mcp/
└── read/         只读域：sessions、specs（app/*_sessions.go、specs.go 的读路径）
```

一个域一个包，**包内自带 HTTP/网络调用的超时与错误分类**；跨域共享的类型（`EffectivePaths`、
`Profile` 基类）放 `service/contract`，**不要**反向依赖 `internal/app` 的类型。

### 3.2 方法签名约定

```go
// ✅ service：Go 习惯签名 + 显式选项 + 结构化结果
func (s *Codex) Apply(ctx context.Context, id string, opts ApplyOptions) (ChangeSet, error)
func (s *Paths) Show(ctx context.Context) (contract.EffectivePaths, error)

// ❌ app 里长出来的业务逻辑（现状反模式）
func (a *App) ApplyCodexProfile(id string) (CodexConfigStatus, error) {
    // 读文件 → 拼 TOML → 写盘 → 返回 status 全在这里
}
```

- 一律带 `ctx`（CLI 有 `--timeout`，GUI 有取消需求）。
- 结果类型放 `service/<域>` 或 `service/contract`；`internal/app` 转发时**直接复用**同一类型，
  保证 bindings 的 JSON 字段不变（这是「不改变前端契约」的硬要求）。
- service 不调用 `os.Exit`、不写日志到 stdout、不读 `os.Args`。

### 3.3 ChangeSet：单一写盘路径

沿用 [`tui-design.md`](./tui-design.md) 的 Plan → Preview → Apply，落到 Go：

```jsonc
// ChangeSet —— GUI diff / CLI --dry-run / 未来 TUI 预览共用同一份计算结果
{
  "applied": false,
  "changes": [
    {
      "path": "/home/u/.codex/config.toml",
      "kind": "toml", // json | jsonc | toml | yaml | text
      "beforeHash": "sha256:…", // 读盘时的哈希，用于陈旧检测
      "after": "…", // 应用后的完整内容（预览时按 secrets 规则打码）
      "redacted": true, // after 是否含打码
      "backup": true,
    },
  ],
  "conflicts": [
    { "path": "…", "reason": "stale", "detail": "文件在计算后被外部修改" },
  ],
}
```

分期落地（重要：不要一次把 GUI 改成「先预览再应用」）：

| 阶段 | 内容                                                                                       |
| ---- | ------------------------------------------------------------------------------------------ |
| 3a   | service 内部把「读 → 算 → 写」拆成 Plan/Apply 两步，`app` 仍调用一次性 `Apply`（行为不变） |
| 3b   | CLI 用 `--dry-run` 暴露 Plan，`--apply`（默认）执行 Apply                                  |
| 3c   | 需要时再让 GUI 消费 Plan 做 diff 面板（可选，非本期承诺）                                  |

### 3.4 迁移批次（按「headless 价值 ÷ 风险」排序）

| 批  | 域                                                                     | 理由                                                              |
| --- | ---------------------------------------------------------------------- | ----------------------------------------------------------------- |
| B1  | `paths`、`channels`、`export`                                          | 只读/半只读 + 明确的写盘点，最快产出 `dg path show` 等验收载体    |
| B2  | `codex`、`pi`、`hermes`、`omp`、`opencode`、`openclaw`                 | profile CRUD + apply 是 headless 主要诉求；格式坑最多，最需要测试 |
| B3  | `claude`、`copilot`、`droid`、`dsh`、`factory`、`mcp`、`legacy_config` | 写面复杂（settings/凭据/DB），需要 B1–B2 的 ChangeSet 收敛        |
| B4  | `sessions`、`specs`（只读）                                            | 纯读 + watcher；watcher 留在 GUI 适配层                           |

### 3.5 期 1 验收

- `go vet ./... && go test ./...`（`wails/`）通过，`npm run check:all` 零错误。
- app 层行为不变：现有 `*_test.go` 全部保持（它们是「前端契约不变」的保险丝）。
- 分层门禁通过（§7）：`internal/service` 无 Wails 依赖。
- 每个已迁移域至少一条 service 级测试，且**断言写盘后的最终文件内容**（不是断言中间结构）。

---

## 4. 期 2：`cmd/dg` CLI

### 4.1 定位

- 独立二进制 `dg`（`wails/cmd/dg`），与 GUI 共享 `internal/service` + `internal/core`。
- **不链接 Wails**：CLI 必须能在没有 WebKitGTK / Cocoa 的镜像里 `CGO_ENABLED=0 go build`。
- 版本号：**已落地（期 0）** —— `internal/version.Version` 是唯一注入点，GUI（`GetAppVersion`、
  About 菜单）与 `dg version` 共用；`wails/build/Taskfile.yml`、`wails/build/windows/Taskfile.yml`
  与 `.github/workflows/release-single.yml` 的 `-X` 目标已同步（改名时三处必须一起改）。
- 依赖策略：**本期只用标准库**（`flag` + 自写表格）。`cobra`/`urfave/cli`/`bubbletea`
  一律延后到 §6 的 TUI 决策点，与 ADR-001「需显式放行第三方依赖」一致。

### 4.2 命令树（镜像 GUI，按层级标注）

```bash
dg version                                    # T3 ✅
dg doctor                                     # T2 ✅（只读：不加载 channels，避免触发迁移写）
dg path show|set <key> <path>|reset <key>     # T1 ✅（set/reset 支持 --dry-run）
dg channels list|show <id>|tokens <id>        # T1/T2 ✅（tokens 默认打码，--reveal 才出明文）
dg export list|save --file <json>|delete <name>|run <name>  # T1/T2 ✅（run 支持 --dry-run）
dg codex profiles list|show <id>|apply <id>   # T1    期 2 下一批（依赖 B2 的 profile 域下沉）
dg codex status                               # T2    同上（对应 GetCodexConfigStatus）
dg pi|hermes|omp|opencode|openclaw|claude|copilot ...  # 同上形态
dg droid models list|apply                    # T1    B3
dg dsh providers list|apply [--profile desktop|web]     # T1    B3
dg mcp list|toggle <id>|apply                 # T1/T2 B3
dg sessions projects|list|show <id>           # T2    B4（只读）
dg spec list|cat <name>                       # T2    B4（只读，写走 $EDITOR 手工路径）
```

✅ = 本仓库已实现并有测试（`cmd/dg/dg_test.go` + `service/*_test.go`）；其余按 §3.4 的批次推进。

覆盖面分级（`PORTING.md` 的 `T1/T2/T3` 含义，沿用本文件）：

| 级别 | 含义                                                    | 规则                                                 |
| ---- | ------------------------------------------------------- | ---------------------------------------------------- |
| T1   | 会产生写盘/外部副作用的动作                             | **必须有 `dg` 对应命令**（否则记入豁免表）           |
| T2   | 只读/诊断动作                                           | 鼓励提供；缺省时 CI 不失败，但需在 README 覆盖表标注 |
| T3   | GUI 专属（窗口、菜单、通知、更新、PTY、字体、外部打开） | 不提供；豁免无需申请                                 |

### 4.3 全局 flags 与输入

| flag            | 语义                                                                   | 状态                                                                      |
| --------------- | ---------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| `--json`        | stdout 输出**单个 JSON 文档**；人读文本不出现在 stdout                 | ✅                                                                        |
| `--dry-run`     | 只计算并输出 ChangeSet（`applied:false`），不落盘                      | ✅（`path set/reset`、`export run`）                                      |
| `--apply`       | 默认行为；显式写出便于脚本自解释（与 `--dry-run` 互斥，同给则 exit 2） | ✅                                                                        |
| `--home DIR`    | 重定向整个 profile（见下）；等价于 Rust TUI 的全局 `--home`            | ✅                                                                        |
| `--force`       | 忽略陈旧冲突（`changeSet.conflicts` 非空时才允许写）                   | ⏳ 随冲突检测（§5.2）落地，**尚未定义**                                   |
| `--timeout DUR` | 单条命令的总截止时间；默认 `0` = 不额外限制                            | ✅                                                                        |
| `--reveal`      | 预览/输出中包含明文 secrets（默认打码，见 §5.3）                       | ✅                                                                        |
| `--quiet`       | 抑制 stderr 进度文本                                                   | ⏳ **暂不定义**：当前没有进度输出，不提供占位 flag（无用 flag = 假 shim） |

`--home` 的实现约束：`internal/core` 用 `os.UserHomeDir()`（`core/paths.go`），
所以 main 在分发命令前设置 `HOME`/`USERPROFILE`；**Windows 还要一起设 `APPDATA`/`LOCALAPPDATA`**，
否则 `%LOCALAPPDATA%/hermes`（Hermes 的优先级链）会指回真实 profile，沙箱就漏了——
`cmd/dg/dg_test.go` 的 `TestHomeFlagSandboxesTheWholeProfile` 锁住这一点。CLI 是短命进程，
进程级覆盖可接受；若将来 TUI/GUI 需要同进程多套 home，再把 `HomeDir` 提升为 service `Options` 字段。

所有全局 flag 都在**命令前后**可写（`dg path show --json` 与 `dg --json path show` 等价）：
main 里先扫一遍 argv 把已知全局 flag 摘出来，剩下的交给子命令自己的 `flag.FlagSet`。

裸 `dg` 打印 usage 并返回 **exit 2**（用法错误）；`dg help` / `dg --help` 打印 usage 返回 **0**。

### 4.4 输出契约（机器可依赖）

```jsonc
// 成功·只读操作：data 直接就是 bindings 的返回形状（数组或对象），不套信封
["…"]                      // dg channels list --json
{ "key": "codex", "path": "/home/u/.codex", "isDefault": false }   // dg path show --json

// 成功·写操作：GUI 侧返回值为空，CLI 给一个最小回执，形状固定
{ "applied": true, "key": "codex", "path": "/elsewhere/codex" }    // dg path set --json
{ "applied": true, "result": { "template": "t1", … } }             // dg export run --json
{ "applied": false, "changes": [{ "path": …, "after": … }] }       // dg export run --dry-run --json
```

```jsonc
// 失败：stdout 保持空，stderr 输出一个 JSON 对象
{
  "error": {
    "code": "conflict",
    "message": "config.toml 在计算后被外部修改",
    "hint": "--force",
  },
}
```

| exit | 含义                                     | 典型 `code`            | 状态      |
| ---- | ---------------------------------------- | ---------------------- | --------- |
| 0    | 成功（含 `--dry-run` 与空变更）          | —                      | ✅        |
| 1    | 运行时错误 / 目标不存在 / 校验失败       | `not_found`、`runtime` | ✅        |
| 2    | 用法错误（flag/参数不对）                | `usage`                | ✅        |
| 3    | 状态冲突（陈旧文件、需要 `--force`）     | `conflict`             | ⏳ 未实现 |
| 4    | 外部依赖失败（网络、上游凭据、上游 CLI） | `upstream`             | ✅        |

exit 4 由 `service/channels.ErrUpstream` 哨兵驱动：包装器只加标记、**不改文案**
（GUI 的 `error-i18n.ts` 按原文匹配），`dg` 用 `errors.Is` 映射成 4。
exit 3 依赖 §5.2 的 `beforeHash` 冲突检测，随 `--force` 一起落地。

其他契约：

- 字段命名与 bindings 完全一致（`camelCase`；Go 侧用 `json:"…"` 标签，**不要在 CLI 层重新命名**）。
- 错误信息中**不得回显** secrets（沿用 GUI 的 `src/wails/error-i18n.ts` 分类思路，CLI 侧按 `code` 分类）。
- 人读模式（不带 `--json`）向 stdout 打表格；进度/警告走 stderr，便于 `dg … > file`。
- 不设置颜色/动画（无 TTY 检测也安全）；需要时 `NO_COLOR` 遵循。

### 4.5 与 GUI 一一对应

任何新增 GUI 写操作，必须同时回答下表，否则按规则 9 记豁免：

| GUI 入口              | service 方法      | `dg` 命令                       | 测试                   |
| --------------------- | ----------------- | ------------------------------- | ---------------------- |
| Preferences → Paths   | `paths.Set/Reset` | `dg path set` / `dg path reset` | golden + 临时 HOME     |
| Export 面板 → Run     | `export.Run`      | `dg export run <tpl>`           | golden 导出目录快照    |
| Codex → Apply profile | `codex.Apply`     | `dg codex profiles apply <id>`  | 落盘内容 + `--dry-run` |

---

## 5. headless 与安全约束

### 5.1 无桌面环境

| 约束                  | 处理                                                                  |
| --------------------- | --------------------------------------------------------------------- |
| 无 `DISPLAY`/WebView  | CLI 完全不初始化 Wails（R3），因此天然可跑                            |
| 容器里的 `HOME`       | `--home` 或 `HOME=` 环境变量；`dg doctor` 打印解析后的 effective 路径 |
| WSL                   | 复用 `core/wsl*.go` 的探测结果；`dg doctor` 报告 `WslInfo`            |
| 只读文件系统 / 无权限 | 归为 exit 1 + `code: "permission"`，不进入重试循环                    |
| 长驻/重复调用         | CLI 无单实例锁：靠 §5.2 的冲突检测，而非抢占 GUI 的 single-instance   |

### 5.2 并发写（CLI 与 GUI 同时开着）

- 写盘统一走 `core.AtomicWrite`（tmp → rename），并使用 service 的 `beforeHash` 做**乐观并发控制**：
  计算时记录文件哈希，写前重新读取比对，不一致则返回 `conflict`（exit 3）并提示 `--force`。
- GUI 侧保持单实例（`main.go` 的 `SingleInstance`）不变；**CLI 不参与该锁**——
  否则 SSH 里的脚本会因为桌面端开着而随机失败。
- `--force` 语义：丢弃 `beforeHash` 校验，但仍走原子写 + 备份（`backup:true`）。

### 5.3 secrets

| 场景                  | 默认                              | 显式动作   |
| --------------------- | --------------------------------- | ---------- |
| `--json` 列表/详情    | 凭据字段打码（`****1234` 形态）   | `--reveal` |
| `--dry-run` ChangeSet | `after` 打码并标 `redacted:true`  | `--reveal` |
| 实际写盘              | 完整内容（打码只影响展示）        | 无需       |
| 输出重定向/日志       | 永不写明文 token                  | 无需       |
| 凭据文件权限          | 保持 `0600`（沿用 Go 侧现有实现） | 无需       |

---

## 6. 期 4：TUI —— 延后决策（记录理由与触发条件）

**当前决策：CLI 稳定前不做 TUI，也不引入 `bubbletea`/`lipgloss`/`cobra`。**
ADR-001 选择路线 B（Go 重写 TUI）仍然有效，但执行顺序调整为「CLI 先行、TUI 按需启动」。

触发条件（任一满足再启动）：

1. B1–B2 的域已迁到 service，且 `dg` 在真实 SSH/容器场景跑通一轮；
2. 出现「CLI 无法表达的批量交互」需求（多选、逐项 diff 审核）；
3. 维护者明确要移除 Rust TUI 的发布项（ADR-001 期 4）。

届时需要显式决定并记录的事项：

| 待决          | 选项                                                    | 记录位置                     |
| ------------- | ------------------------------------------------------- | ---------------------------- |
| UI 框架       | `bubbletea + lipgloss`（默认） / 纯标准库 ANSI          | 本文件 + `wails/go.mod` 注释 |
| 依赖放行      | `PORTING.md` 目前限制新增三方依赖，需显式豁免并写理由   | `PORTING.md`                 |
| 交互模型      | 沿用 `tui-design.md` 的左侧模块 + 右侧列表/详情/Preview | `tui-design.md`              |
| 与 CLI 的关系 | 同一二进制 `dg tui` 子命令 / 独立 `dgtui` 二进制        | 本文件                       |
| i18n          | 复用 `locales/`？仅英文？                               | 本文件                       |

TUI 的唯一复用面是 **service + ChangeSet**：TUI 不得绕过 service 直接读文件，
也不得自带第二套 diff 渲染数据源。

---

## 7. 质量门禁

| 门禁                                          | 覆盖                                                                              | 状态      |
| --------------------------------------------- | --------------------------------------------------------------------------------- | --------- |
| `go vet ./... && go test ./...`               | service/app/CLI 单测（251 条，含 26 条新增）                                      | ✅ 已引入 |
| 分层门禁 `scripts/check-go-layering.mjs`      | R1/R3：`go list -deps` 里不得出现 `wailsapp`（service）、`internal/app`（cmd/dg） | ✅ 已引入 |
| CI job `wails.yml → cli-headless`（ubuntu）   | `CGO_ENABLED=0 go build ./cmd/dg` + service/core/cmd 测试；**不装 wails3/npm**    | ✅ 已引入 |
| CI job `wails.yml → build-test`（windows）    | 前端 + Go 全量 + 分层门禁 + 命令面 + 版本/registry 漂移                           | ✅ 已引入 |
| `node scripts/check-command-surface.mjs`      | 命令面基线（含**参数名**，只允许收缩，见 §0 规则 1）                              | ✅ 已有   |
| `npm run check:all`                           | 前端 + Rust 历史门禁（保持绿；本分支不新增 Rust 代码）                            | ✅ 已有   |
| `node wails/scripts/sync-version.mjs --check` | 版本号单一来源（改 ldflags 目标必须三处同步，见 §4.1）                            | ✅ 已有   |

CLI 契约测试的写法约定：直接调 `run(args, &stdout, &stderr)`（它不调 `os.Exit`，
可测），用 `t.Setenv` 把 `HOME`/`USERPROFILE`/`APPDATA`/`LOCALAPPDATA` 指到 `t.TempDir()`
（`t.Setenv` 会在用例结束时还原，而 `run(--home …)` 自己改的环境靠它兜底）；
断言「stdout JSON + exit code + 落盘文件内容」三者，**不 mock service**（那是 service 单测的事）。
不用 `exec` 编译出的二进制：进程内调用覆盖同一代码路径，且省掉测试期的 build。

> 为什么必须有 `cli-headless`：`internal/app` **无法**交叉编译到 linux
> （`wails/v3/pkg/application` 拉进 gtk 绑定），而 `cmd/dg` + `service` + `core` 可以。
> 这个 job 就是 R3 的可执行证明——它一旦变红，说明 CLI 拽进了 GUI 依赖。

---

## 8. 实施顺序

| 期  | 内容                                          | 状态 / 完成判据                                         |
| --- | --------------------------------------------- | ------------------------------------------------------- |
| 0   | `internal/version` 提取；分层门禁脚本         | ✅ 已完成（ldflags 三处同步，门禁接入 `wails.yml`）     |
| 1   | `internal/service` 抽取（B1 → B2 → B3 → B4）  | 🚧 B1（paths/channels/export）已完成；B2–B4 待做        |
| 2   | `cmd/dg` 骨架 + T1 命令                       | 🚧 骨架 + paths/channels/export 已完成；codex/pi/… 待做 |
| 3   | `dg` 覆盖 T1 全量 + T2 只读面                 | ⏳ 未开始（§4.5 对照表无缺口或有豁免记录）              |
| 4   | TUI 决策（§6）或删除 `src-tauri`（§0 规则 4） | ⏳ 未开始（独立 ADR/任务，不在本期承诺）                |

---

## 9. 维护约定

- 改动 CLI 契约（flag、exit code、JSON 形状）必须同时更新本文件 §4 与 golden 测试。
- 新增 GUI 写操作时，先补 `internal/service` 方法与 `dg` 子命令，再写前端调用；
  `PORTING.md`「CLI contract」与本文件冲突时，以本文件为准并回写 `PORTING.md`。
- 「豁免」必须写进 `wails/PORTING.md` 的豁免清单（含理由与日期），口头豁免无效。
- **改 `internal/app` 方法的参数名会弄红 CI**：生成的 bindings 按参数名与 shim 对齐
  （`check-command-surface.mjs` 报 `参数名不一致`）。service 内部用什么名字随意，
  但 app 转发层的签名（含参数名）是前端契约——必要时像 `app/channels.go` 那样
  给 service 包起别名（`svc "…/service/channels"`）来腾出参数名。

---

## 10. 落地状态（本分支当前进度）

| 模块                                                                           | 状态 |
| ------------------------------------------------------------------------------ | ---- |
| `internal/version`（GUI/CLI 共用 ldflags 目标）                                | ✅   |
| `scripts/check-go-layering.mjs` + `wails.yml` 两个 CI job                      | ✅   |
| `internal/service/paths`（九个路径键 + WSL）                                   | ✅   |
| `internal/service/channels`（列表/凭据/token/模型/类型探测 + `ErrUpstream`）   | ✅   |
| `internal/service/export`（模板 CRUD + 运行引擎 + dry-run 预览与打码）         | ✅   |
| `cmd/dg`：`version` `doctor` `path` `channels` `export`                        | ✅   |
| 发布构建：`release-single.yml` 三端 + `release.yml` Windows 都产出 `dg` 单文件 | ✅   |
| 测试：`service/*` 15 条 + `cmd/dg` 13 条契约测试（合计 253 条）                | ✅   |
| `internal/service/{codex,pi,hermes,omp,opencode,openclaw,…}`（B2/B3）          | ⏳   |
| `dg` 的 codex/pi/… 子命令（期 2 剩余）                                         | ⏳   |
| `--force` + `beforeHash` 冲突检测（exit 3）                                    | ⏳   |
| TUI（§6 决策点）/ 删除 `src-tauri`                                             | ⏳   |

下一批建议顺序：B2 的 `codex`（profile CRUD + apply，headless 需求最高）→
`dg codex profiles …` → 再回来补冲突检测（那时 `--force` 才有真实语义）。
