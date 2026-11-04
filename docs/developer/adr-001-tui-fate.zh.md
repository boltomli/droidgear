# ADR-001：TUI/CLI 命运 —— Go 重写 TUI（冻结 Rust 实现）

- **状态**：已接受
- **日期**：2026-11
- **决策人**：维护者
- **关联**：[`wails-port-guidelines.zh.md`](./wails-port-guidelines.zh.md) §5、[`wails-round2-layout-and-gaps.zh.md`](./wails-round2-layout-and-gaps.zh.md) B11、[`wails-tui-cli-design.zh.md`](./wails-tui-cli-design.zh.md)（**执行细则**）、[`tauri-removal-design.zh.md`](./tauri-removal-design.zh.md) + [`tauri-removal-tasks.zh.md`](./tauri-removal-tasks.zh.md)（期 4「删除 `src-tauri`」的设计与任务书）、[`tui-parity.zh.md`](./tui-parity.zh.md)（TUI 与 GUI 对齐、成为默认终端体验并退役 `dg`）、AGENTS.md 规则 9、[`wails/PORTING.md`](../../wails/PORTING.md)「CLI 契约」

## 背景

Wails 移植后仓库里存在两条平行的 core：

```
Rust: droidgear-core ──► src-tauri（Tauri commands）──► React GUI（Tauri 壳）
                     └─► droidgear-tui（ratatui TUI ≈24.9k 行 + 少量 CLI 子命令）
Go:   internal/core  ──► internal/app（Wails service）──► React GUI（Wails 壳，同一份前端）
```

事实（2026-11 核对）：

| 端          | 现状                                                                                                                     |
| ----------- | ------------------------------------------------------------------------------------------------------------------------ |
| TUI（Rust） | 覆盖 10 工具 + Sessions/Specs/MCP/Paths/Channels；**`Export Templates` 0 处引用**（GUI 已有，AGENTS.md 规则 9 实质已破） |
| CLI（Rust） | 仅 `droidgear-tui run codex\|copilot\|claude\|droid --list`（临时运行），无配置类 headless 命令                          |
| CLI（Go）   | 不存在（无 `cmd/`）                                                                                                      |
| service 层  | 不存在——业务逻辑直接写在 `wails/internal/app/*.go`，App 方法自行拼装写盘                                                 |

三条路线的代价：

| 路线                  | 代价                                                              | 规则 9         |
| --------------------- | ----------------------------------------------------------------- | -------------- |
| A. 冻结 Rust TUI      | 零成本，但 TUI 永久落后、headless 只有「临时运行」                | 无法成立       |
| B. **Go 重写 TUI**    | 抽 `internal/service` + bubbletea 重写 ≈7k 行等量工程，分多期落地 | **可真正成立** |
| C. Rust 回归唯一 core | 与「Go 功能先行」方向相反，需回滚大量已完成移植                   | 成立但方向倒退 |

## 决策

**采用路线 B：TUI 用 Go 重写（bubbletea），与 GUI 共享同一套 `internal/service`。**

配套约束：

1. **过渡期冻结 Rust TUI 的新功能**：`droidgear-tui` 只修 bug，不再接受功能增量；
   README 的 TUI 覆盖清单标注「以 Go 版重写落地为准」。
2. **新功能先落 service 层**（`wails/internal/app` 业务逻辑下沉到 `internal/service`），
   GUI 与 CLI/TUI 都是薄适配层——这是路线 B 能兑现规则 9 的前提。
3. **`cmd/dg` CLI 先行**：`dg <tool> <action> --json` 与 GUI 操作一一对应，
   用最低成本先解决 SSH/容器的 headless 脚本需求，并作为 service 层的验收载体；
   交互式 Go TUI 在 CLI 稳定后启动。
4. **Rust 侧只保留** `droidgear-core`（Tauri 壳仍依赖）与过渡期的 `droidgear-tui`
   （发布流水线继续构建，直到 Go TUI 功能追平后移除）。

## 后果

- **正面**：单 core、单实现；规则 9「GUI/TUI parity」恢复可执行；headless 有
  `dg --json` 可脚本化；CLI/TUI/GUI 三端共享 ChangeSet（Plan → Preview → Apply）。
- **负面/风险**：约 7k 行交互式 TUI 的重写工程量（分多期，见下）；过渡期内
  Rust TUI 与 Go GUI 的功能差会暂时扩大（以 README 覆盖差异表披露）；
  `droidgear-core` 在 Go TUI 追平前不能删除。
- **对 AGENTS.md 规则 9 的改写**：见规则 9 现行文本——GUI（Tauri+Wails）必须同步，
  非 GUI 消费者在追平前豁免并披露。

## 分期（不承诺日期，只定顺序）

| 期  | 内容                                                                                 | 验收                                      |
| --- | ------------------------------------------------------------------------------------ | ----------------------------------------- |
| 1   | 抽 `wails/internal/service`（app 层只做参数编组，不改前端契约）                      | `check:all` 零错误，app 行为不变          |
| 2   | `cmd/dg` CLI（`--json`，覆盖 channels/dsh/pi/hermes/omp/codex 的 list/apply/export） | 每个子命令有 golden 测试；README 记录用法 |
| 3   | Go TUI（bubbletea）按工具分批迁移，Rust TUI 同步标注差异                             | 迁移完一个工具即从差异表移除一个          |
| 4   | Go TUI 功能追平后，移除 `droidgear-tui` 与 Rust TUI 发布项                           | README/发布流水线无 Rust TUI 引用         |

### 执行细则与顺序调整

具体分层、`dg` 契约与门禁见 [`wails-tui-cli-design.zh.md`](./wails-tui-cli-design.zh.md)。
相对本 ADR 原文的两处调整（不改变决策本体）：

1. **TUI 延后到显式决策点**：先做完期 1（service）与期 2–3（`cmd/dg`），
   `bubbletea`/`cobra` 等三方依赖在启动 TUI 时再放行，避免为未确定的 UI 先背依赖。
2. **本分支（`spike/wails-v3-minimal`）的 Tauri 壳与 Rust TUI 一律冻结**：
   不加命令/插件/shim，新能力只进 Go service；彻底删除 `src-tauri` 作为独立任务
   （含 `package.json` 的 `tauri:*`/`rust:*`/`tui:*` 脚本与 `release.yml` 的 Tauri/TUI 发布项）。
   该独立任务的设计与任务书：[`tauri-removal-design.zh.md`](./tauri-removal-design.zh.md)、
   [`tauri-removal-tasks.zh.md`](./tauri-removal-tasks.zh.md)。

## 未决问题

- bubbletea/lipgloss 是否引入为 `wails/go.mod` 依赖（PORTING.md 目前禁止新增三方依赖，
  届时需显式放行并记录）。
- Rust `droidgear-tui` 的最终移除时机取决于第 3 期进度，不在本 ADR 内锁定。
