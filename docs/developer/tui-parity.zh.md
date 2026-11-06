# Go TUI 对齐（parity）设计 —— 成为默认终端体验，`dg` 保留为脚本化 CLI

- **状态**：设计稿（**决策已确认**，见 §0）
- **日期**：2026-11
- **适用分支**：`spike/wails-v3-minimal`
- **关联**：
  - [`adr-001-tui-fate.zh.md`](./adr-001-tui-fate.zh.md) — 路线 B（Go 重写 TUI）
  - [`wails-tui-cli-design.zh.md`](./wails-tui-cli-design.zh.md) §6 — TUI 决策点与分层规则（R1–R4）
  - [`tauri-removal-design.zh.md`](./tauri-removal-design.zh.md) — 剥离 Tauri 的方案（Q2/Q3 决策）
  - [`tui-design.md`](./tui-design.md) — ChangeSet（Plan → Preview → Apply）契约
  - 执行任务：[`tauri-removal-tasks.zh.md`](./tauri-removal-tasks.zh.md) 阶段 C

---

## 0. 决策（Q4，2026-11 维护者确认）

1. **Go TUI 必须与当前 GUI 完全对齐**：GUI 有的功能，TUI 都要有；
   GUI-only 的部分（窗口、菜单、通知、PTY、更新、字体等）显式豁免（§1）。
2. **对齐完成后，TUI 成为默认终端体验**：README/文档把 TUI 放在首位，
   `dg tui` 是终端里打开 DroidGear 的入口。
3. **`dg` 保留，不退役（2026-11 修订，维护者确认）**：原Q4.3「对齐后移除
   `dg`」**作废**。终端分工为两层：`dg tui` = 交互入口（TUI），`dg <子命令>` =
   可脚本化 headless 层（`--json` / `--dry-run` / exit code，SSH 脚本与 CI
   `cli-headless` 验收的载体，TUI 无法替代）。命令树按
   [`wails-tui-cli-design.zh.md`](./wails-tui-cli-design.zh.md) §4.2 补齐到与
   TUI 屏同域（见 §5 P4）。
4. **推进顺序**：`opencode` / `openclaw` 已标记为**弃用**（维护者 2026-11 指定），
   不做 TUI 屏、不再新增功能；左栏把它们排在最后。其余按矩阵推进。

> 与 ADR-001 的关系：路线 B 不变，验收标准 =「TUI 与 GUI 对齐 + `dg` 命令树
> 补齐」（原「CLI 退役」已作废）。ADR-001 期 4 的验收相应改为本文 §5。

### 0.5 左栏模块顺序（GUI 与 TUI 同源，2026-11 维护者确认）

- **批次 1**（日常主力，按字母序）：`codex` → `droid` → `hermes` → `omp` → `pi`
- **批次 2**（按字母序）：`claude` → `copilot` → `dsh`
- **弃用**（排最后、带「弃用」标记）：`openclaw` → `opencode`
- TUI 模块列表把 GUI 的独立 Channels 按钮放在最前，然后是批次 1 / 批次 2，
  最后是 Preferences 的三个屏（Settings / Registry / Paths）。
- 两处顺序必须一致：GUI = `src/components/layout/LeftSideBar.tsx` 的
  `TOOL_BATCHES`；TUI = `wails/internal/tui/screen.go` 的 `DefaultModules`，
  由 `TestDefaultModulesMatchParityMatrix` 钉住。

---

## 1. 豁免清单（GUI-only，不计入未对齐）

