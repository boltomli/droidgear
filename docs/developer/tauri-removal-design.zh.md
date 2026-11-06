# Tauri 剥离设计方案（Wails-only 收敛 + CLI/TUI 接棒）

- **状态**：设计稿（**决策已确认**：Q1-A 删除自动更新、Q2-A 现在启动 `dg tui`、Q3-B 立即停发 Rust TUI；本文件不含代码改动）
- **日期**：2026-11
- **适用分支**：`spike/wails-v3-minimal`
- **关联**：
  - [`adr-001-tui-fate.zh.md`](./adr-001-tui-fate.zh.md) — TUI/CLI 命运决策（路线 B：Go 重写）
  - [`wails-tui-cli-design.zh.md`](./wails-tui-cli-design.zh.md) — service 分层、`dg` 契约、Wails-only 边界（§0 规则 4 即本文件的立项依据）
  - [`../../wails/PORTING.md`](../../wails/PORTING.md) — GUI 移植契约与「CLI contract」
  - [`../agents/wails-port.md`](../agents/wails-port.md) — 面向 agent 的 Wails 速查
  - 任务分解：[`tauri-removal-tasks.zh.md`](./tauri-removal-tasks.zh.md)（**执行时只看那一份**）

---

## 0. 一页结论（TL;DR）

**终态**：仓库里只有一个 GUI 运行时（Wails v3 + Go）和两个非 GUI 消费者（`dg` CLI、`dg tui`）。
`src-tauri/`、Cargo 工具链、`@tauri-apps/*` / `tauri-pty` / `tauri-plugin-system-fonts-api`
依赖、Tauri 发布流水线全部消失；前端只有一套入口、一套 Vite 配置、一套门禁。

**路线图**（P0 立即执行；A 与 C 可并行）：

| 阶段 | 名称                     | 产物                                                                | 用户可见性          |
| ---- | ------------------------ | ------------------------------------------------------------------- | ------------------- |
| P0   | 停发 Rust TUI（Q3-B）    | `release.yml` 去掉 `droidgear-tui` 打包；README 差异表披露空窗      | 资产变更            |
| A    | 前端去壳（de-shim）      | `src/lib/shell/*` 取代 `@tauri-apps/*`；`src/wails/` 上提为唯一入口 | 不可见（行为不变）  |
| B    | 删除 Rust 工具链         | 删 `src-tauri/`、Cargo/just/tauri 脚本、CI 的 Tauri 发布项          | 不可见 + 发布项变更 |
| C    | 能力补齐（TUI/CLI 接棒） | service B2 剩余 → B3 → B4；`dg` 覆盖 T1；`dg tui`（P1 只读优先）    | 新增能力            |

**三个硬性不变量**（每步都必须成立）：

1. **126 个 `@/lib/bindings` import 点不动**：`src/lib/bindings.ts` 路径、`commands` 对象形状
   （229 个 snake_case 方法、`Result<T>` 信封）保持不变 —— 只换它内部的 `invoke` 实现。
2. **service 是唯一写盘路径**：删除 Tauri 不产生任何新的写盘实现；`internal/app` 仍只做转发。
3. **每阶段可独立合并、可回滚**：A 阶段结束时 Tauri 壳仍能构建（只是不再维护），
   B 阶段才真正删除；任何一步变红都能单独 revert。

**已定决策（2026-11，维护者确认）**：

- **Q1 = A**：删除 Tauri 自动更新（`services/updater.ts` + 两处 UI 开关），「检查更新」改为打开 Releases 页；
  Go 原生 updater 单列 backlog，不阻塞 B 阶段。
- **Q2 = A**：现在启动 Go TUI（`dg tui`，bubbletea + lipgloss），P1 只读 → P2 ChangeSet 预览 → P3 apply。
- **Q3 = B**：**立即**停发 `droidgear-tui` 发布资产（去掉 `release.yml` 的 4 个打包 step），
  能力空窗由 README 差异表披露，Go TUI 追平后撤表。

---

## 1. 现状盘点（`99d6e90` 实测）

### 1.1 三端现状

