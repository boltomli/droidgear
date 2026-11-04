# Tauri 剥离开发任务书（执行清单）

> 配套设计：[`tauri-removal-design.zh.md`](./tauri-removal-design.zh.md)（决策、架构、风险）。
> 本文件只写**怎么干**：任务、改动文件、验证命令、完成判据。
> 契约来源仍是 [`wails-tui-cli-design.zh.md`](./wails-tui-cli-design.zh.md) 与
> [`../../wails/PORTING.md`](../../wails/PORTING.md)，冲突时以其为准。

- **状态**：待评审 → 待执行
- **适用分支**：`spike/wails-v3-minimal`
- **总原则**：一个任务 = 一个可合并 PR；每个 PR 独立可 revert；**不夹带行为改动**。

---

## 0. 开工前（Day 0）

### 0.1 三项决策（已确认，2026-11，设计文件 §8）

- [x] **Q1 = A**：删除 Tauri 自动更新（`services/updater.ts` + 两处 UI 开关），「检查更新」跳转 Releases
- [x] **Q2 = A**：现在启动 Go TUI（`dg tui`，bubbletea + lipgloss），P1 只读优先
- [x] **Q3 = B**：**立即**停发 `droidgear-tui` 发布资产（P0 执行）

派生约束：`release.yml` 的 TUI 打包移到 **P0**（不等删除 `src-tauri`）；
README 必须同步披露空窗能力；`dg tui` P1 需在两个发布周期内交付。

### 0.2 建立基线（每个阶段开始前跑一次，结果贴 PR 描述）

```bash
git checkout spike/wails-v3-minimal && git pull
npm ci
cd wails && go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0 && cd ..
cd wails && wails3 generate bindings -clean=true -ts -i && cd ..   # gitignored，必须先跑
npx vite build --config wails/vite.config.ts --mode production
cd wails && go vet ./... && go test ./... && cd ..
npm run typecheck && npm run lint && npm run test:run
node scripts/check-command-surface.mjs && node scripts/check-go-layering.mjs
```

基线全绿才动工。**任何一步在基线就红**，先单独修，不混入本计划。

### 0.3 约定

- 提交信息走 conventional commits（`refactor: de-shim events`、`chore: drop tauri deps`）。
- 一个 PR 只做一个任务；例外见 A6（入口合并必须与 vite 收敛同 PR，否则构建必红）。
- 机械替换（import 改名）与语义改动**分开提交**，便于 review 与 revert。
- 每个 PR 的验证输出贴到描述里；不贴视为未完成。
- 不新增三方依赖（除 Q2-A 的 bubbletea/lipgloss，需同时更新 `PORTING.md` 豁免表）。

---

---

## 阶段 P0：立即停发 Rust TUI（Q3-B，1 个 PR）

> 与 A/B/C 无依赖，可最先合并。目的：把「发布项」这条不可逆动作与源码删除解耦。

### P0-1 — `release.yml` 移除 `droidgear-tui`

- [ ] 删除 `publish-tauri` 里与 TUI 相关的 4 个 step（`Build droidgear-tui`、
      `Package droidgear-tui (tar.gz)`、`Package droidgear-tui (zip)`、`Upload droidgear-tui`）；
      **保留** Tauri 自身的安装包构建（B3 再整段删除）
- [ ] `justfile` 标记为「历史 TUI 交叉编译」，或直接删除（与 B1 合并亦可）
- [ ] `README.md` / `README_EN.md`：TUI 安装章节改为「TUI 正在迁移到 `dg tui`」，
      并列出空窗能力（Sessions/Specs/MCP 只读、批量 apply 预览等）
- [ ] `CHANGELOG.md` 记一条：停止发布 `droidgear-tui`，改由 `dg` / `dg tui` 承担
- [ ] 在 release notes 模板里加一行说明与最后可用版本号（当前为 `v1.3.9`）
- **验证**：PR 触发的 CI 绿；`workflow_dispatch` 空跑一次发布流水线，资产清单无 TUI 条目。
- **完成判据**：`.github/` 内只剩说明性文本提到 `droidgear-tui`；README 差异表存在且可读。
- **回滚**：单 PR revert（会把 TUI 打包加回来，源码仍在，可重新发布）。

---

## 阶段 A：前端去壳（de-shim）

> 目标：`src/` 里不再出现 `@tauri-apps/*`；`src/wails/` 上提；GUI 行为零变化。
> 依赖：无（可立即开始）。风险最高的两处：`commands` 枢纽、Vite 根与 Tailwind。