| 能力                                              | 出处                                              | TUI 的替代                                |
| ------------------------------------------------- | ------------------------------------------------- | ----------------------------------------- |
| 窗口（show/hide/size/fullscreen/drag/底色同步）   | `src/lib/shell/window.ts`、`window-theme.ts`      | 终端由终端管理，无对应物                  |
| 原生菜单 / 原生右键菜单                           | `src/lib/shell/menu-bridge.ts`、`context-menu.ts` | TUI 内置的命令面板与右键等价操作          |
| 原生通知                                          | `src/lib/notifications.ts`、Go `notifications.go` | 状态栏与消息区提示                        |
| PTY（每个工具的 `terminal` 子视图、TerminalPage） | `src/lib/shell/pty.ts`                            | 终端里再嵌 PTY 无意义；用户本来就在终端   |
| 自动更新                                          | 已随 Tauri 删除（Q1-A）                           | 打开 Releases 页                          |
| 字体枚举（AppearancePane 的终端字体下拉）         | `src/lib/shell/fonts.ts`                          | 字体由终端模拟器决定                      |
| 以桌面终端启动临时运行（launch / temporary run）  | `service/codex/launch.go` 等                      | 需要桌面终端；后续可选「TUI 内直接 exec」 |
| 文件监听（sessions/specs watcher → GUI 事件）     | `internal/app` 的 poll watcher                    | TUI 打开屏即读最新；事件总线只有 GUI 需要 |

矩阵里这些行标 `GUI-only`，不参与对齐判定。**新增 GUI 功能时必须同时回答：
它属于哪一类（对齐 / GUI-only）**，否则按规则 9 记豁免。

---

## 2. 对齐矩阵

状态：`✅` 已对齐 · `🚧` 部分实现 · `⏳` 未开始 · `GUI-only` 豁免