```
                 ┌──────────────────────────────┐
   Tauri 壳      │ src-tauri/ (Rust 117 files)  │──► src/main.tsx ──► App.tsx
   （冻结）      │  core / tui / plugin-pty     │        │
                 └──────────────────────────────┘        │  共享 src/ 353 个 ts/tsx
                                                          │  （60 个文件 import @tauri-apps/*）
                 ┌──────────────────────────────┐        │
   Wails 壳      │ wails/ (Go 117 files)        │──► src/wails/main.tsx ──► AppShell
   （唯一活跃）  │  core / service / app / cmd  │        ▲
                 └──────────────────────────────┘        │
                        │  @tauri-apps/* → src/wails/shims/* (Vite alias)
                        └────────────────────────────────┘
```

要点：**Wails 壳并没有摆脱 Tauri 的编程模型**。它靠 11 个 shim 文件把
`@tauri-apps/api/core`（`invoke`）、`plugin-dialog`、`plugin-os`… 逐个顶替成 Go 实现。
「剥离 Tauri」的实质工作，就是把这些 shim 从「别名替身」变成**一等公民模块**。

### 1.2 前端 Tauri 触面

| Tauri 说明符                               | 引用文件数¹ | Wails 侧现实现                      | 去 Tauri 后的归宿                    |
| ------------------------------------------ | ----------- | ----------------------------------- | ------------------------------------ |
| `@tauri-apps/api/core`（`invoke`）         | 1           | `shims/tauri-core.ts`（22.9K 枢纽） | `src/lib/shell/commands.ts`          |
| `@tauri-apps/api/event`                    | 14          | `shims/tauri-event.ts`              | `src/lib/shell/events.ts`            |
| `@tauri-apps/api/window` / `webviewWindow` | 8           | `shims/tauri-webview-window.ts`     | `src/lib/shell/window.ts`            |
| `@tauri-apps/plugin-dialog`                | 14          | `shims/tauri-dialog.ts`             | `src/lib/shell/dialog.ts`            |
| `@tauri-apps/plugin-clipboard-manager`     | 10          | `shims/tauri-clipboard-manager.ts`  | `src/lib/shell/clipboard.ts`         |
| `@tauri-apps/plugin-os`                    | 8           | `shims/tauri-os.ts`                 | `src/lib/shell/os.ts`                |
| `@tauri-apps/plugin-opener`                | 6           | `shims/tauri-opener.ts`             | `src/lib/shell/opener.ts`            |
| `@tauri-apps/plugin-fs`                    | 3           | `shims/tauri-fs.ts`                 | `src/lib/shell/fs.ts`                |
| `@tauri-apps/api/menu`                     | 5           | 无（原生菜单走 Go `menu.go`）       | `src/lib/shell/menu.ts` + 上下文菜单 |
| `@tauri-apps/plugin-updater`               | 4           | **无**（Wails 无更新器）            | 删除（§8-Q1）                        |
| `@tauri-apps/plugin-process`               | 3           | 无（`relaunch` 无等价）             | 删除                                 |
| `tauri-pty`                                | 5           | `shims/tauri-pty.ts`（Go PTY）      | `src/lib/shell/pty.ts`               |
| `tauri-plugin-system-fonts-api`            | 3           | `shims/tauri-system-fonts.ts`       | `src/lib/shell/fonts.ts`             |

¹ 含测试文件与 shim 自身；去重后 `src/` 下共 **60 个文件**（剔除 shim 为 52 个），
其中 **13 个是测试**（它们 `vi.mock('@tauri-apps/…')`，必须跟着改名）。

命令面是最大的一块，但也是**churn 最小的一块**：

| 项                                         | 数值                                                                      |
| ------------------------------------------ | ------------------------------------------------------------------------- |
| `src/lib/bindings.ts`（tauri-specta 产物） | 3,616 行 / 229 个命令 / `@ts-nocheck`                                     |
| 依赖它的文件                               | **126**（108 直接 + 18 经 `src/lib/tauri-bindings.ts`）                   |
| 真正的 Tauri 依赖点                        | 文件末尾 4 行 import（`invoke`、`Channel`、`events`、`WebviewWindow`）    |
| 已死的 Tauri 设施                          | `Channel`（0 用）、`events` 代理（0 用）、`__WebviewWindow__`（仅代理用） |

→ **结论**：`bindings.ts` 只需保留 `commands` 段与类型段，把 `TAURI_INVOKE` 换成
`shellInvoke`，并删掉末尾 50 行 tauri-specta 全局设施。126 个调用点零改动。

### 1.3 构建与工具链残留

