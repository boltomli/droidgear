# Wails 版完整 UI 设计方案（GUI/TUI/CLI 一并考虑）

> 状态：设计稿（未实施）。目标是把 `wails/` spike 的「最小外壳」升级为与 Tauri 版**像素级一致**的完整界面（参考 `无标题.png`：TitleBar + 左侧「渠道/工具切换」+ 子功能列表 + 主内容区 + 终端），并为将来的 **CLI / TUI** 预留同一套 Go 核心。

## 1. 现状盘点

### 1.1 已有

| 层       | 现状                                                                                                                                                |
| -------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| 前端外壳 | `src/wails/AppShell.tsx` 最小外壳：静态 SideNav（Dsh×2/Pi/Hermes/OMP 五个平铺入口），无 TitleBar、无渠道、无终端                                    |
| 共享组件 | `src/components/**` 全部可复用（shims 已覆盖 `@tauri-apps/api/core`、`event`、`window`、`plugin-os/dialog/fs`）                                     |
| Go 后端  | `wails/internal/app/` 约 70 个方法：Preferences、Factory(BYOK)、Channels（CRUD/凭据/token 拉取/模型探测）、Dsh（全量含高级编辑器）、Pi、Hermes、OMP |
| Go core  | `wails/internal/core/`：paths、原子写、YAML/JSON、WSL 探测、registry 数据                                                                           |
| 绑定管道 | `wails3 generate bindings -ts` → `@wails-bindings` → `tauri-core.ts` 的 `COMMANDS` 映射                                                             |

### 1.2 缺口（对照截图所需 UI 逐一盘点）

**A. 外壳/UI 缺口**

1. TitleBar：自定义标题栏（设置按钮 + 拖拽 + Windows 窗控按钮）。`tauri-webview-window.ts` shim 目前是 no-op。
2. LeftSideBar：渠道按钮 + 工具下拉（10 个工具）+ 各工具子功能列表（模型配置/终端等）+ 底部 hint。**组件本身已存在且零修改可复用**，缺的是它依赖的后端命令。
3. MainWindowContent：子视图路由 + 终端 overlay（`terminalActive` 时全屏覆盖）。组件可复用。
4. CommandPalette / PreferencesDialog / 关闭确认：组件可复用，个别命令未实现（见 B）。
5. Vite alias 缺 `@tauri-apps/plugin-clipboard-manager`、`@tauri-apps/plugin-notification`（终端页、通知用到）。

**B. Go 后端命令缺口**（前端 store/组件已引用但 Go 没有）

| 优先级 | 命令组       | 命令                                                                                                                              | 说明                                                |
| ------ | ------------ | --------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------- |
| P0     | 窗口/状态    | `reset_window_state`、`load_emergency_data`、`save_emergency_data`                                                                | MainWindow/崩溃恢复用                               |
| P0     | 通知         | `send_native_notification`                                                                                                        | Wails v3 有 `application.NewNotification`，直接映射 |
| P0     | 导出模板     | `load_export_templates`、`save_export_template`、`delete_export_template`、`run_export_template`                                  | 渠道页子导航「导出模板」需要                        |
| P1     | PTY          | spawn/write/resize/kill + 数据流                                                                                                  | 见 §4，最大的一块                                   |
| P1     | 会话监视     | `start_sessions_viewer`、`stop_sessions_viewer`、`list_sessions`、`get_session_detail`、`delete_session`、`list_session_projects` | 各工具 Sessions 子页；只读文件系统扫描，可移植      |
| P2     | 规格/MCP     | `list_specs`、`update_spec`、…、`load_mcp_servers`、`save_mcp_server`…                                                            | Droid 子页；等 Droid 后端整体移植时一起做           |
| P2     | 其余工具后端 | Codex/OpenCode/Claude/OpenClaw/Copilot/Droid 全部 profile 命令                                                                    | 工作量大，见 §5 分期                                |

**C. 架构缺口（TUI/CLI 视角）**

- 业务逻辑目前长在 `internal/app`（service 方法）里，service 直接是 Wails 绑定面。将来 CLI/TUI 要复用，需要明确分层（见 §3）。

## 2. UI 设计：对齐截图的外壳

直接复用现有共享组件，`src/wails/` 只写「拼装 + 平台差异」：

```
src/wails/
├── AppShell.tsx          # 改为：TitleBar + (LeftSideBar + MainWindowContent) + 全局组件
├── WailsWindowBridge.tsx # 新增：把 window shim 接到 Wails runtime（窗控/拖拽/主题）
├── capabilities.ts       # 新增：Go 后端能力清单（工具、子功能门控）
├── components/…          # 现有 SideNav 删除，改用共享 LeftSideBar
└── shims/                # 补 clipboard-manager、notification；window shim 变真实现
```

要点：