| GUI 入口                               | 子视图                            | service 包                               | TUI 屏                            | 状态                                                                                                                                                                                                   |
| -------------------------------------- | --------------------------------- | ---------------------------------------- | --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 左栏 · 10 个工具 + Channels            | —                                 | —                                        | 模块列表 + 命令面板               | ✅ P0（模块批次顺序见 §0.5）                                                                                                                                                                           |
| Preferences → Paths                    | 九个路径键 + WSL 信息             | `service/paths`                          | Paths 屏（查看/set/reset）        | ✅ P0                                                                                                                                                                                                  |
| Codex → Providers                      | `providers`                       | `service/codex`                          | Codex 屏 · Providers 分节         | ✅ P0 读                                                                                                                                                                                               |
| Codex → Auth profiles                  | `auth-profiles`                   | `service/codex`                          | Codex 屏 · Auth 分节              | ✅ P1/P2（读 · switch/delete/save-current/rename ✅ 走预览流）                                                                                                                                         |
| Codex → Sessions                       | `sessions`                        | `service/read`（B4）                     | Codex 屏 · Sessions 分节          | ✅ P3（读 · d 删除走预览流）                                                                                                                                                                           |
| Pi → Providers / 默认模型              | `providers` + 默认模型卡          | `service/pi` + `service/importflow`      | Pi 屏 · Providers / Defaults 分节 | ✅ 读 + `i` 渠道导入（`dg pi providers import` 同源）/ `d` 删除；默认模型（`settings.json` 的 defaultProvider/defaultModel/defaultThinkingLevel）e 编辑 · x 清空，与 `dg pi settings` 同源，均走预览流 |
| Hermes → Model                         | `model`                           | `service/hermes`                         | Profiles 屏（通用）               | ✅ P0 读                                                                                                                                                                                               |
| Hermes → Providers                     | provider 条目                     | `service/hermes` + `service/importflow`  | Hermes 屏 · Providers 分节        | ✅ 读 + `i` 渠道导入（`dg hermes providers import` 同源）/ `d` 删除，均走预览流                                                                                                                        |
| OMP → Config                           | `config`                          | `service/omp`                            | OMP 屏 · Config 分节              | ✅ P0 读（15 个模型角色）                                                                                                                                                                              |
| OMP → Providers                        | `providers`                       | `service/omp` + `service/importflow`     | OMP 屏 · Providers 分节           | ✅ 读 + `i` 渠道导入（`dg omp providers import` 同源）/ `d` 删除，均走预览流                                                                                                                           |
| Pi / OMP / Dsh / Droid → Sessions      | `sessions`                        | `service/read`（B4）                     | 各工具屏 · Sessions 分节          | ✅ P3（读 · d 删除走预览流）                                                                                                                                                                           |
| Channels → 列表/详情/token/凭据        | `detail`                          | `service/channels`                       | Channels 屏（分节）               | ✅ P2（list/detail · f token 拉取 · k/p 凭据 · x 清除 · n JSON 建档）                                                                                                                                  |
| Channels → Export templates            | `export-templates`                | `service/export`                         | Channels 屏 · Export 分节         | ✅ P2（list/detail · x run 走预览流）                                                                                                                                                                  |
| Claude → Settings                      | `settings`                        | `service/claude`                         | Claude Settings 屏                | ✅ P1/P2（activate/delete/create/duplicate/merge ✅；launch=§1 豁免）                                                                                                                                  |
| Copilot → Profiles                     | `profiles`                        | `service/copilot`                        | Profiles 屏（通用）               | ✅ P1/P2（apply/delete/save-from-file ✅，五域同形）                                                                                                                                                   |
| Droid → Models                         | `models`                          | `service/factory`                        | Droid 屏 · Models 分节            | ✅ P1                                                                                                                                                                                                  |
| Droid → Settings / Trusted folders     | `settings` `trusted-folders`      | `service/droid`                          | Droid 屏（分节）                  | ✅ P1/P2（值编辑循环/输入 · files use/delete · trusted remove ✅）                                                                                                                                     |
| Droid → Auth profiles / Specs          | `auth-profiles` `specs`           | `service/droid`、`service/specs`         | Droid 屏 · Auth/Specs 分节        | ✅ P1/P2（auth switch/delete/save-current/rename · specs delete/rename ✅）                                                                                                                            |
| Droid → MCP                            | `mcp`                             | `service/mcp`                            | Droid 屏 · MCP 分节               | ✅ P1 读                                                                                                                                                                                               |
| Droid → Missions / Legacy versions     | `missions` `legacy-versions`      | `service/droid`、`service/legacy`        | 同上 · Missions/Legacy 分节       | ✅ P1/P2（missions 模型输入/强度循环 · legacy d 删除 ✅）                                                                                                                                              |
| Dsh → Desktop / Web                    | `desktop` `web`                   | `service/dsh`                            | Dsh 屏（profile 切换）            | ✅ P2（条目 t 开关 · e 字段编辑器（expectedHash）· R restore ✅）                                                                                                                                      |
| OpenCode → Providers                   | `providers`                       | `service/opencode`（弃用）               | —                                 | ⛔ **弃用**：GUI 排在左栏最后并打「弃用」标记，不再新增能力                                                                                                                                            |
| OpenClaw → Providers/Helpers/Subagents | `providers` `helpers` `subagents` | `service/openclaw`（弃用）               | —                                 | ⛔ **弃用**：GUI 排在左栏最后并打「弃用」标记，不再新增能力                                                                                                                                            |
| Preferences → Paths（写）              | set/reset                         | `service/paths`                          | 编辑态 + ChangeSet 预览           | ✅ P2（e 编辑 · x 重置 · 预览→确认→冲突/force）                                                                                                                                                        |
| 任一工具 → Apply profile               | apply/save/delete                 | `service/<域>`                           | 预览（ChangeSet）→ 确认           | ✅ P2（apply/delete/switch/toggle/remove/prompt 建档 ✅，全部走 writeFlow）                                                                                                                            |
| 冲突（外部改动后写盘）                 | —                                 | `core` beforeHash                        | 冲突提示 + `--force` 等价         | ✅ P2（预览与确认双次 dry-run 比对 beforeHash/after；y=force）                                                                                                                                         |
| Preferences → General / Appearance     | 语言、主题                        | `service/preferences`                    | Settings 屏                       | ✅ P1/P2（t/l 循环 · u 开关 · f/h/e 输入 ✅）                                                                                                                                                          |
| Preferences → Models                   | 模型注册表 + 连通性测试           | `core/registry` + `service/connectivity` | Registry 屏（读）                 | ✅（Registry 屏浏览；连通性测试接在 Droid → Models 分节 c/i 与 `dg connectivity`）                                                                                                                     |
| Preferences → About                    | 版本信息                          | `internal/version`                       | `dg version` ✅                   | ✅ 版本（对话框与检查更新见 §1 GUI-only）                                                                                                                                                              |
| 启动临时运行 / 打开终端                | launch                            | `service/<域>`                           | —                                 | GUI-only                                                                                                                                                                                               |
| 窗口 / 菜单 / 通知 / PTY / 更新 / 字体 | —                                 | —                                        | —                                 | GUI-only                                                                                                                                                                                               |