| 位置           | 残留                                                                                                                                                                                                                                                              |
| -------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `package.json` | 15 个脚本：`tauri`、`tauri:dev`、`tauri:build`、`tauri:check`、`tauri:check:ts6`、`rust:fmt(:check)`、`rust:clippy(:fix)`、`rust:test`、`rust:bindings`、`tui`、`tui:dev`、`tui:build`、`tui:check`；`check:all` 内嵌 3 个 rust 步骤；`test:all` 内嵌 `rust:test` |
| 依赖           | 13 个 `@tauri-apps/*`（11 runtime + 2 plugin/fonts 类）+ `tauri-pty` + `tauri-plugin-system-fonts-api` + devDep `@tauri-apps/cli`                                                                                                                                 |
| 配置           | 根 `index.html`（Tauri 入口）、根 `vite.config.ts`（`TAURI_DEV_*`）、`rust-toolchain.toml`、`justfile`（TUI 交叉编译）、`scripts/tauri-dev.sh`、`scripts/tui-dev.sh`                                                                                              |
| `src/` 结构    | `src/main.tsx`（Tauri 入口）、`src/wails/{main.tsx,AppShell.tsx,menu-bridge.ts,theme-window.ts,error-i18n.ts,wails.css}`、`src/wails/shims/*`                                                                                                                     |
| `src-tauri/`   | 117 个非 target `.rs` / 67,570 行：`droidgear-core` 35,877、`droidgear-tui` 24,902、Tauri 壳 6,370、`tauri-plugin-pty` 411；另有 `capabilities/`、`gen/`、`icons/`、4 个 `tauri.*.conf.json`                                                                      |
| CI             | `release.yml`：`publish-tauri`（三端安装包 + 便携更新资产）、`droidgear-tui` 打包上传、`generate-updater-json`；`release-single.yml`：已是 3 OS 的 Wails + `dg` 单文件流水线；`wails.yml`：`build-test` + `cli-headless`                                          |
| 门禁           | `scripts/check-command-surface.mjs`：同时解析 Rust `invoke_handler` 与 Go bindings；`scripts/check-go-layering.mjs`：R1/R3                                                                                                                                        |
| 文档           | `docs/` 约 40 个文件提到 Tauri（其中 8 个是「Tauri 专属」文档：`tauri-commands.md`、`tauri-plugins.md`、`rust-architecture.md`、`code-style-rust.md`…）；`AGENTS.md` 规则 1/2/9；README 双语                                                                      |

### 1.4 Go 侧现状（决定 C 阶段工作量）

| 层                       | 规模                       | 备注                                                                      |
| ------------------------ | -------------------------- | ------------------------------------------------------------------------- |
| `wails/internal/app`     | 27,357 行 / 245 个导出方法 | 业务逻辑仍留在这里（B3/B4 的主战场）                                      |
| `wails/internal/core`    | 1,667 行                   | 路径、原子写、JSON5/YAML 往返、dry-run 录制                               |
| `wails/internal/service` | 8,788 行                   | 6 个域：`paths`、`channels`、`export`、`codex`、`hermes`、`pi`、`omp`     |
| `wails/cmd/dg`           | 2,965 行                   | `version` `doctor` `path` `channels` `codex` `hermes` `pi` `omp` `export` |
| `wails/internal/tui`     | **不存在**                 | Go TUI 尚未开工（ADR-001 期 3）                                           |

缺口（`wails-tui-cli-design.zh.md` §4.2 的 T1/T2 表）：`openclaw`、`opencode`（B2 暂缓）、
`claude`、`copilot`、`droid`、`dsh`、`factory`、`mcp`、`legacy_config`（B3）、
`sessions`/`specs`（B4 只读）、`--force` + `beforeHash` 冲突检测（exit 3）。

---

## 2. 目标与非目标

### 目标

| #   | 目标                                                                                   | 验收形态                                                                                         |
| --- | -------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| G1  | 仓库无 Tauri：`src-tauri/`、Cargo 工具链、`@tauri-apps/*`/`tauri-pty`/fonts 包全部删除 | `scripts/check-no-tauri.mjs` 绿 + `npm ls` 无 tauri 包 + `git ls-files src-tauri` 为空           |
| G2  | 单一前端入口与单一构建                                                                 | 根 `index.html` + `vite.config.ts` → `wails/frontend/dist` → `go:embed`；`wails/` 内无 vite 配置 |
| G3  | 命令面单一来源（Go）                                                                   | `dg` 与 GUI 共用 `internal/service`；`check-command-surface.mjs` 只校验 façade ↔ Go bindings     |
| G4  | 非 GUI 能力由 `dg`（+ `dg tui`）承担                                                   | T1 全覆盖或有 PORTING.md 豁免记录；`dg tui` 覆盖只读浏览 + apply 预览                            |
| G5  | GUI 功能无回归                                                                         | 现有 `src/**/*.test.tsx` 全绿（仅 mock 目标改名）；手工冒烟清单通过                              |
| G6  | 文档与规则同步                                                                         | `AGENTS.md` 规则 1/2/9 改写；README 双语；`docs/agents/*` 无 Tauri 操作指引                      |

