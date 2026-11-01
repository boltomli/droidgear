# Wails v3 移植版：审计结论与新版指导

> 审计日期：2026-11-01　适用代码：`spike/wails-v3-minimal`（`6051ae8`）
> 取代 `docs/developer/wails-ui-design.zh.md` 中「P0–P4 已实施 / 仅剩 P5」的结论；
> 该文件保留为历史设计记录（见文首横幅）。
>
> 本文回答三件事：**移植得对不对**（正确性）、**移植得全不全**（完整性）、
> **接下来怎么改**（GUI / TUI / CLI / 上游工具同步的指导规则）。

## 0. TL;DR

| 维度             | 结论                                                                                                                                  | 严重度   |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------- | -------- |
| 命令面连通性     | 前端实际调用的 229 条命令全部有 Go 实现；256 条 shim 映射的参数顺序与 Go 签名逐条对齐；`go build/vet/test`、`tsgo` 全绿               | ✅ 健康  |
| 多工具支持       | 11 个工具（10 工具 + Channels）后端齐备，仅自动更新 3 条命令无 Go 实现（设计例外）                                                    | ✅ 健康  |
| GUI 外壳         | 复用共享组件可运行，但缺 `LegacyConfigDialog`、原生菜单、窗口状态持久化、单实例；外部链接/文件管理器/系统字体三条链路是坏的           | ❌ P0    |
| GUI 细节         | 版本号三处不一致（1.3.9 / 0.2.0 / 0.0.1）；更新检查未做能力屏蔽；通知退回 PowerShell 方案                                             | ❌ P0/P1 |
| 核心写盘正确性   | 原子写、未知字段保留、路径覆盖、`~/.droidgear` 共享均正确；但 OpenClaw 用严格 JSON 读 JSON5、Codex `wire_api="chat"` 已被上游废弃     | ❌ P0    |
| 长时运行稳定性   | PTY 无输出合并、shim 句柄表不回收、无退出清理；会话监视器 500ms 全树遍历                                                              | ⚠️ P1    |
| TUI / CLI        | Wails 侧为 **0**；Rust TUI 仍在且自成一套 core，Go 新功能不会流入 TUI；`Export Templates` 已在 GUI 落地但 TUI 缺失（规则 9 实质已破） | ❌ P1    |
| 上游工具文档同步 | Codex 高危（`chat` wire API 已废弃）、OpenClaw 高危（JSON5）、OpenCode v2 格式预览、Hermes schema 版本待核                            | ⚠️ P0    |
| 工程化           | Wails 完全不在 CI 中；registry 数据是手工复制；40 条 bindings 命令前端从不调用                                                        | ⚠️ P1    |

**一句话**：移植的「骨架」是可信的（命令面、写盘、10 个工具后端、264 项测试都在），
但「外壳周边的系统集成」和「旧实现里本来就存在的上游/格式缺陷」被一起搬过来了，
而 TUI/CLI 这一层目前只有 Rust 老实现，没有被新版覆盖。

---

## 1. 审计方法与可复现证据

```bash
cd wails
gofmt -l . && go build ./... && go vet ./... && go test ./...   # 全通过（app 18.3s）
cd .. && npm run typecheck                                       # tsgo 通过
```

| 检查         | 结果                                                                                            |
| ------------ | ----------------------------------------------------------------------------------------------- |
| Go 方法总数  | 266（263 exported + 3 未导出 helper），生成绑定 263 个函数                                      |
| Go 测试函数  | 229 个（含 `smoke_test.go` 的挂载序列回归）                                                     |
| shim 命令表  | 256 条；与生成绑定逐条比对：**参数个数全等，顺序全等**（仅 `Id`/`ID` 命名差异，位置参数无影响） |
| 前端实际调用 | 229 条（`commands.*` 去重后映射到 invoke 名），缺 shim 的只有 updater 3 条                      |
| 前端从不调用 | 40 条（见附录 A），其中 12 条连 shim 都没有                                                     |
| 静态检查     | `gofmt` 无输出、`go vet` 无告警、`tsgo --noEmit` 通过                                           |

> 复现脚本见附录 B；建议把它固化成 CI 任务（§8）。

---

## 2. 命令面与多工具完整性

### 2.1 工具 × 层次矩阵

| 工具           | Go 后端                                                                          | shim 命令数 | GUI 页面 | 会话查看     | TUI（Rust）   |
| -------------- | -------------------------------------------------------------------------------- | ----------- | -------- | ------------ | ------------- |
| Droid          | `droid_settings.go` / `droid_trusted.go` / `specs.go` / `mcp.go` / `sessions.go` | 32          | ✅       | ✅           | ✅            |
| Codex          | `codex*.go`                                                                      | 27          | ✅       | ✅           | ✅            |
| Claude         | `claude.go` / `claude_settings.go`                                               | 23          | ✅       | ❌（无该页） | ✅            |
| OpenCode       | `opencode.go`                                                                    | 11          | ✅       | ❌           | ✅            |
| OpenClaw       | `openclaw.go` / `openclaw_subagents.go`                                          | 12          | ✅       | ❌           | ✅            |
| Copilot        | `copilot.go`                                                                     | 12          | ✅       | ❌           | ✅            |
| Hermes         | `hermes.go`                                                                      | 10          | ✅       | ❌           | ✅            |
| Pi             | `pi.go` / `pi_sessions.go`                                                       | 17          | ✅       | ✅           | ✅            |
| OMP            | `omp.go` / `omp_sessions.go`                                                     | 26          | ✅       | ✅           | ✅            |
| Dsh            | `dsh*.go` / `dsh_sessions.go`                                                    | 17          | ✅       | ✅           | ✅            |
| Channels       | `channels.go` / `export.go`                                                      | 13          | ✅       | —            | ⚠️ 无导出模板 |
| 通用/窗口/环境 | `app.go` / `paths.go` / `window.go` / `notifications.go` / `recovery.go`         | 46          | ✅       | —            | 部分          |