### A1 — 门禁与基线（先行，防止回填）

- [ ] 新增 `scripts/check-no-tauri.mjs`
  - 扫描范围：`src/**/*.{ts,tsx}`、`package.json`、`vite.config.ts`、`vitest.config.ts`、
    `.github/workflows/*.yml`、`justfile`、`rust-toolchain.toml`、`scripts/*.{sh,js,mjs}`
  - 禁止模式：`@tauri-apps/`、`tauri-pty`、`tauri-plugin-system-fonts-api`、`src-tauri`、
    `cargo `、`droidgear-tui`、`tauri-specta`
  - 允许：`docs/**`（历史说明）、`wails/PORTING.md`（契约历史）、本计划的两个文件
  - **分阶段开关**：A 阶段只扫 `src/`（shim 与 `src/wails/` 白名单），
    B 阶段去掉白名单 + 扩到 `package.json` 等
  - 用 `git ls-files` 取文件列表，避免扫 `node_modules`/`target`
- [ ] 接入 `package.json`：`"check:no-tauri": "node scripts/check-no-tauri.mjs"`，
      并把 `check:all` 的前端门禁段落改为先跑它
- [ ] 更新 `docs/agents/commands.md` 的验证命令清单
- **验证**：

  ```bash
  node scripts/check-no-tauri.mjs          # A 阶段应绿（src 白名单含 shims）
  npm run lint && npm run format:check
  ```

- **完成判据**：脚本存在、CI 里会跑、对故意插入的一行 `import '@tauri-apps/api/core'`
  能报错（本地试一次再删）。

### A2 — `commands` 枢纽（`src/lib/shell/commands.ts`）

> 这是整个去 Tauri 的**唯一高风险点**。126 个调用点不动，只换内部实现。

- [ ] 新建 `src/lib/shell/commands.ts`
  - 从 `src/wails/shims/tauri-core.ts` 整体搬入 `COMMANDS` 表（229 条 `{fn, args}`）
  - 导出 `invokeGo(cmd: string, args?: Record<string, unknown>): Promise<unknown>`
  - 保留错误规范化：Wails `RuntimeError`（`.cause` / `.message`）→ 裸字符串
  - 保留 `localizeWailsError`（来自 `src/lib/shell/error-i18n.ts`，A2 里先一并搬）
  - 保留未知命令的错误信息格式（供测试断言）
- [ ] 改 `src/lib/bindings.ts` 末尾：
  - 删除 `import { invoke as TAURI_INVOKE, Channel as TAURI_CHANNEL } from '@tauri-apps/api/core'`
  - 改为 `import { invokeGo } from './shell/commands'`，把 `await TAURI_INVOKE(` 批量替换为
    `await invokeGo(`（229 处，`sed` 可做）
  - 删除 `TAURI_CHANNEL`（0 使用）、`TAURI_API_EVENT` + `__EventObj__` + 导出的 `events`
    （0 使用）、`@tauri-apps/api/webviewWindow` import（为 `events` 而存在）
  - 更新文件头注释：不再是 tauri-specta 产物，而是「Go 命令面 façade（形状固定）」
- [ ] `src/lib/tauri-bindings.ts` → `src/lib/app-bindings.ts`，更新 18 个 import 点
- [ ] `src/wails/shims/bindings-module.d.ts` 挪到 `src/lib/shell/bindings-module.d.ts`
- **验证**：

  ```bash
  npm run typecheck && npm run test:run
  npx vite build --config wails/vite.config.ts --mode production
  grep -c "TAURI_INVOKE" src/lib/bindings.ts   # 期望 0
  # A2 阶段该脚本仍会读 src-tauri（Rust 侧校验）；先只依赖 typecheck + vite build
  ```

- **完成判据**：`bindings.ts` 无 tauri 字样；`commands.*` 全部经 `invokeGo`；前端测试全绿。
- **回滚**：单 PR revert（无数据/协议变化）。

### A3 — 事件 / 窗口 / OS / opener