### 非目标

- **不改 UI/交互**：A 阶段是纯 import 替换 + 入口合并，组件行为、样式、i18n 一律不动。
- **不重写 Rust 历史**：`droidgear-core` 只是不再编译；如需参考，从 git history 取。
- **不为 Tauri 修 bug**：任何 Rust/Go 行为不一致，一律以 Go 为准（延续 §0 冻结规则）。
- **不新增三方依赖**（除 §8-Q2 的 TUI 决策）：CLI 继续只用标准库。
- **不做 Wails 自动更新实现**（除非 §8-Q1 选 B）。

---

## 3. 目标架构

### 3.1 目录树（after）

```
package.json            # 只剩 Wails 脚本：dev / build / typecheck / lint / test / check:all
vite.config.ts          # 唯一前端构建（root=仓库根，outDir=wails/frontend/dist）
vitest.config.ts        # 别名同步（去掉 tauri-pty）
index.html              # 唯一入口 → /src/main.tsx
src/
├── main.tsx            # 唯一入口（原 src/wails/main.tsx + 原 src/main.tsx 的 theme 初始化）
├── App.tsx             # 唯一壳（原 src/wails/AppShell.tsx；原 Tauri App.tsx 删除）
├── shell.css           # 原 src/wails/wails.css
├── lib/
│   ├── bindings.ts     # 保留路径与 commands 形状，改由 shellInvoke 驱动（126 处不动）
│   ├── app-bindings.ts # 原 lib/tauri-bindings.ts（unwrapResult + 常用类型再导出）
│   └── shell/          # ◀ 新增：唯一 shell 抽象层（不出现 tauri 字样）
│       ├── index.ts        commands.ts    events.ts     window.ts
│       ├── dialog.ts       clipboard.ts   fs.ts         os.ts
│       ├── opener.ts       menu.ts        context-menu.ts
│       ├── pty.ts          fonts.ts       error-i18n.ts
│       ├── menu-bridge.ts  window-theme.ts
│       └── shell-env.ts?   （可选：并入 services/shell-env.ts）
└── services/           # 去掉 updater.ts（§8-Q1）
wails/
├── internal/{core,service,app,tui,version}
├── cmd/dg
└── frontend/dist       # 构建产物（.gitkeep 保留，go:embed 依赖目录存在）
```

### 3.2 依赖方向与门禁（after）

```
   src/lib/shell/*  ──► @wailsio/runtime + @wails-bindings（生成）
        ▲
        │  只有 shell/ 允许 import @wailsio/runtime 与 @wails-bindings
   src/lib/bindings.ts（façade）──► src/lib/shell/commands.ts
        ▲
   src/components/**、src/hooks/**、src/store/**（零 shell 细节）

   wails/cmd/dg ──► internal/service ──► internal/core      （禁止 import internal/app / wailsapp）
   wails/cmd/dg tui ──► internal/tui ──► internal/service   （禁止 import internal/app）
   wails/internal/app ──► internal/service                  （薄转发）
```

门禁（改造后）：

| 门禁                                        | 覆盖                                                                                                     |
| ------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `scripts/check-no-tauri.mjs`（新）          | `src/`、`package.json`、`vite.config.ts`、workflows 内禁止 `@tauri-apps`/`tauri-pty`/`src-tauri`/`cargo` |
| `scripts/check-command-surface.mjs`（收缩） | façade 表 ↔ 生成 bindings 的参数名与顺序；Go 方法全覆盖（routed/direct/baseline）                        |
| `scripts/check-go-layering.mjs`（扩展 R4）  | 原有 R1/R3 + `internal/tui` 不得 import `internal/app`                                                   |
| `wails.yml → cli-headless`（不变）          | `CGO_ENABLED=0 go build ./cmd/dg`（含 `internal/tui`）                                                   |
| `npm run check:all`（改造）                 | 去掉 rust 三步；加入 `check-no-tauri`；门禁全在前端测试前跑                                              |