**结论**：README 宣称的「10 工具 + Channels 全量移植」在命令面上成立。
`isToolSupported()`（`src/lib/capabilities.ts`）现在对所有工具返回 true，
门控逻辑保留为死代码（对 updater 之类的 Tauri-only 面已不再起作用，见 §3.7）。

### 2.2 命令面残留

1. **40 条前端从不调用的命令**（附录 A）：其中 `list_claude_profiles` / `apply_claude_profile` /
   `read_claude_current_config` 等属于「Claude 已改用 settings 文件 UI，profile UI 被移除」后的
   双端死代码；`list_dsh_profiles` 亦然。它们仍在 Rust、Go、shim、bindings 里维护，是纯负担。
2. **12 条既无 shim 也无调用的命令**：`get_env_var` / `set_env_var` / `remove_env_var` /
   `get_default_model` / `save_default_model` / `get_config_paths` / `launch_claude` /
   `get_claude_temporary_run_plan` / `get_codex_temporary_run_plan` /
   `plan_claude_temporary_run_from_file` / `prepare_codex_in_app_run` / `set_active_claude_profile_id`。
   它们属于早期设计的废弃面（临时运行改为 `preview_*` + `launch_*_with_settings`）。
3. **3 条已调用但无 shim**：`check_portable_update` / `install_portable_update` / `get_update_channel`。
   只被 `src/services/updater.ts` 调用，而 updater 只在 Tauri 的 `App.tsx` / 菜单里启动——
   Wails 外壳没有启动它，但 **Preferences → General 的「检查更新」按钮仍在调用它**（§3.7）。

**指导**：新增/删除命令时跑附录 B 的对账脚本；「Rust 有、Go 无」允许（记录原因），
「前端调用、shim 无」一律视为缺陷。

---

## 3. GUI 与 UI/UX 审计

共享组件零改动复用这一点做到了（`MainWindow` / `LeftSideBar` / `MainWindowContent` 一致），
问题全部集中在 **外壳系统集成**。按用户可见程度排序：

### 3.1 `LegacyConfigDialog` 未渲染（功能缺失）

- **证据**：`src/App.tsx:144` 渲染了 `<LegacyConfigDialog />`，`src/wails/AppShell.tsx` 没有。
- **影响**：`~/.droidgear/config.json` 旧配置迁移提示永不出现；Go 端 `CheckLegacyConfig` /
  `DeleteLegacyConfig` 已实现，shim 已映射，属于「后端就绪、前端没挂」。
- **修法**：`AppShell` 的 `MainWindow` 之后补一行，与 Tauri 完全一致。

### 3.2 原生菜单缺失（macOS 上尤其硬伤）

- Tauri `App.tsx` 调 `buildAppMenu()`（`src/lib/menu.ts`：About / Check Updates / Preferences /
  Hide / Quit / Edit / View / Window），Wails 外壳完全没有菜单；`src/lib/menu.ts` 依赖
  `@tauri-apps/api/menu`，Wails 未 alias，属于「不可导入」状态。
- 后果：Windows 上少一整套快捷入口；**macOS 上没有 Edit 菜单 → 复制/粘贴快捷键失效**（Tauri 注释明确写了这一点）。
- **修法**：用 Wails v3 自带菜单 API（`app.NewMenu()` + `AddRole(application.EditMenu)` 等，
  见 `$GOMODCACHE/github.com/wailsapp/wails/v3@v3.0.0/examples/menu`），
  在 Go 侧 `main.go` 构建，文案通过 `i18n` 事件或启动参数注入；语言切换时重建菜单。

### 3.3 外部链接 / 文件管理器（当前是坏的）

- `@tauri-apps/plugin-opener` **没有 shim**（`wails/vite.config.ts` 别名表里没有）。
- 受影响位置：`AboutPane`（GitHub 按钮）、`McpPage`（文档链接）、`LegacyVersionsPage`、
  `ExportTemplatesPage`（`revealItemInDir` / `openPath`）。
- 运行期表现：真实插件会 `invoke('plugin:opener|open_url')` → shim 抛
  `Wails shim: unsupported command`，页面 toast 报错或静默失败。
- **修法**：新增 `src/wails/shims/tauri-opener.ts`（Go 命令 `OpenUrl` / `RevealItemInDir` /
  `OpenPath`：Windows `rundll32 url.dll,FileProtocolHandler` 或 `explorer /select,`；
  macOS `open`；Linux `xdg-open`），并把它加进别名表。这是 §6 表格里的 P0 项。

### 3.4 系统字体（终端字体下拉为空）

- `AppearancePane` 用 `tauri-plugin-system-fonts-api` 的 `getSystemFonts()`，Wails 无 shim。
- 现状：异常被 `catch` 掉，只打日志 → 字体下拉为空，用户只能手输。
- **修法**：Go 侧枚举字体（Windows 读 `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`
  或 `C:\Windows\Fonts`；macOS `system_profiler SPFontsDataType -json` 太重，可用 CoreText；
  Linux `fc-list`），shim 保留同名 API。

### 3.5 通知实现层次偏低

- `wails/internal/app/notifications.go` 用 PowerShell 拼 XML 弹 Windows Toast，
  其它平台直接返回错误。问题：依赖 PowerShell 执行策略、Toast 的 AUMID 未注册（可能静默不显示）、
  无法处理点击/权限。