- [ ] `src/lib/shell/events.ts` ← `shims/tauri-event.ts`（导出 `listen/once/emit`，语义不变）
- [ ] `src/lib/shell/window.ts` ← `shims/tauri-webview-window.ts`
- [ ] `src/lib/shell/os.ts` ← `shims/tauri-os.ts`
- [ ] `src/lib/shell/opener.ts` ← `shims/tauri-opener.ts`
- [ ] 改 import（机械替换，逐组替换后立刻跑测试）：

  | 组     | 文件（去 `src/` 前缀）                                                                                                                                                                                                                                                                                                              |
  | ------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
  | events | `components/ThemeProvider.tsx`、`components/codex/CodexSessionsPage.{tsx,test.tsx}`、`components/droid/SessionsPage.tsx`、`components/droid/SpecsPage.tsx`、`components/dsh/DshSessionsPage.{tsx,test.tsx}`、`components/omp/OmpSessionsPage.{tsx,test.tsx}`、`components/pi/PiSessionsPage.{tsx,test.tsx}`、`wails/menu-bridge.ts` |
  | window | `App.tsx`（A6 删除）、`components/layout/MainWindow.tsx`、`components/titlebar/MacOSWindowControls.tsx`、`components/titlebar/WindowsWindowControls.tsx`、`components/titlebar/titlebar-double-click.ts`、`lib/commands/window-commands.ts`                                                                                         |
  | os     | `components/droid/LegacyVersionsPage.tsx`、`components/droid/TerminalView.{tsx,test.tsx}`、`components/preferences/panes/AppearancePane.tsx`、`hooks/use-platform.{ts,test.ts}`、`i18n/language-init.ts`                                                                                                                            |
  | opener | `components/droid/LegacyVersionsPage.tsx`、`components/droid/McpPage.tsx`、`components/export/ExportTemplatesPage.tsx`、`components/preferences/panes/AboutPane.tsx`                                                                                                                                                                |

- [ ] `src/lib/commands/window-commands.ts`：把 `relaunch()`（`plugin-process`）改为
      「提示重启」或删除该命令项（Q1 决定后定），`exit()` 走 `shell/window.ts` 的关闭 API
- **验证**：`npm run test:run`；冒烟：Sessions 页事件刷新（切工具→保存 profile→列表更新）。
- **完成判据**：4 个 shim 文件仅被 `src/lib/shell/*` 引用（即将搬走）。

### A4 — dialog / fs / clipboard

- [ ] `src/lib/shell/{dialog,fs,clipboard}.ts` ← 对应 shim
- [ ] 改 import 表：

  | 组        | 文件                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
  | --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
  | dialog    | `components/channels/ChannelList.tsx`、`components/claude/ClaudeFeatureList.tsx`、`components/codex/CodexConfigPage.{tsx,test.tsx}`、`components/copilot/CopilotConfigPage.{tsx,test.tsx}`、`components/droid/DroidFeatureList.{tsx,test.tsx}`、`components/droid/SpecsPage.tsx`、`components/droid/TerminalPage.tsx`、`components/droid/TrustedFoldersPage.{tsx,test.tsx}`、`components/models/ModelConfigPage.tsx`、`components/preferences/panes/PathsPane.tsx` |
  | fs        | `components/channels/ChannelList.tsx`、`components/droid/SpecsPage.tsx`、`components/models/ModelConfigPage.tsx`                                                                                                                                                                                                                                                                                                                                                   |
  | clipboard | `components/claude/ClaudeFeatureList.tsx`、`components/droid/DroidFeatureList.{tsx,test.tsx}`、`components/droid/DroidSettingsPage.tsx`、`components/droid/SpecsPage.tsx`、`components/droid/TerminalPage.tsx`、`components/droid/TerminalView.{tsx,test.tsx}`、`components/openclaw/OpenClawFeatureList.tsx`、`components/opencode/OpenCodeFeatureList.tsx`                                                                                                       |

- **验证**：`npm run test:run`；冒烟：导入/导出文件对话框、路径选择、复制 API key。
- **完成判据**：三个 shim 无外部引用。

### A5 — pty / 字体 / 菜单 / 上下文菜单

- [ ] `src/lib/shell/pty.ts` ← `shims/tauri-pty.ts`，导出 `spawn()` 与 `IPty`（`TerminalView` 依赖的
      方法子集：`write`、`resize`、`onData`、`onExit`、`kill`）
- [ ] `components/droid/TerminalView.{tsx,test.tsx}`：`from 'tauri-pty'` → `from '@/lib/shell/pty'`
- [ ] `src/lib/shell/fonts.ts` ← `shims/tauri-system-fonts.ts`；改
      `components/preferences/panes/AppearancePane.tsx`