### 3.3 模块映射表（旧 → 新）

| 旧路径                                                             | 新路径                                             | 迁移方式                                                                        |
| ------------------------------------------------------------------ | -------------------------------------------------- | ------------------------------------------------------------------------------- |
| `src/wails/main.tsx`                                               | `src/main.tsx`                                     | 合并（保留 `applyInitialTheme()`）                                              |
| `src/main.tsx`（Tauri）                                            | **删除**                                           | —                                                                               |
| `src/wails/AppShell.tsx`                                           | `src/App.tsx`                                      | 改 import；删 `isToolSupported` 重定向（无禁用工具）                            |
| `src/wails/wails.css`                                              | `src/shell.css`                                    | 原样搬                                                                          |
| `src/wails/menu-bridge.ts`                                         | `src/lib/shell/menu-bridge.ts`                     | 原样搬                                                                          |
| `src/wails/theme-window.ts`                                        | `src/lib/shell/window-theme.ts`                    | 原样搬                                                                          |
| `src/wails/error-i18n.ts`                                          | `src/lib/shell/error-i18n.ts`                      | 原样搬                                                                          |
| `src/wails/shims/tauri-core.ts`                                    | `src/lib/shell/commands.ts`                        | 保留 COMMANDS 表；`invoke`→`invokeGo`                                           |
| `src/wails/shims/tauri-event.ts`                                   | `src/lib/shell/events.ts`                          | 导出 `listen/once/emit`（Wails runtime events）                                 |
| `src/wails/shims/tauri-webview-window.ts`                          | `src/lib/shell/window.ts`                          | 导出 `getCurrentWindow()` 等壳内 API                                            |
| `src/wails/shims/tauri-{dialog,fs,clipboard-manager,os,opener}.ts` | `src/lib/shell/{dialog,fs,clipboard,os,opener}.ts` | 原样搬 + 改文件名                                                               |
| `src/wails/shims/tauri-pty.ts`                                     | `src/lib/shell/pty.ts`                             | 导出 `spawn()` + `IPty`（Go PTY）                                               |
| `src/wails/shims/tauri-system-fonts.ts`                            | `src/lib/shell/fonts.ts`                           | 导出 `getSystemFonts()`                                                         |
| `src/lib/menu.ts`（Tauri 原生菜单）                                | **删除**                                           | Wails 用 `internal/app/menu.go` + `menu-bridge.ts`                              |
| `src/lib/context-menu.ts`                                          | `src/lib/shell/context-menu.ts`                    | 改成 Wails 的上下文菜单实现或 Radix 回退（见任务书）                            |
| `src/services/updater.ts` + `updater.test.ts`                      | **删除**（§8-Q1）                                  | 检查更新改为「打开 Releases 页」                                                |
| `src/lib/capabilities.ts`                                          | 简化                                               | 删 `IS_WAILS_BACKEND`/`WAILS_TOOLS`；`isUpdaterSupported()` 随 updater 一起消失 |
| `wails/vite.config.ts`                                             | 根 `vite.config.ts`                                | 合并；`wails/build/Taskfile.yml` 去掉 `--config`                                |
| `wails/index.html`                                                 | 根 `index.html`                                    | 合并（保留 Tauri 版的 loader CSS 注释）                                         |

---

## 4. 关键决策

### D1 前端抽象落地形态 —— 选 A

| 选项     | 做法                                                                                      | 代价                                                                  |
| -------- | ----------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| **A ✅** | 新建 `src/lib/shell/*`，把 52 个非 shim 文件的 import 改成 `@/lib/shell/*`，删依赖与 shim | 一次性 ~60 文件机械替换；vite/vitest 别名表扩张                       |
| B        | 保留 `@tauri-apps/*` 说明符，vite alias 指向新模块                                        | import 名与实现不符；依赖树无法收缩（`npm ls` 仍红）；knip 报未用依赖 |
| C        | 组件直接调 `@wailsio/runtime`                                                             | 组件层被运行时绑死，测试与可移植性最差                                |

**结论 A**。`src/lib/shell/` 名下的每个模块导出**语义化函数**（`openFileDialog`、`showMessage`、
`listen`、`spawnPty`），不再模仿 Tauri 的类/方法签名 —— 但为了减少 churn，**保留同名函数与
参数形状**（例如 `listen(event, handler) => Promise<UnlistenFn>`），组件里的
`await listen('codex-sessions-changed', …)` 可以只改 import 一行。