- **修法**：Wails v3.0.0 自带跨平台通知服务：`github.com/wailsapp/wails/v3/pkg/services/notifications`
  （`SendNotification`、`RequestNotificationAuthorization`、点击回调）；
  以 `application.NewService(notifications.New())` 注册后从这里转发即可，删掉 PowerShell 路径。

### 3.6 版本号三处不一致（用户可见）

| 位置                                    | 值                      |
| --------------------------------------- | ----------------------- |
| `package.json` / Tauri                  | `1.3.9`                 |
| `wails/internal/app/app.go:46`          | `0.2.0`（About 页显示） |
| `wails/build/config.yml` `info.version` | `0.0.1`（安装器元数据） |

- 另有产品标识不一致：Tauri `com.droidgear.app`，Wails `com.droidgear.wailsminimal`
  （影响单实例锁、Toast 归属、安装器升级路径；若「并存」是刻意设计，需在文档与发布物里写清）。
- **修法**：`main.go` 用 `-ldflags "-X main.version=$(node -p require('./package.json').version)"`
  注入，`GetAppVersion` 返回该值；`build/config.yml` 由脚本生成。

### 3.7 更新器：设计例外，但 UI 没有屏蔽

- Wails 构建里 `isUpdateCheckDisabled()` 之类的前端开关没被用到；
  `Preferences → General`（`GeneralPane`）和命令面板的 dev 调试项仍会走 `services/updater.ts`：
  `getUpdateChannel` / `checkPortableUpdate` / `plugin-updater|check` 在 shim 里都是未支持命令。
- **修法**（二选一，必须显式决策）：
  1. 用 Wails v3 的 `pkg/updater`（含 `providers/github`）重做更新链路，前端适配；
  2. 保留 Tauri-only，但在 `capabilities.ts` 增加 `isUpdaterSupported()`，
     Wails 下隐藏/禁用「检查更新」相关入口（这是该文件原本的设计意图，目前是死代码）。
- 短期建议先做 2，长期再评估 1（release 流水线目前也不产出 Wails 产物）。

### 3.8 窗口行为

| 项                  | Tauri                          | Wails 现状                                                              | 结论        |
| ------------------- | ------------------------------ | ----------------------------------------------------------------------- | ----------- |
| 标题栏 / 拖拽       | `data-tauri-drag-region`       | `wails.css` 映射到 `--wails-draggable: drag`，双击最大化由 runtime 处理 | ✅          |
| 窗控按钮            | `window-commands`              | `Window.Minimise/Maximise/Close`，`isMaximized` 用 resize 事件同步      | ✅          |
| 窗口位置/大小持久化 | `tauri-plugin-window-state`    | 无；每次启动 1280×840 居中                                              | ❌ 记忆丢失 |
| 单实例              | `tauri-plugin-single-instance` | 无；可多开 → 同时写同一份 `~/.droidgear/*.json`                         | ❌ 数据竞争 |
| 小屏适配            | 依赖系统                       | `AppShell` 按 `screen.avail* - 48` 缩窗 + `Center()`                    | ✅          |
| 主题底色            | 跟随主题                       | `BackgroundColour: NewRGB(6,7,15)` 固定深色，切浅色主题时窗口底色不跟随 | ⚠️          |
| 退出清理            | 插件负责                       | 无 `OnShutdown`；PTY 子进程/会话 watcher 未显式收尾                     | ⚠️          |

- 单实例：Wails v3 有 `application.Options.SingleInstance`（`UniqueID` + `OnSecondInstanceLaunch`），
  接上后把第二实例的启动聚焦到主窗口即可，与 Tauri 行为对齐。
- 窗口状态：Wails 无内置持久化，需自己存 `x/y/width/height/maximised` 到
  `~/.droidgear/window-state.json`（与 Tauri 插件写的位置不同，需确认是否要兼容读取）。

### 3.9 终端（PTY）UX

`TerminalView` 零改动复用成功，但底层实现有几处与设计文档不符：

1. **没有输出合并**：`pty.go:readLoop` 每读到 ≤8KB 就 `Emit` 一次，base64 编码；
   设计文档写的是「按 ~16ms 聚合再 emit」，实现里没有。大量输出（`npm i`、`cargo build`）
   会以 IPC 风暴形式打到 webview。
2. **shim 句柄表不回收**：`src/wails/shims/tauri-pty.ts` 的 `dataHandlers` / `exitHandlers`
   在 exit 后不 `delete`，`pendingData` 也没有过期；每次重连/新标签都会常驻两个闭包引用（含整个 `IPty`）。
3. **无流控**：`pause()` / `resume()` 直接 `throw`，`clear()` 只 `warn`；
   如果哪天 `TerminalView` 打开 `handleFlowControl` 就会炸。
4. **无退出清理**：没有 `OnShutdown` 杀 PTY 会话；Windows 下建议复用 `Kill` 里的 `taskkill /T`。
5. 剪贴板 shim（`Clipboard.SetText/Text`）与 IME 修复正常。

### 3.10 平台覆盖

`src/wails/shims/tauri-os.ts` 把 `platform()` / `family()` 硬编码为 `windows`，
`version()` 返回 UA 字符串；`notifications` 非 Windows 直接报错；`build/` 下只有
`windows/` 与 `msix` 资产。**macOS/Linux 目前是「能编译、不能正确运行」**：

- 标题栏分支会永远走 Windows 右侧三键（macOS 应左侧红绿灯 + 隐藏原生标题栏）；
- 需要 `MacOptions.MacTitleBarHiddenInset` 或自绘红绿灯；
- Linux 需 `LinuxTitleBar` 分支 + WebKitGTK 拖拽验证。