- [ ] 菜单：
  - `src/wails/menu-bridge.ts` → `src/lib/shell/menu-bridge.ts`（A6 前先挪，避免双重改动）
  - `src/lib/menu.ts`（Tauri 原生菜单构建器）：**删除**（仅 `src/App.tsx` 使用）
  - `src/lib/context-menu.ts` → `src/lib/shell/context-menu.ts`。Wails 无原生菜单 API，
    两种实现二选一，**在 PR 描述里记录选择**：
    (a) 复用 `components/ui/context-menu`（Radix）在调用点改为受控组件（改动 5 个 Sessions 页）；
    (b) 用 Wails 的 `menu` 服务（若 bindings 暴露）实现 `showContextMenu(items)`
  - 5 个调用点：`components/codex/CodexSessionsPage.tsx`、`components/droid/SessionsPage.tsx`、
    `components/dsh/DshSessionsPage.tsx`、`components/omp/OmpSessionsPage.tsx`、
    `components/pi/PiSessionsPage.tsx`；测试 `src/lib/context-menu.test.ts` 同步
- **验证**：`npm run test:run`；冒烟：终端页签可输入/滚屏/关闭；Sessions 右键菜单可用。
- **完成判据**：`src/wails/shims/` 只剩 `tauri-core.ts`（A2 已搬，故为空目录待删）。

### A6 — 入口合并 + Vite 收敛（必须一个 PR）

- [ ] 根 `index.html`：采用 `wails/index.html` 的内容（`/src/main.tsx`），**保留** Tauri 版里
      「不要在 HTML 内联 `<style>`」的注释（xterm 字体 hack 的成因）
- [ ] `src/main.tsx`：以 `src/wails/main.tsx` 为骨架，并合入 Tauri 版的 `applyInitialTheme()`
      与（可选）`ReactQueryDevtools`
- [ ] `src/App.tsx`：用 `src/wails/AppShell.tsx` 覆盖；删除 `isToolSupported`/`WAILS_FALLBACK_TOOL`
      重定向（所有工具都有后端）；`src/wails/wails.css` → `src/shell.css`
- [ ] 删除 `src/wails/{main.tsx,AppShell.tsx,wails.css}` 与空目录 `src/wails/shims/`
- [ ] 根 `vite.config.ts` = 现 `wails/vite.config.ts` 的配置，改动：
  - `root: __dirname`（仓库根），`outDir: 'wails/frontend/dist'`
  - `publicDir: 'public'`，`rollupOptions.input: index.html`
  - 别名保留 `@`、`@wails-bindings`，以及 A3–A5 的新 shell 模块；删除所有 `@tauri-apps/*` 别名
  - `server.fs.allow: [__dirname]`；端口沿用 `9245`（或 `WAILS_VITE_PORT`）
  - 保留 `restoreGitkeep()` 插件（`wails/frontend/dist/.gitkeep`）
  - `define.__WAILS_BACKEND__` 可删（capabilities 简化后无消费者），删除时同步删
    `src/lib/capabilities.ts` 里的声明
- [ ] 删除 `wails/vite.config.ts`；`wails/build/Taskfile.yml` 的 `build:frontend`
      改为 `npx vite build {{mode}}`（去掉 `--config`）
- [ ] `vitest.config.ts` 别名同步（删 `tauri-pty`）
- [ ] `src/App.css`：确认 `@source './**/*.{ts,tsx}'` 仍在（Vite root 变了，Tailwind 依赖它）
- **验证**（全部必跑）：

  ```bash
  rm -rf wails/frontend/dist && npx vite build     # 产出 wails/frontend/dist
  ls wails/frontend/dist && test -f wails/frontend/dist/.gitkeep
  grep -o "h-screen" wails/frontend/dist/assets/*.css | head -1     # Tailwind guard
  cd wails && go vet ./... && go test ./... && cd ..
  cd wails && wails3 task build && wails3 task run   # 真启动一次
  ```

- **完成判据**：`npm run dev` 起的是 Wails 前端；`wails3 task run` 正常；冒烟清单（设计 §6）通过。
- **回滚**：单 PR revert；这是唯一「一 revert 就回到双入口」的 PR，故必须独占提交。

### A7 — 测试 mock 迁移

- [ ] `src/test/setup.ts`：把 `vi.mock('@tauri-apps/…')` 换成新模块路径

  ```ts
  // 旧
  vi.mock('@tauri-apps/plugin-dialog', () => ({ open: vi.fn(), ... }))
  // 新
  vi.mock('@/lib/shell/dialog', () => ({ openFileDialog: vi.fn(), ... }))
  ```