### D2 命令 façade：先迁移，后（可选）生成 —— 选 A

| 选项     | 做法                                                                                                 | 代价                                    |
| -------- | ---------------------------------------------------------------------------------------------------- | --------------------------------------- |
| **A ✅** | `commands` 对象 + `COMMANDS` 表（229 条）搬进 `src/lib/shell/commands.ts`；`TAURI_INVOKE`→`invokeGo` | 手写表继续存在，靠门禁防漂移            |
| B        | 由 `wails3 generate bindings -ts` 产物生成映射表（`scripts/gen-command-map.mjs`）                    | 需要额外生成脚本 + 与 go:embed 顺序耦合 |
| C        | 126 个调用点全部改成生成的 PascalCase 绑定                                                           | 巨大 churn，收益只是「类型更真」        |

**结论 A**，B 作为后续优化（不阻塞去 Tauri）。理由：A 把 churn 压到 0，且
`check-command-surface.mjs`（收缩后）已经能捕获「表 ↔ 生成 bindings」的漂移；
B 的价值在于消灭手写表，等 A 落地后再评估。

### D3 自动更新 —— 选 A（删除），Go updater 单列 backlog

事实：`@tauri-apps/plugin-updater` 是 Tauri 独有；Wails 构建里 `isUpdaterSupported()===false`，
相关 UI 已隐藏；`release.yml` 的 `latest.json` 由 `publish-tauri` 的产物喂。

| 选项     | 做法                                                                                                               | 代价                                          |
| -------- | ------------------------------------------------------------------------------------------------------------------ | --------------------------------------------- |
| **A ✅** | 删除 `services/updater.ts` 与两处 UI 开关；「检查更新」改为打开 Releases 页（`openUrl` 已有）；README 说明手动更新 | 自动更新能力短期消失，需在 release notes 声明 |
| B        | 先做 Go 原生 updater（下载 → 校验 → 替换 → 三平台自替换），再删 Tauri                                              | 阻塞 B 阶段；三平台自替换是独立大工程         |
| C        | 保留 `latest.json` 资产但不消费                                                                                    | 无意义，且误导用户                            |

**结论 A**。若社区需要，Go updater 作为 B 阶段之后的独立任务（可复用 `GetAppVersion`
与 `internal/core` 的下载/原子替换）。**注意：若选 B，B 阶段要等到 updater 完成**。

> **2026-11 补充**：该 backlog 已落地。新更新器只针对单文件发布线（`wails-v*`
>
> - `SHA256SUMS.txt`），走 `internal/service/updater`，GUI 与 `dg update` 共用；
>   设计与限制见 [`self-update-single-file.zh.md`](./self-update-single-file.zh.md)。
>   本文其余部分保留为当时的决策记录。

### D4 TUI 形态 —— 选 A（`dg tui`，bubbletea），分期只读优先

| 选项     | 做法                                                                                             | 代价                                                      |
| -------- | ------------------------------------------------------------------------------------------------ | --------------------------------------------------------- |
| **A ✅** | 同一二进制 `dg tui` 子命令，`wails/internal/tui`，bubbletea + lipgloss；复用 service + ChangeSet | 引入 2 个三方依赖（需在 PORTING.md 记录放行）；二进制变大 |
| B        | 独立 `dgtui` 二进制                                                                              | 两套构建/发布/测试入口                                    |
| C        | 不做 TUI，`dg` 覆盖 T1/T2 后显式豁免                                                             | 交互式体验归零，仅靠 CLI                                  |

**结论 A，分期**：T1 只读浏览（模块列表 → 详情）→ T2 预览（ChangeSet diff）→ T3 apply/删除。
`droidgear-tui` 的 24,902 行不要求 1:1 复刻：先覆盖 10 个工具的 profile CRUD + apply 预览，
Sessions/Specs 只读列表按 B4 的 service 就绪度追加。

### D5 删除顺序 —— A → B → C，且 B 的前置是 A

- **A（前端去壳）必须先做**：B 删掉 `src-tauri/` 后，根 `vite.config.ts`、`index.html`、
  `src/main.tsx` 都必须是 Wails 版；否则 GUI 无法构建。