**指导**：在 `tauri-os.ts` 用 `navigator.userAgent` 或 Go 注入的 runtime 信息判断平台，
而不是硬编码；打包资产与 `build/config.yml` 同步补齐后再宣称跨平台。

---

## 4. 核心功能正确性

### 4.1 共享状态（做对了，别改）

- `appDataDir()` 的三平台取值与 Tauri `path().app_data_dir()` 一致（`%APPDATA%\com.droidgear.app`、
  `~/Library/Application Support/com.droidgear.app`、`$XDG_DATA_HOME/com.droidgear.app`），
  `preferences.json` 两个版本互读。
- `~/.droidgear/settings.json` 的 `configPaths` 覆盖、WSL 路径、`dsh_settings` 均由
  `wails/internal/core/paths*.go` 统一解析，与 Rust 同源。
- `core.AtomicWrite` 自动建目录、临时文件 + rename，`AtomicWritePrivate` 保持 0600（Dsh 凭据）。

### 4.2 未知字段与键序保留（做对了，但脆弱）

`core.UnmarshalJSONWithExtra` / `MarshalJSONWithExtra` 复刻了 Rust 的 `#[serde(flatten)] extra`；
OpenClaw 另外实现了保序树（`openclawObject`）。
⚠️ 脆弱点：`MarshalJSONWithExtra` 要求传入「方法集被剥离的影子类型」，否则无限递归——
`smoke_test.go` 记录过一次真实的栈溢出回归。新增此类类型时必须加往返测试。

### 4.3 JSON5 / JSONC 容错（旧缺陷，被继承）

| 文件                                  | 上游格式                                            | 实现                                                                | 结论                                                                 |
| ------------------------------------- | --------------------------------------------------- | ------------------------------------------------------------------- | -------------------------------------------------------------------- |
| `~/.openclaw/openclaw.json`           | **JSON5**（官方配置参考明确「允许注释与尾随逗号」） | `go`：`json.Unmarshal`；`rust`：`serde_json`                        | ❌ 用户带注释的文件会读失败（`Invalid config JSON`），应用直接不可用 |
| `~/.config/opencode/opencode.json(c)` | JSONC                                               | Rust `json_comments::StripComments`；Go `opencodeStripJSONComments` | ⚠️ 注释能处理，**尾随逗号不能**（两版一致）                          |

**修法**：Go 引入一个 JSON5/JSONC 归一化步骤（去注释 + 去尾逗号 + 单引号），
Rust 同样处理；写回时保持上游可接受的格式（JSON5 文件建议继续写严格 JSON，合法且最安全）。

### 4.4 模型协议推断与 Codex wire API（上游漂移，高危）

1. `wails/internal/app/export.go:225` 的 `inferProtocolFromModelID` 与前端
   `src/lib/model-protocol/global-inference.ts` **不一致**：前端有 `mimo-` → `openai`，
   Go 没有。导出模板里 `model.protocol` 字段会因此与 GUI 判断不同。
   → 修法：把推断规则收敛到一处（JSON 数据表 + 两端读取），或 Go 侧补齐并对齐测试。
2. **Codex 已废弃 `wire_api = "chat"`**（官方 discussion #7782，2025-12；2026 起启动即报
   `wire_api = "chat" is no longer supported`）。而 `ProviderDialog` 仍提供
   `Chat Completions` 选项（`WIRE_API_OPTIONS`），渠道导入时 Anthropic 协议还会**默认推断成 `chat`**。
   → 修法：GUI/TUI/Go 三端把默认改为 `responses`，`chat` 选项标灰并加迁移提示；
   已有 profile 在 Apply/预览时给出警告。

### 4.5 会话监视器（Go 特有）

`wails/internal/app/sessions.go:562` 用 **500ms 全树 `WalkDir`** 轮询替代 Rust 的 notify：
每 500ms 对 `~/.factory/sessions`、`~/.codex/sessions`、`~/.dsh/sessions` 等目录做完整 stat 遍历。
会话多（数千 jsonl）时是明显的 CPU/IO 开销，且 5 个查看器可同时开启。
→ 修法：用 `fsnotify`（Go 生态成熟）做事件驱动 + 兜底低频轮询（如 5–10s），
或至少把间隔调到 1–2s 并只 stat 目录层。

### 4.6 导出模板（Go 侧实现正确，规则执行有遗漏）

JSON / YAML / TOML 三种格式与 Rust `ExportFormat` 一致，`resolveProtocol` 支持
`pattern*` 前缀覆盖。唯一问题是 §4.4 的推断表不同步。

### 4.7 错误字符串与 i18n

- `src/wails/error-i18n.ts` 56 条规则在 `locales/en.json` / `zh.json` 中**全部有译文**（本轮已校验），
  契约完整。
- 但其中 3 条是早期 spike 遗留的**死规则**（`dialogsUnsupported` / `fsUnsupported` /
  `channelsUnsupported`，"minimal build" 时代话术），后端已不再返回这些字符串。
  → 修法：删规则与对应 locale 键，避免以后有人误以为这些能力不存在。
- 新增错误字符串的流程（PORTING.md 已写）保持不变：Go 必须字节级复刻 Rust 文案，
  否则前端匹配失败会退化成英文原文。

### 4.8 数据文件重复（漂移风险）

`wails/internal/core/registrydata/model-registry-data.json` 与
`src/lib/model-registry-data.json` 当前**字节一致**（md5 `caa3e216…`），但没有任何同步脚本或 CI 校验；
`register-model` skill 更新前端数据时不会自动更新 Go 副本。
→ 修法：CI 加 `cmp` 校验，或改成构建期复制（`go:generate` / Taskfile 步骤）。

---

## 5. TUI / CLI：能力评估与分层建议

### 5.1 现状