**矩阵的完整性门禁（已落地）**：`node scripts/check-tui-parity.mjs` 解析
`src/store/ui-store.ts` 的子视图类型（`DroidSubView`、`CodexSubView` …），
逐个比对本表的行；GUI 新增子视图而矩阵没跟上 → CI 红。

---

## 3. 阻塞关系：TUI 的上限 = service 的边界

TUI 只允许依赖 `internal/service` + `internal/core`（**R4**：不得 import `internal/app`）。
因此 `internal/app` 里还没下沉的域，TUI 天然够不着：

| 已下沉（TUI 可用）                         | 未下沉（TUI 被阻塞）                                 |
| ------------------------------------------ | ---------------------------------------------------- |
| `paths` `channels` `export`                | `opencode` `openclaw`（弃用，不再下沉、不做 TUI 屏） |
| `codex` `hermes` `pi` `omp`                | —                                                    |
| `factory` `mcp` `preferences`              | —                                                    |
| `copilot` `claude` `droid` `legacy_config` | —                                                    |
| `read`（sessions B4）`specs`               | —                                                    |

结论：**一个域「对齐」= service 下沉 ✅ + TUI 屏 ✅**，两者都打勾才算数。
所以本文的分期与 [`wails-tui-cli-design.zh.md`](./wails-tui-cli-design.zh.md) §3.4 的
B1–B4 批次是同一条工作流的两个视角：`dg` 期看命令覆盖，TUI 期看屏幕覆盖。

---

## 4. 分期

| 期     | 内容                                                                                       | 验收                                             |
| ------ | ------------------------------------------------------------------------------------------ | ------------------------------------------------ |
| **P0** | `internal/tui` 骨架（模块列表 / 详情 / 帮助 / 状态栏）+ Paths 屏 + 四个 profile 域只读     | `go test ./internal/tui`；`dg tui` 在 80×24 可用 |
| **P1** | service B3（claude/copilot/droid/dsh/factory/mcp/preferences）→ 对应屏；Channels/Export 屏 | 每域 service 测试 + TUI 屏测试 + 矩阵更新        |
| **P2** | 写路径：apply/save/delete → ChangeSet 预览 → 确认；冲突（beforeHash）与 `--force` 等价     | 预览与实际写盘同源（`core.BeginDryRun`）         |
| **P3** | Sessions/Specs 只读屏；`opencode` / `openclaw`（后置项）                                   | 矩阵无 `⏳`（除 GUI-only）                       |
| **P4** | 对齐验收 → 默认终端体验（TUI 优先） → `dg` 命令树补齐                                      | §5 的三个动作                                    |

**P1 进度（2026-11）**：Channels / Export / MCP / Models（factory）/ Copilot / Claude /
Droid（Settings·Missions·Trusted·Auth·Specs·Legacy·Sessions，MCP 与 Models 已并入其
分节）/ Settings（Preferences）/ Codex·Pi·OMP·Dsh 各屏与 Sessions（B4，`service/read`，
watcher 留在 app）各屏已落地；每下沉一个 `internal/app` 域就加一个屏，矩阵随之打勾。

**P2 收尾（2026-11）**：输入原语（`tui/prompt.go` 单行输入 + `tui/view.go`
结果查看）落地后，矩阵内全部写路径（token 拉取、channel JSON 建档、Dsh 字段编辑器、
连通性测试）都已接线；**除 OpenCode/OpenClaw 两个后置域外，矩阵无 ⏳/🚧**。

**P2 进度（2026-11）**：写路径统一走 `writeFlow`（`internal/tui/write.go`）——
预览 = 真实代码路径跑在 `core.BeginDryRun` 上（与 `dg --dry-run` 同源），确认时再跑一次
dry-run 比对每个文件的 `beforeHash`/`after`，不一致即冲突提示，`y` 为 `--force` 等价。
已接预览流的操作：paths set/reset、五组 profile 的 apply/delete、claude activate/delete、
codex/droid auth switch/delete、trusted remove、mcp toggle、sessions delete（5 域）、
specs delete、legacy delete、theme 循环。service 侧裸 `os.WriteFile`/`os.Remove`/`os.Rename`
写盘点补了 `core.RecordWrite/RecordRemove/RecordRename` 录制钩子（droid/specs/copilot/
claude/read），保证预览不漏文件。