- [ ] 13 个测试文件的 mock 目标同步：
      `components/codex/CodexConfigPage.test.tsx`、`components/codex/CodexSessionsPage.test.tsx`、
      `components/copilot/CopilotConfigPage.test.tsx`、`components/droid/DroidFeatureList.test.tsx`、
      `components/droid/TerminalView.test.tsx`、`components/droid/TrustedFoldersPage.test.tsx`、
      `components/dsh/DshSessionsPage.test.tsx`、`components/omp/OmpSessionsPage.test.tsx`、
      `components/pi/PiSessionsPage.test.tsx`、`components/preferences/panes/GeneralPane.test.tsx`、
      `hooks/use-platform.test.ts`、`lib/context-menu.test.ts`、`services/updater.test.ts`（Q1-A 时删除）
- [ ] 补 1 个 `src/lib/shell/commands.test.ts`：未知命令、参数顺序、错误字符串规范化
      （防止 A2 的映射表被后续改动破坏）
- **验证**：`npm run test:run`（覆盖率阈值 60% 不得下调）。
- **完成判据**：`grep -rn "@tauri-apps" src` 为空。

### A8 — 依赖与脚本清理（可与 B1 合并）

- [ ] `package.json` 删除依赖：13 个 `@tauri-apps/*`、`tauri-pty`、
      `tauri-plugin-system-fonts-api`、devDep `@tauri-apps/cli`
- [ ] `npm install` 重新生成 lockfile（**独立 commit**：`chore: drop tauri deps`）
- [ ] 删除脚本：`tauri*`、`rust:*`、`tui*`，以及 `test:all`/`check:all` 里的 rust 步骤
- [ ] 删除 `scripts/tauri-dev.sh`、`scripts/tui-dev.sh`
- [ ] `knip.json`、`eslint.config.js`、`.gitignore`、`sgconfig.yml` 去掉 `src-tauri` 相关项
      （`eslint` 的 ignore、`tsconfig` 的 include/ignore 若有）
- **验证**：`npm ci && npm run check:all`（此时 check:all 已无 rust 步骤）零错误；
  `npm ls --depth=0` 无 tauri 包。
- **完成判据**：`rg -n "tauri" package.json` 为空。

---

## 阶段 B：删除 Rust 工具链

> 前置：A6 完成（否则删除后无 GUI 可构建）。目标：仓库不再需要 Cargo。

### B1 — 构建脚本与工具链

- [ ] `justfile` 删除（或整份替换为 Go 版 `dg` 交叉编译配方，二选一，PR 描述说明）
- [ ] `rust-toolchain.toml` 删除
- [ ] `package.json` 最终确认：无 `rust:*`/`tui:*`/`tauri*`
- [ ] `docs/agents/commands.md` 去掉 Rust/TUI 段落，补 Go/CLI 段落
- **验证**：`node -e "const p=require('./package.json');console.log(Object.keys(p.scripts))"`；
  `npm run check:all` 零错误。
- **完成判据**：仓库内除 `docs/`（历史）外，`rg -n "cargo|rustup" --glob '!docs/**'` 为空。

### B2 — 删除 `src-tauri/`

- [ ] `git rm -r src-tauri`（`rm -rf` 后 `git add -A` 亦可；遵循 AGENTS 规则 8 的 `rm -f` 习惯）
- [ ] 删除根 `index.html` 里遗留的 Tauri 注释（若 A6 未处理）
- [ ] 确认 `wails/scripts/*`、`scripts/check-*.mjs` 不再读 `src-tauri`
- **验证**：

  ```bash
  test ! -d src-tauri && echo ok
  node scripts/check-command-surface.mjs     # 需先完成 F1 收缩，否则会因缺 Rust 输入而红
  node scripts/check-go-layering.mjs
  cd wails && go vet ./... && go test ./...
  ```

- **完成判据**：`git ls-files src-tauri | wc -l` 为 0；所有门禁绿。
- **注意**：删除前打 tag（`git tag pre-tauri-removal`）便于追溯 core 实现。

### B3 — CI / 发布流水线收编

- [ ] `release.yml`（TUI 的 4 个 step 已在 P0 删除，这里只处理 Tauri）：
  - 删除 `publish-tauri` job（三端 Tauri 安装包 + 便携更新资产）
  - 删除 `generate-updater-json`（依赖 `publish-tauri`；Q1-A 已确认一并删）
  - 保留 `publish-wails`（windows GUI + 安装包）
  - `create-release` 的资产说明更新