| 端              | 实现                                                                                                        | 依赖                  | 覆盖面                                                               |
| --------------- | ----------------------------------------------------------------------------------------------------------- | --------------------- | -------------------------------------------------------------------- |
| GUI（Tauri）    | React + Tauri commands                                                                                      | Rust `droidgear-core` | 全量                                                                 |
| GUI（Wails）    | React + Go `internal/app`                                                                                   | Go `internal/core`    | 全量（命令面）                                                       |
| TUI（Rust）     | `src-tauri/crates/droidgear-tui`：`main.rs` 368 + `app.rs` 1928 + `ui.rs` 4690 + `tui/*`（keys\_\* 逐工具） | Rust core             | 10 工具 + Sessions/Specs/MCP/Paths/Channels，**无 Export Templates** |
| CLI（Rust）     | 同一二进制的子命令：`run codex\|copilot\|claude --list`、全局 `--home`                                      | Rust core             | 仅「临时运行」                                                       |
| CLI（Go/Wails） | **不存在**                                                                                                  | —                     | 0                                                                    |

### 5.2 结构性问题

```
Rust: droidgear-core ──► src-tauri（Tauri commands）──► React GUI
                     └─► droidgear-tui（TUI + 少量 CLI）
Go:   internal/core  ──► internal/app（Wails service）──► React GUI（同一份前端）
```

- 两条 core 是**双实现**：任何格式/迁移逻辑都要写两遍、测两遍（§4.3/§4.4 已经出现漂移）。
- AGENTS.md 规则 9「GUI/TUI parity」在 Wails 语境下**无法执行**：新功能只落 Go，
  而 TUI 只认 Rust core。证据：`Export Templates` 已进 GUI（Rust + Go + 前端），TUI 里 0 处引用。
- Wails 构建目前是「纯 GUI」，headless 场景（SSH/容器）没有可用入口。

### 5.3 推荐路线（按性价比）

1. **先抽 `internal/service`**（P5 原计划）：把 `internal/app/*.go` 里的业务逻辑下沉，
   `app` 只做参数编组；对外暴露结构化结果（`Result{files, before, after, hash}`）。
   这一步是 CLI/TUI/Wails 三端复用的前提，且**不改变前端契约**（app 层保留转发方法）。
2. **`cmd/dg` CLI（Go）**：与 GUI 操作一一对应、`--json` 输出可被脚本消费：
   `dg channels list`、`dg dsh providers --profile desktop`、`dg codex apply <id>`、
   `dg export run <template>`、`dg path show`。实现成本低（service 现成），可立即解决 headless 需求。
3. **TUI 去留**（需要显式决策，不要拖）：
   - A. **保留 Rust TUI 作为过渡**，但冻结其新功能，并在 README 标注覆盖差异；
   - B. **Go + bubbletea 重写 TUI**（复用 service，约 7k 行的等量工程）；
   - C. **Rust 为唯一 core**，Go 只做 GUI 适配 —— 与当前「Go 功能先行」方向相反，需回滚大量工作。
     建议：A + 观察 3 个月；`cmd/dg` 先把 headless 的「脚本化」需求吃掉，再评估是否值得重写交互式 TUI。
4. **ChangeSet 契约**（沿用 `docs/developer/tui-design.md` 的 Plan → Preview → Apply）：
   service 层返回「计划写哪些文件 / 前后内容 / hash」，GUI 显示 diff、CLI `--dry-run`、
   TUI 预览共用同一套计算，避免三端各自实现 diff。
5. **规则 9 改写建议**（提交到 AGENTS.md）：
   > 新功能必须先落在共享 service/core 层；随后至少落地一个非 GUI 消费者（TUI 或 CLI），
   > 或在 PR 描述里显式记录豁免与理由。GUI 独有（窗口、菜单、更新、通知）不受此约束。

---

## 6. 上游工具文档同步检查（本轮抽样）

| 工具/配置面                                  | 我们写的内容                                                                                                              | 上游依据与结论                                                                                                        | 动作                                                      |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| Codex `~/.codex/config.toml`                 | `model_providers.<id>.{base_url,wire_api,env_key,http_headers,…}`、`model_catalog_json`                                   | 字段仍有效；但 **`wire_api="chat"` 已废弃**（discussion #7782）；`model_catalog_json` 必须在首个表头之前（TOML 语义） | **P0**：去掉 `chat` 默认/选项                             |
| Claude Code `~/.claude/settings.json` `env`  | `ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY` / `ANTHROPIC_MODEL` / `ANTHROPIC_DEFAULT_HAIKU_MODEL` | 官方文档一致（gateway 场景推荐 `AUTH_TOKEN`，`apiKeyHelper` 用于轮换凭据）                                            | ✅ 保持；可增 `apiKeyHelper` 支持                         |
| OpenCode `opencode.json(c)`                  | `provider.<id>.{npm,options.baseURL,models}`                                                                              | v1 文档一致；**v2 预览改用 `providers.<id>.{package,settings,models}`**                                               | ⚠️ 跟踪 v2 GA，不要提前迁移                               |
| OpenClaw `~/.openclaw/openclaw.json`         | `models.providers.*`、`agents.defaults.model.{primary,fallbacks}`、`agents.defaults.models`                               | 字段与官方配置参考一致；**格式是 JSON5**（注释/尾逗号）                                                               | **P0**：解析器支持 JSON5                                  |
| Pi `~/.pi/agent/models.json`                 | `providers.<id>.{baseUrl,api,apiKey,models[]}`、`$schema` 可选                                                            | 与 pi.dev「Models Configuration」一致（未知 provider 需 `baseUrl` + `api`）                                           | ✅ 保持                                                   |
| Hermes `~/.hermes/config.yaml`               | `model.{provider,default,base_url,api_mode}` + `providers` 字典（v12 形态）、`custom_providers` 迁移                      | 官方文档称「当前 schema version 10」，且 `model` 可为空串哨兵；DroidGear 已支持空串/映射两形态                        | ⚠️ 与上游核实 v10/v12 命名与 `hermes config migrate` 行为 |
| Copilot CLI BYOK env                         | `COPILOT_PROVIDER_BASE_URL/TYPE/API_KEY`、`COPILOT_MODEL`…                                                                | 本轮未取到权威页面                                                                                                    | 待核实（下一轮）                                          |
| OMP `models.yml` / `config.yml`              | 自定义 Provider + 模型角色                                                                                                | 仓库内文档自洽                                                                                                        | 待核实（上游文档入口不明确）                              |
| Dsh `cordis.patch.yml` / `.credentials.yaml` | `llm-pi-ai.providers`、`!!js` 往返、0600 凭据                                                                             | 仓库 `docs/developer/dsh-config-surfaces.zh.md` 已是权威内部规范                                                      | 保持；补上游 DeepSeek Harness 变更跟踪                    |