1. **LeftSideBar / MainWindowContent 零修改复用**。它们只依赖 store + `commands.*`；通过 `capabilities.ts` 做特性门控——工具下拉里 Go 后端未就绪的工具（Droid/OpenCode/Codex/Claude/OpenClaw/Copilot）在 Wails 构建中**隐藏**（或显示为 disabled + "即将支持"），而不是点击后抛 `unsupported command`。
   - 实现方式：`SECTIONS`/`NavigationView` 遍历处读一个注入的 capability 集合（Tauri 构建注入"全开"，Wails 构建注入 Go 实际能力），避免为每个工具写 if。未就绪工具以 **disabled 占位**渲染（灰显 + tooltip「该工具的 Wails 支持即将推出」），布局与截图完全一致。
2. **TitleBar**：共享 `TitleBar` 组件 + 真实 window shim。
   - Wails v3 窗口用 frameless（`application.WebviewWindowOptions.Frameless: true`），拖拽用 `application.WindowDrag()`（shim 里把 `data-tauri-drag-region` 转发为 mousedown → WindowDrag）。
   - 窗控按钮：Minimise/Maximise/Close → `Window.Minimise()/Maximise()/Close()`。
   - `usePlatform()`（plugin-os shim）返回 `'windows'`，走右侧窗控分支，与截图一致。窗口/二进制命名保留 `DroidGear Minimal`，便于与 Tauri 版并存区分。
3. **终端 overlay**：MainWindowContent 已内置（`terminalActive` 时绝对定位覆盖），前端无需新代码，只缺 PTY 后端。
4. **视觉一致性**：共享同一份 `App.css`/Tailwind 主题，`wails.css` 只保留高度链覆盖。窗口默认 1280×840、min 800×600 已与 Tauri 版一致。

## 3. 分层架构：为 CLI/TUI 预留

关键决策：**业务逻辑收敛到 `internal/core` + `internal/service`，三个消费端共享**。

```
internal/
├── core/        # 纯函数：paths、YAML/JSON round-trip、原子写、WSL、registry
├── service/     # 从 internal/app 迁出：DshService、PiService、HermesService、
│                #   OmpService、ChannelService、FactoryService、PreferencesService
│                #   无 Wails 依赖，错误用可本地化 sentinel（error-i18n 契约不变）
└── app/         # 薄壳：Wails 绑定方法 = service 调用 + 参数编组 + 结构体定义
```

三个消费端：

| 端  | 形态                                                          | 复用方式                                                                                                                                                                          |
| --- | ------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GUI | Wails v3（本方案）                                            | `app` 绑定层                                                                                                                                                                      |
| CLI | `cmd/dg`（独立 `dg` 命名，cobra 或标准库 flag）               | 直接调 `service`；子命令与 GUI 操作一一对应（`dg channels list`、`dg hermes apply --profile 默认`、`dg export run <模板>`）；`--json` 输出供脚本/ TUI 集成                        |
| TUI | `cmd/droidgear-tui`（bubbletea+lipgloss，或继续维护 Rust 版） | 调 `service`；"Plan/Preview/ChangeSet" 模型与 GUI 一致——service 层统一暴露 `Plan → Preview(diff) → Apply` 三段式，GUI 的「应用配置文件」与 TUI 的「预览并应用」共用同一 diff 计算 |

原则（呼应 `docs/developer/tui-design.md` 的 ChangeSet 设计）：

- service 层方法返回**意图与结果分离**的结构（计划写哪些文件、写完内容、hash），GUI 只负责渲染，不重新实现业务。
- 错误保持 sentinel/结构化，前端 `error-i18n.ts` 与 CLI 的错误展示共用同一份映射。
- TUI/TUI parity 规则（AGENTS.md 规则 9）下沉为：**新功能必须先在 service 层落地，GUI/TUI/CLI 是三个适配器**，而不是"GUI 实现完再补 TUI"。

关于 Rust 侧：现有 `src-tauri/crates/droidgear-tui` + `droidgear-core` 与 Go core 是**双实现**。建议把 Rust 版定位为"过渡期并行"，新功能只进 Go service；待 Wails GUI 达到 parity 后评估退役 Rust core（开放问题见 §7）。

## 4. 终端（PTY）方案

现状：前端 `TerminalView` 用 npm 包 `tauri-pty`（spawn/IPty：write/resize/kill/onData/onExit），背后是 Rust `tauri-plugin-pty`（Windows 走 ConPTY）。

Wails 侧方案：

1. **Go PTY 服务**：`internal/app/pty.go` + `internal/core/pty/`。
   - Windows：`github.com/creack/pty`（v1.1.20+ 支持 ConPTY）或 `golang.org/x/sys/windows` 直接 ConPTY；Linux/macOS：`creack/pty` forkpty。
   - 会话管理：`map[sessionID]*ptySession`（cmd、stdin/stdout、尺寸、退出状态），`PtyStart(shell,args,cwd,env,cols,rows)→id`、`PtyWrite(id,data)`、`PtyResize(id,cols,rows)`、`PtyKill(id)`。