- [ ] `release-single.yml`：确认为**三 OS 的 GUI + `dg` 唯一出口**；job 名/资产名去掉歧义
      （`droidgear-wails-*` → 是否改名见 PR 讨论，**本期不改名**，避免破坏下载链接）
- [ ] `wails.yml` → 重命名 `ci.yml`（可选，建议做），保留 `build-test` + `cli-headless`，
      在 `build-test` 里加 `node scripts/check-no-tauri.mjs`
- [ ] 在 PR 用 `workflow_dispatch` 空跑一次发布流水线（不 push tag）
- **验证**：`gh workflow run` 或 PR 触发的 CI 全绿；发布草稿里资产清单人工核对。
- **完成判据**：CI 无 Rust 安装步骤；发布资产 = GUI（3 OS）+ `dg`（3 OS）。
- **风险**：`generate-updater-json` 删掉后，旧客户端自动更新会 404 → 在 README/Release notes 声明。

### B4 — 门禁收缩 + 文档与规则改写

- [ ] `scripts/check-command-surface.mjs`：
  - 删除 Rust `invoke_handler` 解析、`RUST_ONLY` 集合、dead-command baseline
  - 保留：COMMANDS 表 ↔ 生成 bindings 的**参数名与顺序**一致性；Go 方法全覆盖（routed/direct/baseline）
  - 文件头注释改写：不再提 Tauri
- [ ] `scripts/check-no-tauri.mjs`：去掉 A 阶段白名单，扫描面扩到全仓库（`docs/` 白名单）
- [ ] `docs/agents/wails-port.md`：删「Shell-specific code / Tauri 列」，保留 Wails 陷阱与验证清单
- [ ] `AGENTS.md`：
  - 删除「Dual Shell (Tauri + Wails)」整段与分支横幅，改为「单壳（Wails v3 + Go）」
  - 规则 1/2 重写：`src/` 只允许 `src/lib/shell/*` 触碰 `@wailsio/runtime`/`@wails-bindings`；
    命令面三处对齐（Go 方法 ↔ 生成 bindings ↔ façade 表）
  - 规则 9 改写：parity 对象 = Wails GUI ↔ `dg`（+ `dg tui`）；删除 Rust TUI 豁免条款
  - 规则 10：去掉 `npm run rust:fmt`
- [ ] `docs/developer/README.md`：删/归档 Tauri 专属文档条目（`tauri-commands.md`、
      `tauri-plugins.md`、`rust-architecture.md`），标记为「历史（v1.3 前）」
- [ ] `README.md` / `README_EN.md`：安装/发布/开发章节去掉 Tauri/TUI，补 `dg` 用法；
      覆盖差异表按 Q3 决定处理
- [ ] 归档 Tauri 专属文档：移入 `docs/archive/tauri/`（或文件头加 `> **冻结**`）
- **验证**：`npm run format:check && npm run lint`；`node scripts/check-no-tauri.mjs`；
  人工通读 AGENTS.md（它是 agent 的入口）。
- **完成判据**：新 agent 从 AGENTS.md 读不到任何「还要同步 Tauri」的指令。

---

## 阶段 C：能力补齐（service + CLI/TUI 接棒）

> 与 A/B 并行；目标：让 `dg` 成为 T1 全覆盖的非 GUI 消费者，并落地 `dg tui`。
> 每个域一个 PR，模式与 B2 已完成的 `codex/hermes/pi/omp` 完全一致（见
> `wails-tui-cli-design.zh.md` §10 的迁移备注）。

### C0 — 通用前置（一次性）

- [ ] `--force` + `beforeHash` 冲突检测：`core` 增加「写前重读 → 哈希比对 → 不一致返回
      `conflict`（exit 3）」；`dg` 全局 flag 增加 `--force`
- [ ] 扩充 `cmd/dg/dg_test.go` 的 `sandbox()`：`PATH` 隔离（已有）+ 新增域的 home 变量
- [ ] `wails/PORTING.md` 的 T1/T2 覆盖表补「未覆盖 = 豁免」列

### C1 — `openclaw`（B2 暂缓）

- [ ] `wails/internal/service/openclaw/`：搬 `app/openclaw.go`（1,243 行）+ 测试；
      向上依赖就地解决（对照 `pi`/`hermes` 的 `fileExists` 处理）