**指导**：把上表升级为 `docs/agents/tool-config-sources.md`（每行附官方 URL + 最近核对日期），
发版前跑一次「抽样核对」；对「已废弃字段」建立弃用清单，GUI/TUI/Go 同步处理。

---

## 7. 「旧实现的坑是否被带到新版」总表

| #   | 缺陷                                              | 旧版（Tauri/Rust）  | 新版（Wails/Go） | 证据/说明                                      | 优先级       |
| --- | ------------------------------------------------- | ------------------- | ---------------- | ---------------------------------------------- | ------------ |
| 1   | OpenClaw 用严格 JSON 读 JSON5                     | 有                  | **继承**         | `openclaw.go` `json.Unmarshal`；上游明确 JSON5 | P0           |
| 2   | OpenCode JSONC 尾随逗号不支持                     | 有                  | **继承**         | 只去注释，不去尾逗号                           | P1           |
| 3   | Codex `wire_api="chat"` 已废弃仍作为选项/默认     | 有                  | **继承**         | `WIRE_API_OPTIONS`、渠道导入推断               | P0           |
| 4   | 协议推断表两端不一致（`mimo-`）                   | 无（前端唯一）      | **新增**         | `export.go:225` vs `global-inference.ts`       | P1           |
| 5   | 版本号来源不统一                                  | 无                  | **新增**         | 1.3.9 / 0.2.0 / 0.0.1                          | P0           |
| 6   | `LegacyConfigDialog` 未挂载                       | 无                  | **新增**         | `App.tsx:144` vs `AppShell.tsx`                | P0           |
| 7   | 外部链接/文件管理器不可用                         | 无                  | **新增**         | 缺 `plugin-opener` shim                        | P0           |
| 8   | 更新检查未屏蔽                                    | 无                  | **新增**         | `GeneralPane` 仍调用 updater                   | P0           |
| 9   | 系统字体列表为空                                  | 无                  | **新增**         | 缺 system-fonts shim                           | P1           |
| 10  | 无原生菜单（macOS 复制粘贴失效）                  | 无                  | **新增**         | `lib/menu.ts` 不可导入                         | P0（macOS）  |
| 11  | 无窗口状态持久化 / 无单实例                       | 无                  | **新增**         | Tauri 两插件均在                               | P1           |
| 12  | 通知依赖 PowerShell / 平台受限                    | 无                  | **新增（劣化）** | 应改用 Wails notifications service             | P1           |
| 13  | PTY 无输出合并 + shim 句柄泄漏 + 无 shutdown 清理 | 无（Rust PTY 插件） | **新增**         | `pty.go`、`tauri-pty.ts`                       | P1           |
| 14  | 会话监视 500ms 全树轮询                           | 无（notify）        | **新增（劣化）** | `sessions.go:562`                              | P1           |
| 15  | 平台硬编码 windows                                | 无                  | **新增**         | `tauri-os.ts`                                  | P1（跨平台） |
| 16  | 40 条死命令 + 3 条陈旧错误规则                    | 部分                | **继承+新增**    | 附录 A、§4.7                                   | P2           |
| 17  | registry 数据手工复制无校验                       | 无                  | **新增**         | 两份 md5 相同但无同步                          | P2           |
| 18  | 无 CI（Wails 不进流水线）                         | 不适用              | **新增**         | `.github/workflows` 只有 release（Tauri/TUI）  | P1           |
| 19  | TUI 缺 Export Templates（规则 9 已破）            | 有（GUI-only 功能） | **继承**         | TUI 源码 0 处引用                              | P2           |
| 20  | 双 core（Rust/Go）漂移                            | 不适用              | **新增**         | §5.2                                           | P1（战略）   |

**移植检查清单（新增到 PORTING.md）**：每移植一个命令面，除了「参数/JSON/错误串」，
还要显式回答：
① 它依赖的**前端插件**（opener / fonts / notification / menu / updater / process）在新壳里有没有等价实现？
② 它读写的**文件格式**是否允许注释/尾逗号/未知键？上游最近有没有弃用字段？
③ 它在**退出/重启/多实例**下的资源与一致性如何收尾？

---

## 8. 工程化：让这次审计可重复

### 8.1 CI（建议新增 `wails.yml`）