- **C（能力补齐）可与 A 并行**：两者改的文件集不重叠（`src/` vs `wails/internal`）。
- **Rust TUI 立即停发（Q3-B）**：不再等待 `dg tui`，因此 B 阶段不被 TUI 阻断；
  代价是空窗期必须在 README/Release notes 显式披露，且 `dg tui` 的 P1 只读要在两个发布周期内交付。

### D6 门禁重写 —— 收缩 + 新增

- `check-command-surface.mjs`：删掉 Rust `invoke_handler` 解析与 `RUST_ONLY`/dead-command baseline，
  保留「COMMANDS 表 ↔ 生成 bindings 参数名/顺序」与「Go 方法全覆盖」两条。
- 新增 `check-no-tauri.mjs`：把「不再引入 Tauri」从约定变成 CI 红绿灯（用 git ls-files 而非
  `grep -r`，避免扫 node_modules/target）。
- `check-go-layering.mjs`：新增 R4（`internal/tui` 不得 import `internal/app`），
  R1/R3 保留 —— 它们是 `cli-headless` job 的语义说明。

---

## 5. 分阶段路线图

```
P0（立即，1 个 PR）
  P0-1 release.yml 去掉 droidgear-tui 打包 + README 差异表披露
阶段 A（前端去壳，~8 个 PR）       阶段 C（能力补齐，可持续并行）
  A1 门禁与基线                        C0 通用前置：conflict detection（--force / exit 3）
  A2 shell/commands（枢纽）            C1 service: openclaw
  A3 事件/窗口/OS/opener               C2 service: opencode + dg 子命令
  A4 dialog/fs/clipboard               C3 service: claude / copilot
  A5 pty/字体/上下文菜单               C4 service: droid / dsh / factory / mcp
  A6 入口合并 + vite 收敛              C5 B4 只读：sessions / specs（+ dg）
  A7 测试 mock 迁移                    C6 Go TUI（bubbletea）：只读 → 预览 → apply
  A8 依赖与脚本清理（可延后到 B1）
阶段 B（删除 Rust 工具链）
  B1 package.json / scripts / just / toolchain
  B2 删除 src-tauri/
  B3 CI：release.yml 收编 + wails.yml 改名（TUI 打包已在 P0 移除）
  B4 文档与 AGENTS 改写
```

**里程碑判据**：

- **M1 = A 完成**：`npm ci && npm run dev` 起 Wails 前端；`npm test` 全绿；
  `grep -r "@tauri-apps" src --include=*.ts*` 为空；Tauri 壳仍能构建（但已无脚本入口）。
- **M2 = B 完成**：`src-tauri/` 删除；`npm run check:all` 不含 rust 步骤且零错误；
  CI 只剩 Wails 流水线；`scripts/check-no-tauri.mjs` 接入。
- **M3 = C 完成（T1 全覆盖）**：`dg` 覆盖 10 工具 + paths/channels/export/mcp/sessions/specs；
  `dg --dry-run` 对所有写操作生效；`dg tui` 至少完成只读 + 预览。
- **M4 = Rust TUI 停发（已提前到 P0）**：`release.yml` 无 `droidgear-tui` 打包；
  README 差异表列出空窗能力，`dg tui` P1 交付后逐步销项。

---

## 6. 验收与门禁

| 阶段 | 命令                                                                 | 期望                                                                             |
| ---- | -------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| A    | `npm run typecheck && npm run lint && npm run test:run`              | 零错误                                                                           |
| A    | `npx vite build`（根配置）                                           | `wails/frontend/dist` 产出，Tailwind guard 通过（`.h-screen`、`w-[280px]` 存在） |
| A    | `node scripts/check-no-tauri.mjs`                                    | 绿（src 内无 tauri 说明符）                                                      |
| B    | `node scripts/check-command-surface.mjs`                             | 绿（façade ↔ Go bindings）                                                       |
| B    | `cd wails && go vet ./... && go test ./...`                          | 绿                                                                               |
| B    | `CGO_ENABLED=0 go build ./cmd/dg`（linux 容器或 `cli-headless` job） | 绿                                                                               |
| B    | `npm run check:all`                                                  | 绿（无 rust 步骤）                                                               |
| C    | `go test ./cmd/dg ./internal/service/...`                            | 每个域至少一条断言落盘内容的用例                                                 |
| C    | `dg doctor --json`、`dg <tool> profiles apply <id> --dry-run --json` | golden JSON                                                                      |