- [ ] `app/openclaw.go` 保留薄转发（签名不变，`check-command-surface` 必须仍绿）
- [ ] `dg openclaw profiles list|show|active|apply|save|delete` + `status`
- [ ] secrets：列出 openclaw 的密钥字段，默认打码、`--reveal` 出明文
- **验证**：`go test ./internal/service/openclaw ./cmd/dg`；`dg openclaw profiles apply <id> --dry-run --json` golden。

### C2 — `opencode` + `dg opencode`

- [ ] 同上模式（`app/opencode.go` 712 行），加上 opencode 特有的 providers/config 读取
- **验证**：同 C1。

### C3 — `claude` / `copilot`（B3 前半）

- [ ] `service/claude`：`app/claude_settings.go`（1,603）+ `app/claude.go`（665）+
      `claude_settings_mapping` 相关；注意 settings.json 的 unknown-key 往返
- [ ] `service/copilot`：`app/copilot.go`（1,607）
- [ ] `dg claude profiles …`、`dg copilot profiles …`（同形态 `list|show|active|apply|save|delete` + `status`）
- **验证**：每域至少一条「断言落盘最终内容」的用例（含 unknown 字段保留）。

### C4 — `droid` / `dsh` / `factory` / `mcp` / `legacy_config`（B3 后半）

- [ ] `droid`：`droid_settings.go`（1,669）+ `droid_trusted.go` + 启动器（launch 偏好留 app）
- [ ] `dsh`：`dsh.go` + `dsh_config{,_read,_write}.go`（1,063+690+396+503）
- [ ] `factory`：`factory.go` + `factory_auth_profiles.go`
- [ ] `mcp`：`mcp.go`（318）
- [ ] `legacy_config`：`legacy_config.go`
- [ ] `dg droid models list|apply`、`dg dsh providers list|apply --profile`、
      `dg mcp list|toggle|apply`
- **验证**：`dg dsh providers apply --dry-run --json`（依赖 C0 的 ChangeSet 输出）golden；
  每个域断言落盘内容。

### C5 — B4 只读：sessions / specs

- [ ] `service/read`：sessions（codex/dsh/omp/pi/claude/droid）+ specs 只读查询；
      watcher 留在 `internal/app`
- [ ] `dg sessions projects|list|show <id>`、`dg spec list|cat <name>`
- **验证**：只读命令在 `--home` 沙箱下的 golden JSON。

### C6 — `dg tui`（Q2-A 已确认：现在启动，P1 只读优先）

- [ ] `wails/internal/tui/`：bubbletea Model（左模块 / 右列表+详情 / 底部状态），
      数据只来自 service；apply 前展示 ChangeSet diff（复用 `dg --dry-run` 的同一份结果）
- [ ] `dg tui` 子命令接入 `cmd/dg/main.go`（`--home`、`--json` 不适用于 TUI，需明确报 usage）
- [ ] 依赖放行：`wails/go.mod` 加 `bubbletea`/`lipgloss`，在 `PORTING.md` 记录理由与版本
- [ ] 分层门禁 R4：`internal/tui` 不得 import `internal/app`
- [ ] 分期：P1 只读浏览（10 工具 + paths）→ P2 预览 → P3 apply/delete
- **验证**：`go test ./internal/tui`（Model 级测试，不测终端渲染）；`CGO_ENABLED=0 go build ./cmd/dg`；
  SSH 冒烟（`dg tui` 在 80×24 与 120×40 下不崩）。
- **完成判据**：P1 覆盖 10 工具；P2 对 `codex/pi/hermes/omp` 四个已迁移域给出 diff。

---

## 阶段 D：收尾（每个阶段末尾顺手做，最后统一确认）

- [ ] `CHANGELOG.md` 记录「移除 Tauri」与「自动更新变化」
- [ ] `docs/developer/releases.md` 更新发布流程（只剩 Wails 三 OS + dg）
- [ ] `docs/developer/testing.md` 去掉 Tauri mocking 段落，补 shell mock 约定
- [ ] `docs/developer/architecture-guide.md`、`data-persistence.md`、`logging.md` 等
      提到 Rust/Tauri 的句子改为「Go core」（**纯文字，不改行为**）
- [ ] 打 tag `pre-tauri-removal` / `wails-only`，README 徽章与截图更新
- [ ] 最后跑一遍全量验证（见附录 A）

---

## 附录 A：全量验证命令（收工前必跑）