```yaml
name: wails
on:
  pull_request:
    paths: ['wails/**', 'src/**', 'src/lib/model-registry-data.json']
jobs:
  build-test:
    runs-on: windows-latest # ConPTY/Windows 优先；再补 ubuntu 编译 job
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.27' }
      - uses: actions/setup-node@v4
        with: { node-version: 22, cache: npm }
      - run: npm ci
      - run: npm run typecheck && npm run lint
      - name: Go static + tests
        working-directory: wails
        run: |
          gofmt -l . | tee /tmp/fmt && test ! -s /tmp/fmt
          go vet ./...
          go test ./...
      - name: Command-surface drift
        # 落地附录 B 的脚本后启用
        run: node scripts/check-command-surface.mjs
      - name: Shared data drift
        run: cmp src/lib/model-registry-data.json wails/internal/core/registrydata/model-registry-data.json
```

`scripts/check-command-surface.mjs` 用附录 B 的逻辑，断言：

1. 前端调用的每个 `invoke` 名都在 shim 中（白名单：updater 三条）；
2. shim 每条 `fn` 都存在于生成绑定；
3. shim `args` 顺序 == 生成绑定参数顺序（忽略 `Id/ID` 这类大小写差异）；
4. bindings.ts 中前端从未调用的命令清单不增长（允许基线，禁止新增）。

### 8.2 发布流水线

`release.yml` 目前只产出 Tauri 应用与 `droidgear-tui`。若要发布 Wails 版，需要：
`wails3 task build`（Windows）+ 代码签名 + 与 `latest.json` 的更新通道打通（或明确不做更新）。

### 8.3 文档维护

- 本文取代 `wails-ui-design.zh.md` 的「已完成」结论；新决策请直接改本文 §0 / §5 / §9。
- `wails/PORTING.md` 增补「系统集成面清单」（§7 检查清单），并明确：**禁止 no-op shim**
  （如 `emit()` 空实现、`listen()` 名不对），要么实现，要么在 `capabilities.ts` 显式禁用。

---

## 9. 修复路线图（建议）

| 期   | 内容                                                                                         | 验收                                                                          | 状态    |
| ---- | -------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- | ------- |
| P0-1 | 版本号统一（ldflags + config.yml 生成）；`GetAppVersion` 正确                                | About/General 显示 1.3.9，安装器元数据一致                                    | ✅ 完成 |
| P0-2 | 挂 `LegacyConfigDialog`；`opener` shim（openUrl/openPath/revealItemInDir）；更新检查入口屏蔽 | 三个入口可用/不报错；命令面板无 unsupported 报错                              | ✅ 完成 |
| P0-3 | Codex `wire_api` 弃用 `chat`（GUI/TUI/Go 三端 + 迁移警告）                                   | 新导入渠道不再产生 `chat`；旧 profile 有提示                                  | ✅ 完成 |
| P0-4 | OpenClaw JSON5 解析（两端）                                                                  | 带注释/尾逗号的 `openclaw.json` 可读可写，往返不丢键                          | ✅ 完成 |
| P0-5 | 原生菜单（Wails menu API，含 macOS Edit/App 菜单，i18n 联动）                                | macOS ⌘C/⌘V 生效；菜单项与 Tauri 对齐                                         | ✅ 完成 |
| P1-1 | 单实例 + 窗口状态持久化 + `OnShutdown` 清理（PTY/watcher）                                   | 二次启动聚焦主窗口；重启位置记忆；退出无残留 shell                            | ✅ 完成 |
| P1-2 | PTY：输出合并（16ms）、shim 句柄回收、flow control 或显式禁用                                | `npm i` 期间主线程不掉帧；反复开关标签内存不增长                              | ✅ 完成 |
| P1-3 | 会话 watcher 改 fsnotify + 兜底轮询                                                          | 空转 CPU ≈ 0；大目录变更事件 < 1s 到达                                        | ✅ 完成 |
| P1-4 | Wails 通知改用 `pkg/services/notifications`                                                  | Windows/macOS/Linux 均能弹出；点击回调可用                                    | ✅ 完成 |
| P1-5 | CI + 对账脚本 + registry 同步校验                                                            | PR 红灯能挡住漂移                                                             |
| P2-1 | `internal/service` 下沉 + `cmd/dg` CLI（`--json`）                                           | GUI 行为不变；`dg` 覆盖 channels/dsh/pi/hermes/omp/codex 的 list/apply/export |
| P2-2 | 上游文档核对制度化（`docs/agents/tool-config-sources.md`）+ Copilot/OMP/Dsh/Hermes 补齐      | 表格每行有 URL 与核对日期                                                     |
| P2-3 | TUI 决策（保留冻结 / Go 重写）+ 规则 9 改写                                                  | 文档与 AGENTS.md 落地                                                         |
| P3   | 死命令清理（40 条）、陈旧错误规则清理、macOS/Linux 打包与平台 shim                           | 命令面清单收敛；三平台可运行                                                  |

---

## 10. 本轮未覆盖 / 待验证

- macOS / Linux 实际运行（仅静态阅读代码与 Wails 版本 API，未在对应平台验证）。
- OpenCode v2（`providers`/`package`/`settings`）是否已 GA、是否需要兼容双格式。
- Hermes 的 schema 版本号（文档写 10，DroidGear 注释写 v12）与 `hermes config migrate` 的实际行为。
- Copilot / OMP / Dsh 上游权威文档的逐字段核对（本轮只核对仓库内规范）。
- Windows Aero Snap、任务栏缩略图、DPI 125%/150% 下 frameless 窗口的观感。
- Wails 事件通道在高吞吐（PTY 8KB 分片）下的实测吞吐与丢帧阈值。

---

## 附录 A：前端未使用的 40 条命令