2. **数据流**：输出通过 **Wails Events**（`EventsEmit("pty:data", {id, data})`）推给前端；退出推 `pty:exit`。二进制转 base64 或 UTF-8 分片，避免 webview 编码问题。
3. **前端 shim `tauri-pty`**：新增 `src/wails/shims/tauri-pty.ts` 实现 `spawn()` 返回 `IPty`（内部订阅 Wails Events），vite alias 把 `tauri-pty` 从"直接引 dist"改为 shim。`TerminalView` 零修改。
4. **剪贴板**：shim `@tauri-apps/plugin-clipboard-manager` → Wails `ClipboardSet/Get`（v3 runtime 已有）。
5. 自定义 shell（`terminal_shell_command`）、字体（`terminal_font_family`）已走 preferences 命令，Go 已实现，无需新代码。

风险点：ConPTY 在旧版 Windows 10 的兼容性；事件背压（快速输出时按 chunk 限流/合并）；WebView2 与 xterm.js 的输入法问题已有 `terminal-webkit-input-fix.ts` 兜底。

## 5. 分期计划

| 期                 | 内容                                                                                                                                                                                                       | 交付                                                                               |
| ------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| **P0 外壳对齐**    | TitleBar 真实化（frameless+窗控+拖拽）、capabilities 门控、复用 LeftSideBar/MainWindowContent、补 clipboard/notification/dialog shim、Go 补 `reset_window_state`/`send_native_notification`/emergency data | Wails 版外观与截图一致（渠道、Dsh/Pi/Hermes/OMP 全功能；未实现工具 disabled 占位） |
| **P1 渠道补全**    | Go 补导出模板四命令；channels 子页（ChannelList/ChannelDetail/ChannelDialog）全通                                                                                                                          | 渠道管理与 Tauri 版等价（token 拉取、模型探测后端已就绪）                          |
| **P2 终端**        | Go PTY + Events 流 + `tauri-pty` shim；`terminal_shell_command`/字体生效                                                                                                                                   | 截图左侧「终端」子页可用，多标签、重连、剪贴板                                     |
| **P3 会话/只读页** | Go 移植 sessions 扫描、specs 浏览、MCP 读写                                                                                                                                                                | 各工具 Sessions 子页、Droid 只读页                                                 |
| **P4 工具后端**    | 逐个移植 Codex/OpenCode/Claude/OpenClaw/Copilot/Droid 的 profile 命令（每个 ~10-20 命令），每移植一个即在 capabilities 开启并点亮下拉项                                                                    | 工具下拉完整 10 项                                                                 |
| **P5 CLI/TUI**     | service 层下沉（§3）、`cmd/dg` CLI（独立 `dg` 命名）、评估 bubbletea TUI 或复用/退役 Rust TUI                                                                                                              | headless 环境可用                                                                  |

每期收尾跑 `npm run check:all` + `wails3 build` + Go 测试；service 迁移期为 `internal/app` 保留转发方法避免 bindings 破坏。

## 6. 风险与对策

| 风险                                 | 对策                                                                                                                    |
| ------------------------------------ | ----------------------------------------------------------------------------------------------------------------------- |
| 共享组件悄悄引入未 shim 的 Tauri API | `capabilities.ts` 集中声明 + shim 缺命令时抛出带命令名的错误；CI 里跑一次 wails 构建的冒烟（现有 `smoke_test.go` 扩展） |
| service 迁移破坏现有 Wails spike     | 先"新包旧转发"，最后一步才删 `internal/app` 内实现；Go 测试全绿再动                                                     |
| PTY 事件风暴拖垮 webview             | 输出按 ~16ms 聚合再 emit；前端 xterm 写入节流已有                                                                       |
| Rust/Go 双 core 漂移                 | P4 起冻结 Rust core 新功能；用特征化测试（对同一输入文件断言相同输出）锁 Dsh/Hermes/OMP 行为                            |
| 工具下拉未就绪项的观感               | Disabled 占位 + tooltip「该工具的 Wails 支持即将推出」，每移植一个后端点亮一项（P4）                                    |

## 7. 已拍板的决策

1. **Rust TUI 去留**：P5 时再定——先不投入 TUI，Go service 层就绪后按 Wails GUI parity 进度决定 bubbletea 重写还是保留 Rust TUI。
2. **未就绪工具的展示**：**Disabled 占位**——保持截图的 10 项完整布局，未就绪项灰显 + tooltip「该工具的 Wails 支持即将推出」，P4 每移植一个工具后端即点亮一项。
3. **CLI 形态**：**独立 `dg` 命名**（`dg channels list`、`dg hermes apply` 等），与 GUI 操作一一对应，`--json` 输出供脚本/TUI 消费。
4. **Wails 版命名**：**保留 `DroidGear Minimal`**（窗口标题、About、二进制名不变），便于与 Tauri 版并存区分。