```bash
# 1) 依赖与构建
npm ci
cd wails && wails3 generate bindings -clean=true -ts -i && cd ..   # 生成物 gitignored
npx vite build                                                     # → wails/frontend/dist

# 2) 前端质量
npm run typecheck && npm run lint && npm run ast:lint && npm run format:check && npm run test:run

# 3) Go 质量（含 CLI/TUI）
cd wails && go vet ./... && go test ./... && cd ..
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -C wails ./cmd/dg   # 无 WebKit 依赖证明

# 4) 门禁
node scripts/check-no-tauri.mjs
node scripts/check-command-surface.mjs
node scripts/check-go-layering.mjs
node wails/scripts/sync-version.mjs --check
cmp src/lib/model-registry-data.json wails/internal/core/registrydata/model-registry-data.json

# 5) 汇总
npm run check:all       # 应为「无 rust 步骤」版本
```

## 附录 B：机械替换速查

```bash
# A3–A5 的 import 替换（先改 specifier，再跑测试，逐组提交）
# events
rg -l "@tauri-apps/api/event" src | xargs sed -i "s|@tauri-apps/api/event|@/lib/shell/events|g"
# window / webviewWindow
rg -l "@tauri-apps/api/(window|webviewWindow)" src | xargs sed -i -E "s|@tauri-apps/api/(window|webviewWindow)|@/lib/shell/window|g"
# dialog / clipboard / os / opener / fs / menu
for m in dialog:plugin-dialog clipboard:plugin-clipboard-manager os:plugin-os opener:plugin-opener fs:plugin-fs; do
  mod="${m%%:*}"; pkg="${m##*:}"
  rg -l "@tauri-apps/$pkg" src | xargs sed -i "s|@tauri-apps/$pkg|@/lib/shell/$mod|g"
done
rg -l "@tauri-apps/api/menu" src | xargs sed -i "s|@tauri-apps/api/menu|@/lib/shell/context-menu|g"
# pty / fonts
rg -l "tauri-pty" src | xargs sed -i "s|'tauri-pty'|'@/lib/shell/pty'|g"
rg -l "tauri-plugin-system-fonts-api" src | xargs sed -i "s|tauri-plugin-system-fonts-api|@/lib/shell/fonts|g"
```

> `sed -i` 在 macOS 需要 `-i ''`；本仓库主力开发在 Windows（Git Bash / WSL）时按上表执行。
> 替换后立刻 `npm run typecheck`——**不要批量替换后一次性跑**，否则定位成本高。

## 附录 C：任务 → 验收速查

| 任务  | 关键验证                                                       | 完成标志               |
| ----- | -------------------------------------------------------------- | ---------------------- |
| P0    | `workflow_dispatch` 空跑 + 发布草稿资产清单                    | 发布项无 droidgear-tui |
| A1    | `node scripts/check-no-tauri.mjs`（含一次故意违规）            | 门禁存在且在 CI        |
| A2    | `npm run typecheck` + `npx vite build` + `grep TAURI_INVOKE`=0 | bindings 无 tauri      |
| A3    | `npm run test:run` + Sessions 页手工                           | 4 shim 无外部引用      |
| A4    | `npm run test:run` + 文件对话框手工                            | 3 shim 无外部引用      |
| A5    | `npm run test:run` + 终端/Sessions 右键手工                    | shims 目录可删         |
| A6    | `npx vite build` + Tailwind guard + `wails3 task run`          | 唯一入口 + 唯一 vite   |
| A7    | `npm run test:run` + `grep @tauri-apps src`=空                 | 测试全绿               |
| A8    | `npm ci && npm run check:all` + `npm ls`                       | 无 tauri 依赖/脚本     |
| B1    | `npm run check:all`                                            | 无 cargo 入口          |
| B2    | `git ls-files src-tauri`=0 + 全部门禁                          | 源码树无 Rust          |
| B3    | CI 全绿 + 发布草稿资产核对                                     | 流水线只剩 Wails       |
| B4    | `check-no-tauri` 全仓 + 人工读 AGENTS.md                       | 规则无 Tauri           |
| C0    | `--force` + exit 3 用例（外部改动后写盘）                      | 冲突检测可用           |
| C1–C5 | 每域 `go test` + `dg … --dry-run --json` golden                | T1 覆盖或豁免          |
| C6    | `go test ./internal/tui` + SSH 冒烟                            | `dg tui` 可用          |
| D     | 附录 A 全量                                                    | 收工                   |