```
apply_claude_profile              get_claude_profile                 prepare_codex_in_app_run
create_default_claude_profile     get_claude_temporary_run_plan      read_claude_current_config
create_default_omp_profile        get_codex_cli_capability           read_spec
delete_claude_profile             get_codex_profile                  remove_env_var
duplicate_claude_profile          get_codex_temporary_run_plan       save_claude_profile
get_active_claude_profile_id      get_config_paths                   save_default_model
get_active_claude_settings_file   get_connectivity_summary           set_active_claude_profile_id
get_active_droid_settings_file    get_copilot_profile                set_active_omp_profile_id
get_active_factory_auth_profile   get_default_model                  set_active_pi_profile_id
get_claude_config_status          get_env_var                        set_env_var
get_hermes_profile                get_omp_profile                    launch_claude
get_openclaw_profile              get_opencode_profile               list_claude_profiles
get_pi_profile                    is_codex_official_auth             list_dsh_profiles
plan_claude_temporary_run_from_file
```

其中 12 条连 shim 都不存在：`get_env_var` `set_env_var` `remove_env_var` `get_default_model`
`save_default_model` `get_config_paths` `launch_claude` `get_claude_temporary_run_plan`
`get_codex_temporary_run_plan` `plan_claude_temporary_run_from_file` `prepare_codex_in_app_run`
`set_active_claude_profile_id`。

## 附录 B：命令面对账脚本（可直接放进 `scripts/`）

```js
// node scripts/check-command-surface.mjs
import fs from 'node:fs'

const bindings = fs.readFileSync('src/lib/bindings.ts', 'utf8')
const shim = fs.readFileSync('src/wails/shims/tauri-core.ts', 'utf8')
const generated = fs.readFileSync(
  'wails/frontend/bindings/droidgear-wails/internal/app/app.ts',
  'utf8'
)

// bindings.ts: camelCase fn -> invoke name
const fnToCmd = {}
for (const chunk of bindings.split(/\nasync /).slice(1)) {
  const name = /^([A-Za-z0-9_]+)\(/.exec(chunk)?.[1]
  const cmd = /TAURI_INVOKE[^(]*\("([a-z0-9_]+)"/.exec(chunk)?.[1]
  if (name && cmd) fnToCmd[name] = cmd
}

// generated bindings: Go method -> ordered param names
const goParams = {}
for (const m of generated.matchAll(
  /export function ([A-Za-z0-9_]+)\(([^)]*)\)/g
)) {
  goParams[m[1]] =
    m[2].trim() === ''
      ? []
      : m[2].split(',').map(s => s.trim().split(':')[0].replace(/\?$/, ''))
}

// shim: command -> { fn, args }
const specs = {}
for (const m of shim.matchAll(
  /([a-z0-9_]+): \{\s*fn: '([A-Za-z0-9_]+)',\s*args: \[([^\]]*)\]/g
)) {
  specs[m[1]] = {
    fn: m[2],
    args: m[3]
      .split(',')
      .map(s => s.trim().replace(/^'|'$/g, ''))
      .filter(Boolean),
  }
}

const problems = []
const norm = s => s.toLowerCase()

for (const [cmd, spec] of Object.entries(specs)) {
  const params = goParams[spec.fn]
  if (!params) problems.push(`shim → 缺生成绑定: ${cmd} (${spec.fn})`)
  else if (params.length !== spec.args.length)
    problems.push(`参数个数不一致: ${cmd} shim=${spec.args} go=${params}`)
  else
    spec.args.forEach((a, i) => {
      if (norm(a) !== norm(params[i]))
        problems.push(`参数名不一致: ${cmd}[${i}] shim=${a} go=${params[i]}`)
    })
}

// 前端实际调用的命令（排除集合对象上的 find/forEach/reduce 误报）
const used = new Set()
for (const file of walk('src')) {
  if (file.endsWith('bindings.ts')) continue
  const text = fs.readFileSync(file, 'utf8')
  for (const m of text.matchAll(/commands\.([A-Za-z0-9_]+)\(/g)) {
    if (fnToCmd[m[1]]) used.add(fnToCmd[m[1]])
  }
}
const UPDATER_ALLOWLIST = new Set([
  'check_portable_update',
  'install_portable_update',
  'get_update_channel',
])
for (const cmd of used)
  if (!specs[cmd] && !UPDATER_ALLOWLIST.has(cmd))
    problems.push(`前端调用但 shim 未实现: ${cmd}`)

console.log(
  `shim=${Object.keys(specs).length} bindings=${Object.keys(fnToCmd).length} used=${used.size}`
)
if (problems.length) {
  console.error(problems.join('\n'))
  process.exit(1)
}
console.log('command surface OK')

function* walk(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const path = `${dir}/${entry.name}`
    if (entry.isDirectory()) yield* walk(path)
    else if (/\.(ts|tsx)$/.test(entry.name)) yield path
  }
}
```

## 附录 C：人工验收清单（发版前逐项打勾）

- [ ] 标题栏拖拽、双击最大化、边缘 resize、三键窗控（含最大化状态图标回同步）
- [ ] 关闭有未保存改动 → 弹确认；确认后进程退出且无残留 shell
- [ ] Preferences 五个页签全部可交互：General（含更新入口行为正确）、Appearance（字体下拉非空）、
      Paths（含 WSL）、About（版本号正确、链接可打开）
- [ ] 终端：多标签、重连、复制粘贴（含 Ctrl+Shift+C/V）、字体/Shell 设置生效、退出清理
- [ ] 每个工具的「应用配置」后：目标文件格式合法、未知字段与注释仍在（用真实用户配置回归）
- [ ] 会话查看器：五个工具的列表/详情/删除、实时刷新、空目录不报错
- [ ] 首次启动：旧配置提示、语言自动检测、主题跟随系统
- [ ] 二次启动：聚焦已有窗口（不产生第二个进程）