**手工冒烟清单（A/B 每次发布前）**：启动 → 切换 10 个工具页 → 打开/保存 Preferences →
Channels 增删改 → 导出模板运行 → 终端页签（PTY）→ Sessions 页 → 语言切换（含菜单）→
窗口控制（最小化/最大化/双击标题栏）→ 上下文菜单。

---

## 7. 风险、回滚与用户可见影响

| 风险                                                           | 影响 | 缓解                                                                                               |
| -------------------------------------------------------------- | ---- | -------------------------------------------------------------------------------------------------- |
| shim 语义丢失（错误字符串、事件名、监听泄漏）                  | 中   | `error-i18n.ts`、`toRawError` 原样搬；每周冒烟清单跑事件相关页（Sessions/Codex）                   |
| `@wails-bindings` 别名在根 vite 配置里失效                     | 高   | A2 单独一个 PR 并跑 `vite build`；CI 的 `build-test` 覆盖率兜底                                    |
| Tailwind v4 `@source` 回归（Vite root 从 `wails/` 变成仓库根） | 高   | `src/App.css` 保留 `@source './**/*.{ts,tsx}'`；CI 的 Tailwind guard 必须继续存在                  |
| 删除 updater 引起用户抱怨                                      | 中   | release notes 声明 + 「检查更新」跳转 Releases；Go updater 单列 backlog                            |
| Rust TUI 停发后能力空窗（Q3-B 立即停发）                       | 中   | README + Release notes 差异表逐项披露；`dg tui` P1 两个发布周期内交付；`dg` 已覆盖的工具不构成空窗 |
| 发布流水线合并出错（release.yml vs release-single.yml）        | 高   | B3 先在 PR 里 `workflow_dispatch` 空跑；保留 `release-single.yml` 为唯一 3 OS 出口                 |
| 门禁脚本误伤历史文档                                           | 低   | `check-no-tauri` 只扫 `src/`、`package.json`、`vite.config.ts`、`.github/workflows`                |

**回滚策略**：A 阶段每个 PR 独立 revert 即可（无数据迁移）；B 阶段删除 `src-tauri/`
是纯删除，`git revert` 可恢复；**唯一不可逆的是发布项**（P0 已批准的 `droidgear-tui` 停发），
停发前在 release notes 声明，并把最后一份带 TUI 的版本 tag 记入 README（如 `v1.3.9`）。

---

## 8. 决策记录（已定）

> 2026-11 维护者确认。后续若变更，在本表追加一行并注明日期与理由，不要静默改代码。

| #   | 问题                | 决定            | 后果 / 约束                                                                                                       |
| --- | ------------------- | --------------- | ----------------------------------------------------------------------------------------------------------------- |
| Q1  | 自动更新怎么处理    | **A**（已确认） | 删除 `services/updater.ts` 与两处 UI 开关；「检查更新」跳转 Releases；Go updater 单列 backlog；release notes 声明 |
| Q2  | Go TUI 是否现在启动 | **A**（已确认） | `wails/internal/tui` + bubbletea/lipgloss 放行；P1 只读优先；R4 分层门禁同步                                      |
| Q3  | Rust TUI 何时停发   | **B**（已确认） | P0 立即移除 `release.yml` 的 4 个 TUI 打包 step；README 差异表披露空窗；`dg tui` P1 需在两个发布周期内交付        |

> 未选项备查：Q1-B 先做 Go 原生 updater（三平台自替换，会把 B 阶段推后数周）；
> Q2-C 不做 TUI 并走 ADR-001 显式豁免；Q3-A 等 `dg tui` 可用后再停发；Q3-C 永不停发（与 G1 冲突）。

---

## 9. 术语

| 术语            | 含义                                                                                            |
| --------------- | ----------------------------------------------------------------------------------------------- |
| 去壳（de-shim） | 用 `src/lib/shell/*` 取代 `src/wails/shims/*` 与 `@tauri-apps/*` 说明符                         |
| façade          | `src/lib/bindings.ts` 的 `commands` 对象：snake_case 命令 → Go 方法                             |
| 生成 bindings   | `wails3 generate bindings -clean=true -ts -i` 产出的 `wails/frontend/bindings/**`（gitignored） |
| T1 / T2 / T3    | 写操作 / 只读 / GUI 专属（见 `wails-tui-cli-design.zh.md` §4.2）                                |
| ChangeSet       | Plan → Preview → Apply 的统一结果（`applied/changes/conflicts`）                                |