**P0 的实现约定**（与 `dg` 保持一致）：

- 数据只来自 `internal/service`，**不许**在 TUI 里再拼一份读盘/写盘逻辑。
- 四个 profile 域共用一个通用 Profiles 屏：service 的公共形状
  （`ListProfiles` / `GetProfile` / `GetActiveProfileID` / `GetConfigStatus`）通过
  一个适配接口注入，避免四个几乎相同的实现。
- secrets 一律打码，`--reveal` 等价键（`r`）才显示明文（沿用 §5.3 的规则）。
- 最小可用尺寸 80×24；东亚宽字符按显示宽度排版（`lipgloss.Width`）。

---

## 5. P4：默认终端体验与 `dg` 命令树补齐

达到对齐（矩阵除 GUI-only 外全 `✅`）后，按顺序执行（原「移除 `dg`」条款
已随 Q4.3 修订作废，见 §0）：

1. **默认终端体验**
   - `README.md` / `README_EN.md` / `docs/agents/commands.md`：终端章节改为 TUI
     优先（`dg tui` 是入口），CLI 一节标为「脚本化 headless 层（保留）」。
2. **`dg` 命令树补齐**（形态见 `wails-tui-cli-design.zh.md` §4.2，与 TUI 屏同域；
   写操作支持 `--dry-run`，只读命令输出 `--json`）
   - 已有 ✅：`version` `doctor` `path` `channels` `export` `codex`(+`status`) /
     `pi` `hermes` `omp` `mcp` `factory` `tui`
   - ✅ 已补齐（2026-11）：`dg claude settings …`、`dg copilot profiles …`、
     `dg droid {settings,trusted,auth,missions,legacy}`、`dg dsh providers … + status`、
     `dg sessions projects|list|show`、`dg spec list|cat|delete`（golden 见
     `cmd/dg/parity_test.go`；`droid models` = `dg factory models`）
   - 不做：`dg opencode` / `dg openclaw`（两域已弃用，见 §0.4）
   - 每条命令配 `dg_test.go` golden 断言（含 dry-run 不落盘）
3. **记录**：ADR-001 期 4 验收 =「GUI 与 TUI 对齐 + `dg` 命令树补齐」；
   `PORTING.md` 的 CLI contract 保留（不再改为 TUI contract）。

> `dg` 全程保留：CI 的 `cli-headless` job 与 `--dry-run` golden 继续作为
> headless 验收载体（TUI 是交互式的，无法进脚本）。

---

## 6. 门禁

| 门禁                             | 覆盖                                                  |
| -------------------------------- | ----------------------------------------------------- |
| R4：`check-go-layering.mjs`      | `internal/tui` 不得 import `internal/app` / wails v3  |
| `scripts/check-tui-parity.mjs`   | 矩阵必须覆盖 `ui-store.ts` 的全部子视图（豁免需标注） |
| `go test ./internal/tui`         | Model 级测试：导航、选择、退出、打码（不测终端渲染）  |
| `go test ./internal/service/...` | 每域至少一条断言**落盘最终内容**的用例                |
| `cli-headless` job               | 无 WebKit 环境可构建 TUI（`CGO_ENABLED=0`）           |
| `npm run check:all`              | 前端 + 门禁全绿                                       |

---

## 7. 风险

| 风险                              | 缓解                                                            |
| --------------------------------- | --------------------------------------------------------------- |
| bubbletea/lipgloss 为新增三方依赖 | 已在 `wails/go.mod` 放行；`PORTING.md` 记录理由与版本           |
| TUI 与 GUI 行为漂移（两套交互）   | 共用 service + ChangeSet；矩阵门禁防止「GUI 加了 TUI 没加」     |
| 中文/宽字符渲染错位               | 用 `lipgloss.Width` 计算宽度；CI 跑一条含中文的视图测试         |
| 80×24 下布局溢出                  | 屏幕测试用最小尺寸断言；超长行截断而不是换行错乱                |
| `dg` 命令树与 TUI 屏不同步        | P4 第 2 步逐域补齐；`dg` 的 golden 测试继续当 headless 验收载体 |
