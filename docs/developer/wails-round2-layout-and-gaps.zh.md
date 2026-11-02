# Wails v3 第二轮：布局回归定位、剩余需求、双壳回馈与 Agent 配置

> 审计日期：2026-11-02　分支：`spike/wails-v3-minimal`（`886ab84`）
> 本文是 [`wails-port-guidelines.zh.md`](./wails-port-guidelines.zh.md) 的续篇：
> 那份文档回答「移植得对不对 / 全不全」，本文回答**这一轮用户看到的三个问题**
> ——「布局全乱」「窗体本身和按钮不对」「还有什么已知需求没做」，并顺带给出
> **Agent 配置该改什么**与**哪些问题应回馈给 Tauri 版**。
>
> 本轮**未修改任何代码**，只做定位、方案与文档。

---

## 0. TL;DR

| #    | 结论                                                                                                                                                                                                                                          | 证据强度 | 严重度 | 状态              |
| ---- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | ------ | ----------------- |
| P0-1 | **布局崩坏的唯一根因是 Tailwind v4 没有扫描 `src/`**：Wails 构建把 Vite `root` 改成了 `wails/`，自动内容探测跟着跑到了 `wails/`，产物 CSS 只有 45 kB（正确应为 136 kB），`.h-screen` / `.w-[280px]` / 按钮变体全部缺失                        | 实测复现 | ❌ P0  | 待修（方案已验）  |
| P0-2 | **当前工作区 `node_modules` 与 `package-lock.json` 严重漂移**：Tauri 全家桶装成 `3.0.0-alpha.*`（声明是 `^2.x`）、`@wailsio/runtime` 与 `@typescript/native-preview` 直接缺失 → `vite build`、`npm run typecheck`、Wails 前端构建**全部失败** | 实测复现 | ❌ P0  | 待修（`npm ci`）  |
| P0-3 | 干净克隆下 `cd wails && go vet ./...` / `go test ./...` **直接失败**（`//go:embed all:frontend/dist` 找不到目录），而 CI 的 Go 步骤排在构建前端之前 → **`wails.yml` 的 Go 步骤必然失败**（除非机器上残留过 `frontend/dist`）                  | 实测复现 | ❌ P0  | 待修              |
| P1-1 | **Frameless 窗口在 Windows/Linux 上永远拿不到原生菜单**（Wails 源码 `if !options.Frameless && !options.Windows.DisableMenu`）→ 上一轮「原生菜单 ✅ 完成」对 Windows 实际无效                                                                  | 源码定位 | ⚠️ P1  | 需重新决策        |
| P1-2 | Frameless 的代价清单：Windows 失去**标题栏双击最大化**（runtime 只在 macOS 处理 `dblclick`）、失去 Snap Layouts、边缘缩放全靠 runtime 的 JS 命中检测                                                                                          | 源码定位 | ⚠️ P1  | 记录 + 补前端行为 |
| P1-3 | 窗口几何与启动序列和 Tauri 不一致：`1280×840 / 最小 800×600 / 立即显示 / 底色写死 RGB(6,7,15)` vs Tauri `1150×700 / 最小 1150×700 / 先隐藏再 show()`                                                                                          | 代码对比 | ⚠️ P1  | 待对齐            |
| P1-4 | 剩余已知需求（系统字体、平台 shim、跟随主题的窗口底色、TUI/CLI、updater 决策）全部仍在，且分散在两份文档里                                                                                                                                    | 文档核对 | ⚠️ P1  | 本文 §4 汇总      |
| P2-1 | Agent 配置完全没提 Wails：`AGENTS.md`、`docs/agents/*`、`CLAUDE.md` 里 `wails` 出现 0 次；`register-model` 技能漏掉 registry 同步（CI drift 会红）；`knip` 把 15 个 Wails 文件报成未使用                                                      | 实测复现 | ⚠️ P2  | 本文 §7 给稿      |
| P2-2 | 可回馈 Tauri 版的 6 条共性问题（构建护栏缺口、硬编码深色底、`100vw/100vh` 布局盒、拖拽契约耦合、壳层启动逻辑重复、能力矩阵变死代码）                                                                                                          | 分析     | ⚠️ P2  | 本文 §6           |

**一句话**：命令面和 Go 后端是可信的，**问题全部集中在「前端资源构建」和「窗口外壳」这两层**——
布局乱不是组件坏了，是 **CSS 根本没生成**；窗口不对不是 Wails 的锅，是 **frameless 模式的固有代价没被记录**。

---

## 1. 复现前必须先做的事（否则所有结论都不可信）

本轮排查一开始就跑不通任何构建，原因是**工作区依赖漂移**，与代码无关：

```bash
npm ls --depth=0 | grep -E "invalid|UNMET"
# @tauri-apps/api@3.0.0-alpha.3        invalid: "^2.11.0"
# @tauri-apps/plugin-opener@3.0.0-alpha.3 invalid: "^2.5.4"
# @ast-grep/cli@0.51.0                 invalid: "^0.40.3"
# @testing-library/jest-dom@7.0.1      invalid: "^6.9.1"
# @types/node@26.6.6                   invalid: "^25.0.2"
# @vitest/coverage-v8@5.0.5            invalid: "^4.1.12"
# UNMET DEPENDENCY @wailsio/runtime@^3.0.0
# UNMET DEPENDENCY @typescript/native-preview@^7.0.0-dev.20260421.2
# UNMET DEPENDENCY @eslint/js@^9.39.2
# UNMET DEPENDENCY eslint-config-prettier@^10.1.8
```

后果（全部实测）：

| 命令                                         | 结果                                                            |
| -------------------------------------------- | --------------------------------------------------------------- |
| `npm run typecheck`                          | `tsgo: not found`（`@typescript/native-preview` 缺失）          |
| `npx vite build`（Tauri 前端）               | `[MISSING_EXPORT] "revealItemInDir"`（装成了 opener 3.0 alpha） |
| `npx vite build -c wails/vite.config.ts`     | `Rolldown failed to resolve import "@wailsio/runtime"`          |
| `cd wails && go vet ./...` / `go test ./...` | `pattern all:frontend/dist: no matching files found`            |

**结论**：所有「Wails 版坏了」的判断都必须先跑一遍

```bash
npm ci                                   # ← 先把 2.x / lockfile 状态装回来
cd wails && wails3 generate bindings -clean=true -ts -i && cd ..
npx vite build --config wails/vite.config.ts --mode production
cd wails && go vet ./... && go test ./...
```

> 注意 `wails3 generate bindings` 是**必须**的：`wails/frontend/bindings/**` 被 `.gitignore` 忽略，
> 而 `src/wails/shims/tauri-core.ts` 通过 `@wails-bindings` 别名直接 import 它。
> 干净克隆不生成绑定，前端构建第一步就挂（本轮实测）。

---

## 2. P0-1：布局崩坏 = Tailwind 没扫到 `src/`（唯一根因）

### 2.1 证据

`wails/vite.config.ts:22` 把 `root` 改成了 `wails/`：

```ts
root: dir,                                   // dir = <repo>/wails
publicDir: resolve(repoRoot, 'public'),
```

Tailwind v4（`@tailwindcss/vite`）的**自动内容探测以 Vite `root` 为基准**，不看你从哪个 CSS 文件
`@import "tailwindcss"`。于是 `wails/` 之外（即整个 `src/`）的 class 几乎一个都没生成：

| 检查项                                          | 产物 CSS                                                                                                        |
| ----------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| 现状（无 `@source`）                            | **45.19 kB**，`.h-screen` = 0 处，`280px` = 0 处，只剩 `.flex` / `.hidden` 等极少数工具类（`@apply` + base 层） |
| 在 `src/App.css` 加 `@source "./**/*.{ts,tsx}"` | **135.96 kB**，`.h-screen` 存在，`w-[280px]` 存在                                                               |

复现（本轮的验证方式，验证后已还原文件）：

```bash
npx vite build --config wails/vite.config.ts --mode production
ls wails/frontend/dist/assets/index-*.css               # 45.19 kB
grep -c 'h-screen' wails/frontend/dist/assets/index-*.css   # 0
# 追加 @source 到 src/App.css 后再构建 → 135.96 kB，两类名均命中
```

`@source` 追加到 **`src/wails/wails.css` 里无效**（已试）：`wails.css` 没有 `@import "tailwindcss"`，
Tailwind 不会把它当作「生成器的根样式表」，`@source` 被忽略。

### 2.2 为什么症状是「左侧导航栏 + 窗体 + 按钮全乱」

`MainWindow` / `LeftSideBar` 的骨架完全由缺失的那批工具类搭起来：

| 位置                                        | 依赖的类（现状全缺）                            | 缺失后果                                         |
| ------------------------------------------- | ----------------------------------------------- | ------------------------------------------------ |
| `src/components/layout/MainWindow.tsx:80`   | `h-screen w-full flex-col overflow-hidden`      | 外层没有高度 → 整页塌成内容高                    |
| `src/components/layout/MainWindow.tsx:85`   | `w-[280px] shrink-0`                            | 左侧栏没有固定宽度，被内容撑开/挤扁              |
| `src/components/layout/LeftSideBar.tsx:229` | `h-full flex-col border-r bg-background`        | 导航项无排列、无分隔线、无底色                   |
| 所有 `src/components/ui/button.tsx` 变体    | `h-9 px-4 bg-primary text-primary-foreground …` | 按钮变成「无边框纯文本」，即用户说的「按钮坏了」 |
| `TitleBar`                                  | `h-8 border-b bg-background`                    | 标题栏高度失控，拖拽区看起来像「窗体贴错位」     |

所以「左导航栏乱 / 窗体本身乱 / 按钮不对」是**同一个根因的三种表现**，不需要分别修组件。

### 2.3 修法（已验证有效，二选一）

**方案 A（推荐，改动最小、双壳受益）**——在共享样式入口显式声明内容源：

```css
/* src/App.css */
@import 'tailwindcss';
@import 'tw-animate-css';
@import './theme-variables.css';
@import 'sonner/dist/styles.css';

/* Tailwind v4 的自动探测以 Vite root 为基准。Tauri 构建 root=仓库根，碰巧正常；
   Wails 构建 root=wails/，探测不到 src/。显式声明后与构建入口解耦。 */
@source './**/*.{ts,tsx}';
@source '../node_modules/streamdown/dist/*.js';
@custom-variant dark (&:is(.dark *));
```

- Tauri 侧无副作用（只是把自动探测变成显式，重复扫描可忽略）。
- 注意 `@source` 路径**相对于 `App.css` 自身**（`src/`），不是相对 `root`。

**方案 B（备选，不推荐）**——让 Wails 构建的 root 回到仓库根：

```ts
// wails/vite.config.ts
root: repoRoot,
build: { rollupOptions: { input: resolve(dir, 'index.html') }, outDir: resolve(dir, 'frontend/dist') },
publicDir: resolve(repoRoot, 'public'),
```

代价：dev server 的 `/` 变成仓库根（`index.html` 冲突要额外处理）、`?raw`/静态路径语义变化，
收益只是「不用改共享 CSS」。**不要为了不动共享文件选这条**。

### 2.4 验收标准

```bash
npm ci && cd wails && wails3 generate bindings -clean=true -ts -i && cd ..
npx vite build --config wails/vite.config.ts --mode production
css=$(ls wails/frontend/dist/assets/index-*.css)
grep -q '\.h-screen' "$css" && grep -q '280px' "$css" && echo OK
# 目视：wails3 task run → 左侧 280px 定宽导航、标题栏 32px、按钮有底色与圆角
```

---

## 3. P0-2 / P1：窗口、标题栏、按钮的外观与行为

### 3.1 先修环境，再谈现象

「窗体本身不对」里有相当一部分是 **§2 的 CSS 缺失**（标题栏高度、边框、阴影都没生成）＋
**§1 的依赖漂移**（`@wailsio/runtime` 缺失 → `Window.*` 全部不可用 → 窗控按钮点了没反应）。
在这两项修好之前，不要下任何「Wails 窗口 API 有问题」的结论。

### 3.2 Frameless 的代价必须写进文档（清单）

`wails/main.go:79` 用的是 `Frameless: true`。Wails v3.0.0 的实际行为（源码定位）：

| 能力                               | Tauri（`decorations(false)`） | Wails v3 + `Frameless: true`                                                                                                                                              | 结论                         |
| ---------------------------------- | ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------- |
| DWM 阴影/圆角                      | ✅ 系统绘制                   | ✅ `framelessWithDecorations()` 默认保留（`Windows.DisableFramelessWindowDecorations=false`）                                                                             | 可用                         |
| 拖拽移动                           | `data-tauri-drag-region`      | `--wails-draggable: drag`（`internal/runtime/desktop/@wailsio/runtime/src/drag.ts`）                                                                                      | 可用（映射已在 `wails.css`） |
| 标题栏双击最大化                   | ✅ 原生                       | ❌ **仅 macOS**：`suppressEvent()` 里 `dblclick` 分支被 `IsMac()` 守卫；Windows 需前端自己实现                                                                            | **缺陷**                     |
| Snap Layouts（悬停最大化按钮分屏） | ✅ 原生                       | ❌ 无一等支持                                                                                                                                                             | 记录                         |
| 边缘缩放                           | ✅ 原生（`WS_THICKFRAME`）    | ⚠️ 依赖 runtime 的 JS 命中检测（源码注释：frameless 后「disables the resizing from user interaction… we still need WS_THICKFRAME to enable resizing from the frontend」） | 可用但需实测 4 边 + 4 角     |
| 原生菜单                           | ✅（非 macOS 也可）           | ❌ **`webview_window_windows.go:525`：`if !options.Frameless && !options.Windows.DisableMenu`** → frameless 窗口不挂菜单                                                  | **上一轮结论有误**           |

**方案（三选一，必须显式决策）**：

1. **保持 frameless**，接受「Windows 无菜单栏」：
   - 在标题栏加一个 `⋯`/汉堡菜单按钮，复用 `internal/app/menu.go` 的 action ID（不重复实现菜单逻辑）；
   - 在 `wails.css` + `TitleBar` 侧补双击最大化（监听 `dblclick` → `Window.ToggleMaximise()`）。
     **必须只对「需要自己处理」的壳生效**（用 `IS_WAILS_BACKEND` 或一个新的 `capabilities.ts` 旗标门控）：
     Tauri 侧已由系统原生处理双击，共享组件里无条件注册会变成「最大化后又立即还原」；
   - 在 `wails/README.md` 与 guidelines §3.2 里把「原生菜单 ✅」改成「macOS ✅ / Windows&Linux 由窗口内菜单承载」。
2. **去掉 frameless**（`Frameless: false`，标题栏/菜单/双击/分屏全部交回系统）：
   换回原生标题栏 + 原生菜单 + 原生双击/分屏，但会丢掉共享 `TitleBar` 的视觉一致性（两条壳外观分叉），
   且 `src/wails/wails.css` 的拖拽映射变成无用代码。
3. **混合**：Windows 走 1，macOS 保留 frameless（红绿灯由 `MacOSWindowControls` 自绘，需要
   `MacOptions` 相关选项实测），Linux 走 1。

> 建议选 **1**：与「共享 `TitleBar` 零改动复用」的目标一致，代价只有一个菜单按钮（TUI 侧本来也没有原生菜单）。

### 3.3 拖拽区的 opt-out 清单不完整

`wails.css` 目前只把 `button / [role=button] / a / input / select / [data-dropdown-menu-trigger]`
设成 `--wails-draggable: none`。`TitleBarContent` 里还有可点击的 `div`（标题、状态徽标、
自定义下拉触发器）和 `[role=menuitem]`。runtime 只检查**最深命中元素**的 computed
`--wails-draggable`（自定义属性会继承，所以要靠子元素 opt-out 覆盖）。

**修法**：把 opt-out 列表补成
`button, [role='button'], [role='menuitem'], [role='combobox'], a, input, select, textarea, [tabindex]:not([tabindex='-1']), [data-dropdown-menu-trigger], [data-no-drag]`，
并在宿主的共享组件里给这类节点加 `data-no-drag`（跨壳中立属性，见 §6-T6）。

### 3.4 窗口几何 / 启动序列与 Tauri 不一致

| 项       | Tauri（`src-tauri/src/lib.rs:140-160`）     | Wails（`wails/main.go:69-82` + `AppShell.tsx:44-53`） |
| -------- | ------------------------------------------- | ----------------------------------------------------- |
| 默认尺寸 | `1150 × 700`                                | `1280 × 840`（再用 `screen.avail* - 48` 收一次）      |
| 最小尺寸 | `1150 × 700`（= 默认，用户无法再缩）        | `800 × 600`                                           |
| 首次可见 | `visible: false` → 前端 `show()+setFocus()` | `Frameless` 窗口创建即可见 + `#app-loader` 遮罩       |
| 背景色   | 由主题/WebView 决定                         | `BackgroundColour: NewRGB(6,7,15)` 写死深色           |
| macOS    | `transparent(true)` + `decorations(false)`  | 无对应设置（macOS 未验证）                            |
| 窗口状态 | `tauri-plugin-window-state`                 | 自研 `window-state.json`（路径不同，需确认兼容）      |

**结论**：默认/最小尺寸没有「契约」文档，两壳已经分叉；`800` 宽时「280px 左栏 + 主面板 30% 最小」
刚好卡在边界，用户会觉得「一缩小就错位」。

**建议**：把窗口契约写进 `wails/PORTING.md` 与 guidelines（默认 1150×700、最小 1150×700、
居中、先隐藏后 show），两壳按同一张表执行；`BackgroundColour` 改为读偏好里的主题
（或在 `ThemeProvider` 里 `Window.SetBackgroundColour(...)` 跟随切换）。

### 3.5 主题底色导致「窗体贴错位」的观感

- `src/App.css:11` 写死 `html { background-color: #0f0f0f }`，`:15` 的 `#root` 再写一次深色；
- Wails 侧 `BackgroundColour` 也是深色；
- 浅色主题下：窗口外框/首帧/滚动回弹都是深色 → 视觉上像「窗口和内容不是一个整体」。

**修法（双壳共用）**：`html`/`#root` 改用 `var(--background)`，并把首帧底色改成
`prefers-color-scheme` 或 localStorage 里已存的主题（现在 `ThemeProvider` 挂载后才设 class，
存在一帧闪烁）。

### 3.6 壳层差异清单（`App.tsx` vs `AppShell.tsx`）

| 启动步骤                     | Tauri `src/App.tsx`    | Wails `src/wails/AppShell.tsx` | 影响              |
| ---------------------------- | ---------------------- | ------------------------------ | ----------------- |
| `hideAppLoader()`            | ✅（`showMainWindow`） | ⚠️ 在 `main.tsx` 用 `rAF` 隐藏 | 等价但两处实现    |
| `window.show() + setFocus()` | ✅                     | ❌                             | 焦点可能不在窗口  |
| `initializeCommandSystem()`  | ✅                     | ✅                             | 一致              |
| `preloadShellEnv()`          | ✅                     | ✅                             | 一致              |
| `cleanupOldFiles()`          | ✅                     | ✅                             | 一致              |
| 语言初始化 + 菜单            | `buildAppMenu()`       | `initWailsMenu()`              | 各自实现          |
| `LegacyConfigDialog`         | ✅                     | ✅（上一轮补上）               | 一致              |
| `TooltipProvider`            | ❌                     | ✅                             | 两壳 DOM 结构不同 |
| 更新检查                     | ✅（含定时器）         | 无（设计例外）                 | 有意为之          |

**建议**：抽 `src/shell/startup.ts`（平台无关的启动序列 + loader 隐藏 + show/focus），两壳只保留
「菜单构建」和「更新检查」的分歧点。见 §6-T8。

---

## 4. 剩余已知需求（汇总 + 排期建议）

把 `wails/README.md` 的 Known gaps、guidelines §3.4/§3.8/§3.10 与 §9 未完成项合并，按「用户可感知」排序。
**下表只给优先级与方向；每一项的具体改动点（改哪个文件、改成什么代码、怎么验收）见 §9 方案卡片。**

| 序  | 需求                                               | 出处                      | 方案要点                                                                                                              | 建议期 |
| --- | -------------------------------------------------- | ------------------------- | --------------------------------------------------------------------------------------------------------------------- | ------ |
| 1   | Tailwind `@source`（本文 P0-1）                    | 本文 §2                   | `src/App.css` 加 `@source './**/*.{ts,tsx}'`                                                                          | P0     |
| 2   | 依赖与构建自愈（本文 P0-2/P0-3）                   | 本文 §1                   | `npm ci`；CI 加「前端生产构建 + CSS 断言」；`frontend/dist/.gitkeep` 或先建前端再跑 Go                                | P0     |
| 3   | 标题栏双击最大化（Windows）                        | 本文 §3.2                 | `TitleBar` 上 `onDoubleClick` → `getCurrentWindow().toggleMaximize()`（走 shim；**仅 Wails 启用**，Tauri 已原生处理） | P1     |
| 4   | 菜单在 frameless 下的承载方式                      | 本文 §3.2                 | 标题栏窗口内菜单按钮 + 复用 Go 菜单 action ID                                                                         | P1     |
| 5   | 窗口几何/主题底色对齐                              | 本文 §3.4/§3.5            | 统一窗口契约表；`SetBackgroundColour` 跟随主题                                                                        | P1     |
| 6   | 系统字体 shim（终端字体下拉为空）                  | guidelines §3.4           | Go 枚举字体（Windows 注册表 / macOS CoreText / Linux `fc-list`），shim 保持同名 API                                   | P1     |
| 7   | 平台 shim（`tauri-os.ts` 硬编码 `windows`）        | guidelines §3.10          | 从 Go 注入平台信息（启动时 `Events.Emit` 或 `window._wails.environment`），不再猜 UA                                  | P1     |
| 8   | 拖拽 opt-out 清单                                  | 本文 §3.3                 | `wails.css` 扩展选择器 + `data-no-drag`                                                                               | P2     |
| 9   | updater 决策（Tauri-only 还是重做）                | guidelines §3.7           | 短期：`isUpdaterSupported()` 门控全部入口（现已落地，确认无遗漏）；长期：`pkg/updater` + github provider              | P2     |
| 10  | TUI / CLI 层（`internal/service` 下沉 + `dg` CLI） | guidelines §9 P2-1 / P2-3 | Go 侧抽 service，TUI 二选一（冻结 Rust TUI / Go 重写），并据此改写 AGENTS.md 规则 9                                   | P2     |
| 11  | macOS / Linux 打包与实机验证                       | guidelines §3.10 / §10    | `build/{darwin,linux}` 资产、`MacOptions`、WebKitGTK 拖拽验证                                                         | P3     |

---

## 5. 验收与回归守卫（防止下一轮再漂）

### 5.1 手动验收（每次改壳层必做）

1. `wails3 task run`：左栏 280px 定宽、标题栏 32px、按钮有底色/圆角/悬停态；
2. 拖动标题栏移动窗口；双击标题栏最大化/还原；四边四角缩放；最小尺寸 1150×700 生效；
3. 浅色主题下窗口边缘与首帧无深色闪烁；
4. 打开 Preferences / CommandPalette / 一个工具页 / 终端页（xterm 字体与尺寸正常）。

### 5.2 CI 增项（`wails.yml`，建议顺序）

```yaml
- run: npm ci
- name: Install wails3
  run: go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
- name: Generate bindings
  working-directory: wails
  run: wails3 generate bindings -clean=true -ts -i
- name: Build frontend (creates frontend/dist for go:embed)
  run: npx vite build --config wails/vite.config.ts --mode production
- name: Tailwind utilities guard
  shell: bash
  run: |
    css=$(ls wails/frontend/dist/assets/index-*.css)
    grep -q '\.h-screen' "$css" || { echo "missing .h-screen → Tailwind 没扫到 src/"; exit 1; }
    grep -q '280px' "$css"     || { echo "missing w-[280px] → 侧栏宽度丢失"; exit 1; }
    echo "css=$(basename "$css") size=$(wc -c <"$css") bytes"
- name: Go static checks and tests # 必须排在上一步之后
  working-directory: wails
  shell: bash
  run: |
    fmt="$(gofmt -l .)"; [ -z "$fmt" ] || { echo "gofmt: $fmt"; exit 1; }
    go vet ./... && go test ./...
- name: Dependency drift
  run: npm ls --depth=0 >/dev/null
```

要点：

- **Go 的 `vet`/`test` 必须在前端产物之后**（`go:embed all:frontend/dist`）——当前 CI 顺序相反，必然红；
- `grep .h-screen` 是**成本最低的 Tailwind 回归哨兵**（本轮两个症状都能被它挡住）；
- `npm ls --depth=0` 会在 `invalid/UNMET` 时非零退出 → 挡住本轮 §1 那种漂移。

### 5.3 建议的本地脚本（写进 docs/agents/commands.md）

```bash
npm ci                                                        # 0. 同步依赖（必须）
cd wails && wails3 generate bindings -clean=true -ts -i && cd ..   # 1. 生成绑定（gitignored）
npx vite build --config wails/vite.config.ts --mode production     # 2. 前端产物（go:embed 需要）
cd wails && go vet ./... && go test ./... && cd ..                 # 3. Go 校验
cd wails && wails3 task run                                        # 4. 实机
```

---

## 6. 回馈 Tauri 版的问题清单

以下 6 条都是**共性问题**（不是 Wails 独有），建议在 Tauri 分支上单独立项：

| #   | 问题                                                                                                                                | 证据                                                                              | 建议                                                                                   |
| --- | ----------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- |
| T1  | `npm run check:all` **不包含前端生产构建**，所以「包导出缺失」（如 opener 改名）只能等打包时才发现                                  | 本轮：`npx vite build` 因 `revealItemInDir` 失败，而 typecheck/lint/test 各有盲区 | `check:all` 加 `npm run build`（或新增 `build:check`），CI 同步                        |
| T2  | 插件 API 名与版本耦合无记录：`revealItemInDir`（v2）→ `revealItemsInDir`（v3 alpha），且 `src/test/setup.ts:160` 的 mock 用的是旧名 | `node_modules` 装成 3.0.0-alpha 后构建失败                                        | 在 `docs/developer/tauri-plugins.md` 维护「插件 → 版本 → 符号」对照表；升级时同步 mock |
| T3  | 首帧底色写死深色：`src/App.css:11/15` 的 `#0f0f0f`，与主题变量脱耦；`ThemeProvider` 挂载后才加 `dark` class                         | 浅色主题下窗口边缘/首帧深色                                                       | 改用 `var(--background)` + `prefers-color-scheme`/localStorage 预置                    |
| T4  | 布局盒模型冲突：`body{min-height:100vh; min-width:100vw}` + `#root{min-height:100vh}` 与 `height:100%` 链混用                       | `src/App.css:39-40`（Wails 的 `wails.css` 只能再叠一层 `height:100%`）            | 统一 `html,body,#root{height:100%}` + `overflow:hidden`，删除 `100vw/100vh`            |
| T5  | Tailwind 依赖隐式内容探测（跟 Vite root 走）→ 任何换 root 的构建（Wails、未来的 Storybook/Electron）都会静默丢样式                  | 本文 §2                                                                           | `src/App.css` 显式 `@source './**/*.{ts,tsx}'`，把「碰巧正确」变成「显式正确」         |
| T6  | `data-tauri-drag-region="deep"` 是唯一拖拽契约，把共享组件绑死 Tauri 语义（继承 + 子元素 opt-out）                                  | `src/components/titlebar/TitleBar.tsx:56,84`；Wails 侧靠 CSS 映射                 | 抽象为 `data-drag-region="deep"` + `data-no-drag`，两壳各自映射到自己的原语            |

附赠一条**待清理**（不算 bug，但会误导下一次移植）：
`src/lib/capabilities.ts` 的 `isToolSupported()` 在两端都恒为 `true`（`WAILS_TOOLS` 含全部 11 个视图），
`isSessionsSupported()` 只排除 `channels` → 能力矩阵已成死代码。要么删掉，要么让它成为**真实的**能力声明
（「哪些视图由后端实现」），否则下次移植会照着一个恒真的函数做判断。

---

## 7. Agent 配置更新建议（本轮不改，仅给稿）

**现状**：`AGENTS.md`、`CLAUDE.md`、`docs/agents/*.md` 里 `wails` 出现 **0 次**；所有规则都是单壳（Tauri）视角。
上一轮的 Wails 知识全部沉在 `docs/developer/wails-port-guidelines.zh.md`（46 KB），
不符合本仓库自己的「渐进披露」规则（AGENTS.md 规则 1）。

### 7.1 `AGENTS.md`（建议新增/修改，草案）

```markdown
## Dual Shell (Tauri + Wails v3)

`src/` 是两条壳共用的组件树。除 `src/wails/` 与 `src-tauri/` 外，**任何 `src/**` 改动都要同时评估两壳\*\*。

1. **共享代码不得直接依赖未桥接的 Tauri 能力**：新用到的 `@tauri-apps/plugin-*` 必须同时提供
   `src/wails/shims/` 映射（见 `wails/PORTING.md`「禁止 no-op shim」），否则用 `capabilities.ts` 门控。
2. **命令面三处对齐**：Rust command ↔ Go method ↔ shim 映射 ↔ 生成的 TS 绑定，缺一即为缺陷
   （`node scripts/check-command-surface.mjs` 会把关）。
3. **Wails 侧验证顺序**：`npm ci` → `wails3 generate bindings -ts -i` → `npx vite build -c wails/vite.config.ts`
   → `go vet/test` → `wails3 task run`。跳过第 2 步会直接构建失败（绑定目录是 gitignored）。
4. **样式改动必须检查两壳的 CSS 产物**：Wails 构建的 Vite `root` 是 `wails/`，Tailwind v4 的自动内容探测
   会漏掉 `src/`。`src/App.css` 必须保留显式 `@source './**/*.{ts,tsx}'`。
5. **Tauri-only 面（自动更新）**：不得在 Wails 下留下会失败的入口，用 `isUpdaterSupported()` 门控。
```

并把核心规则 9 改为：

```markdown
9. **三端一致（GUI-Tauri / GUI-Wails / TUI）** — 新增 GUI 功能时同步 Wails 侧后端（Go）
   与 TUI 侧入口；无法同步的必须在 `capabilities.ts` / 文档中显式标注为「不支持」。
```

核心规则 6 追加一句：`Wails v3 docs only（v2 与 v3 的 API 不兼容）`。

### 7.2 `docs/agents/commands.md`（必须补，现在完全没有 Wails）

````markdown
## Wails (Go shell)

```bash
npm ci                                                             # 先同步依赖（@wailsio/runtime 等）
cd wails && wails3 generate bindings -clean=true -ts -i            # gitignored，构建前必跑
npx vite build --config wails/vite.config.ts --mode production     # frontend/dist（go:embed 依赖）
cd wails && go vet ./... && go test ./...
cd wails && wails3 task run     # 实机运行（内嵌已构建的前端产物）
cd wails && wails3 dev          # 改 Go / 改前端 → 自动重建 + 重启（无 Vite HMR）
node scripts/check-command-surface.mjs          # 命令面漂移
node wails/scripts/sync-version.mjs --check     # 版本号漂移
node wails/scripts/sync-registry.mjs            # 共享数据 vendoring（改 registry/schema 后必跑）
```
````

同时补一条**已知盲区**：`npm run check:all` 不含前端生产构建，也不含上述 Wails 步骤（见 §6-T1）。

### 7.3 新建 `docs/agents/wails-port.md`（渐进披露入口）

一页纸即可，指向 46 KB 的 guidelines 里「只有 agent 会踩的坑」：

- 绑定是生成的、gitignored、构建前必须生成；
- `frontend/dist` 缺失会让 `go vet` 直接失败（`go:embed`）；
- Tailwind `root` 陷阱（§2）；
- 错误契约：Go `error` → 原始字符串（与 tauri-specta `Err(String)` 对齐），不要包 JSON；
- 未知字段必须透传（`json.RawMessage` / 保留 map 顺序）——配置往返不能丢键；
- 新增/删除命令后必须跑对账脚本，并同步 `wails/frontend/bindings`；
- 不要写 no-op shim（`emit()` 空实现之类的假对齐）。

### 7.4 `.agents/skills/register-model/SKILL.md`（**必须补，会产生 CI 红灯**）

现在只写 `src/lib/model-registry-data.json`，但 `wails/internal/core/registrydata/model-registry-data.json`
是 `go:embed` 的副本，CI 有 `cmp` 漂移检查。技能里应加一步：

```bash
node wails/scripts/sync-registry.mjs   # 把 registry 与 dsh schema 同步到 Go 模块
```

### 7.5 其他配置项

| 文件                                      | 问题                                                                                                                | 建议                                                                                                          |
| ----------------------------------------- | ------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| `knip.json`                               | `entry` 只有 `src/main.tsx` → `src/wails/**` 15 个文件被报「未使用」，`@wailsio/runtime` 被报「未使用依赖」（实测） | `entry` 加 `src/wails/main.tsx`；`ignore` 加 `src/wails/shims/**`；`ignoreDependencies` 加 `@wailsio/runtime` |
| `sgconfig.yml`                            | `wails//` 已在忽略列表（✅ 正确，生成物不进 ast-grep）                                                              | 保持                                                                                                          |
| `.agents/skills/release/SKILL.md`         | 发布流程没有 `sync-version`，Wails 安装器元数据会落后                                                               | 在 `release:prepare` 前加 `node wails/scripts/sync-version.mjs`                                               |
| `CLAUDE.md`                               | 只有指针，可接受                                                                                                    | 若要更严：加一行「Wails 验证见 docs/agents/commands.md」                                                      |
| 新增脚本 `scripts/check-shim-surface.mjs` | 目前没有任何守卫能发现「共享代码调了插件、但 shim 没导出该符号」（本轮 opener 就是这么坏的）                        | 扫描 `src/**` 的 `@tauri-apps/*` 命名导入，逐个核对 `src/wails/shims/*` 的导出，白名单显式声明                |

---

## 8. 附录 A：本轮证据命令

```bash
# A. 依赖漂移
npm ls --depth=0 | grep -E "invalid|UNMET"

# B. Tauri 前端构建（暴露 opener 符号缺失）
npx vite build

# C. 绑定 + Wails 前端构建
cd wails && wails3 generate bindings -clean=true -ts -i && cd ..
npx vite build --config wails/vite.config.ts --mode production

# D. CSS 生成量对账
ls -l wails/frontend/dist/assets/index-*.css          # 45.19 kB（坏） / 135.96 kB（加 @source 后）
grep -c 'h-screen' wails/frontend/dist/assets/index-*.css

# E. Go 在无前端产物时的表现
cd wails && go vet ./...   # pattern all:frontend/dist: no matching files found

# F. Wails 菜单与 frameless 的关系
grep -n "if !options.Frameless && !options.Windows.DisableMenu" \
  "$(go env GOMODCACHE)/github.com/wailsapp/wails/v3@v3.0.0/pkg/application/webview_window_windows.go"
# 双击最大化只在 macOS 处理
grep -n "IsMac()" \
  "$(go env GOMODCACHE)/github.com/wailsapp/wails/v3@v3.0.0/internal/runtime/desktop/@wailsio/runtime/src/drag.ts"
```

> 说明：本文所有「实测」结论均在恢复 `src/App.css` / `src/wails/wails.css` 原样、删除
> `wails/frontend/dist` 之后得出；工作区最终 `git status` 干净。

---

## 9. 附录 B：逐项修补方案（可直接执行）

每张卡片固定四段：**现状 → 改哪里 → 怎么改 → 验收**。卡片编号与 TL;DR / §4 对应。
本轮**未执行任何一张卡片**，代码保持原样。

| 卡片 | 对应问题          | 改动面                                                  | 预估    |
| ---- | ----------------- | ------------------------------------------------------- | ------- |
| B1   | P0-1 布局         | `src/App.css`（1 行）                                   | 5 min   |
| B2   | P0-2 依赖漂移     | 无代码；`npm ci` + CI 门 + 约定                         | 15 min  |
| B3   | P0-3 go:embed     | `wails/.gitignore` + `frontend/dist/.gitkeep` + CI 顺序 | 10 min  |
| B4   | 双击最大化        | `src/components/titlebar/TitleBar.tsx`                  | 30 min  |
| B5   | frameless 菜单    | `TitleBar`/`TitleBarContent` + 可选 Go                  | 0.5–1 d |
| B6   | 窗口契约+主题底色 | `wails/main.go` + 新 `src/wails/theme-window.ts`        | 2 h     |
| B7   | 系统字体          | 新 `wails/internal/app/fonts.go` + shim + 别名          | 0.5 d   |
| B8   | 平台 shim         | `src/wails/shims/tauri-os.ts`（重写）                   | 1 h     |
| B9   | 拖拽 opt-out      | `src/wails/wails.css` + 几处 `data-no-drag`             | 20 min  |
| B10  | updater 决策      | 文档 + `capabilities.ts` 审计                           | 1 h     |
| B11  | TUI / CLI         | 决策记录（ADR）+ 后续分期                               | 0.5 d   |

### B1 — Tailwind 内容源（P0-1，必做第一步）

- **现状**：`wails/vite.config.ts:22` `root: dir`（= `wails/`），Tailwind v4 自动探测跟着 root 走 → `src/` 的 class 全部丢失。
- **改哪里**：`src/App.css`（共享文件，唯一改动点）。
- **怎么改**：

```diff
  @import 'tailwindcss';
  @import 'tw-animate-css';
  @import './theme-variables.css';
  @import 'sonner/dist/styles.css';

+ /* Tailwind v4 的自动探测以 Vite root 为基准：Tauri 构建 root=仓库根（碰巧对），
+    Wails 构建 root=wails/（探测不到 src/）。显式声明后与构建入口解耦。 */
+ @source './**/*.{ts,tsx}';
+
  @source '../node_modules/streamdown/dist/*.js';
```

- **不要**把 `@source` 写进 `src/wails/wails.css`（已实测无效：没有 `@import "tailwindcss"` 的文件不参与生成）。
- **验收**：

```bash
npx vite build --config wails/vite.config.ts --mode production
css=$(ls wails/frontend/dist/assets/index-*.css); grep -q '\.h-screen' "$css" && echo OK
# 体积应从 45 kB 级跳到 130 kB 级
```

### B2 — 依赖漂移的自愈与防再发（P0-2）

- **现状**：`node_modules` 里 `@tauri-apps/*` 全是 `3.0.0-alpha.*`（声明 `^2.x`），`@wailsio/runtime`、`@typescript/native-preview` 缺失 → 三条构建链路全断。
- **改哪里**：不改代码；改**流程与守卫**。
- **怎么改**：
  1. `npm ci`（**不要**用 `npm install <单包>` 局部“救火”，那正是产生本次漂移的方式）；
  2. 约定：任何插件升级必须同步 `package.json` + `package-lock.json` + `src/test/setup.ts` 的 mock（例：`revealItemInDir` → `revealItemsInDir`）；
  3. CI 加一步 `npm ls --depth=0`（`invalid`/`UNMET` 时非零退出）。
- **验收**：

```bash
npm ls --depth=0 >/dev/null && echo clean   # npm ci 之后应为 clean
npx vite build && echo tauri-frontend-ok    # npm ci 之后应通过；现在失败是漂移所致
npx vite build --config wails/vite.config.ts && echo wails-frontend-ok
```

### B3 — `go:embed` 与构建顺序（P0-3）

- **现状**：`wails/main.go:16` `//go:embed all:frontend/dist`；干净克隆无此目录 → `go vet`/`go test` 直接失败。
- **改哪里**：`wails/.gitignore`、新增 `wails/frontend/dist/.gitkeep`、`.github/workflows/wails.yml`。
- **怎么改**：

```diff
# wails/.gitignore
- frontend/dist
+ frontend/dist/*
+ !frontend/dist/.gitkeep
```

- **实测**：只放一个 `frontend/dist/.gitkeep` 后 `cd wails && go build ./...` 通过（exit 0）。注意这只是让 Go 侧能编译，**运行时仍需要真实产物**；发布/CI 必须真构建前端。
- **CI 顺序**（必须）：`npm ci` → `wails3 generate bindings` → `vite build -c wails/vite.config.ts` → `go vet/test` → 其余门禁（见 §5.2）。
- **验收**：干净克隆 `cd wails && go vet ./... && go test ./...` 通过。

### B4 — Windows 标题栏双击最大化

- **现状**：Wails runtime 只在 macOS 处理 `dblclick`（`drag.ts` 的 `IsMac()` 守卫）；Tauri 由系统原生处理。
- **改哪里**：`src/components/titlebar/TitleBar.tsx`（Windows 分支与 macOS 分支的根 `div`）。
- **怎么改**（**必须只对 Wails 生效**，否则 Tauri 上会“最大化后又还原”）：

```tsx
import { IS_WAILS_BACKEND } from '@/lib/capabilities'

const handleTitleBarDoubleClick = (event: React.MouseEvent<HTMLDivElement>) => {
  if (!IS_WAILS_BACKEND) return // Tauri: 系统原生处理
  const target = event.target as HTMLElement
  // 交互元素不触发（与 wails.css 的 opt-out 列表保持一致）
  if (target.closest('button, a, input, select, textarea, [role="button"], [data-no-drag]')) return
  void getCurrentWindow().toggleMaximize()
}

<div data-tauri-drag-region="deep" onDoubleClick={handleTitleBarDoubleClick} … />
```

- **注意**：`getCurrentWindow()` 已在 Wails 侧映射到 `Window.ToggleMaximise()`（`src/wails/shims/tauri-webview-window.ts`），不需要新 shim。
- **验收**：Wails 下双击标题栏空白处最大化/还原；双击标题栏里的按钮**不会**最大化；Tauri 下行为不变。

### B5 — frameless 模式下的菜单承载

- **现状**：`Frameless: true` 时 Wails 在 Windows/Linux 根本不挂应用菜单（`webview_window_windows.go:525`），Go 侧 `internal/app/menu.go` + `menu-bridge.ts` 只在 macOS 有实际效果。
- **改哪里**：前端为主；**推荐方案 A**。
- **方案 A（推荐，0 Go 改动）**：在 `TitleBarRightActions` 里加一个菜单按钮，用 shadcn `DropdownMenu`（已有）复刻 `src/lib/menu.ts` 的条目，动作走**已存在**的 handler（`menu-bridge.ts` 里的 `useUIStore` 动作 / `executeCommand`），渲染条件 `IS_WAILS_BACKEND && platform !== 'macos'`。
- **方案 B（复用 Go 菜单，改动大）**：Go 侧 `app.ContextMenu.Add("app-menu", menu)` 注册同一套条目，前端在按钮上设 CSS 变量 `--custom-contextmenu: app-menu` 并 `dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, clientX, clientY }))`（runtime 的 contextmenu handler 就是这么取菜单名的）。缺点：菜单树要维护两份（`application.Menu` 与 `ContextMenu` 不是同一类型）。
- **验收**：Windows 下能从窗口内打开 Preferences / 切换侧栏 / Reset window；macOS 菜单栏行为不变；`menu:*` 事件不再有两套实现。

### B6 — 窗口契约与主题跟随底色

- **现状**：`1280×840` / 最小 `800×600` / 立即显示 / `BackgroundColour: NewRGB(6,7,15)` 写死。
- **改哪里**：`wails/main.go`（几何 + 启动底色）、新增 `src/wails/theme-window.ts`（跟随切换）。
- **怎么改**：
  1. 几何与 Tauri 对齐（或反向统一 Tauri，二者选一后**写进契约表**）：`Width/Height: 1150×700`、`MinWidth/MinHeight: 1150×700`；
  2. 启动底色读偏好：Go 侧已有 `AppPreferences.Theme`（`app.go:36`，默认 `system`）→ `light` 用 `255,255,255`，否则 `15,15,15`；
  3. 运行期跟随（**不改共享 `ThemeProvider`**）：

```ts
// src/wails/theme-window.ts
import { Window } from '@wailsio/runtime'

const LIGHT = [255, 255, 255] as const
const DARK = [15, 15, 15] as const

/** 监听 documentElement 的 light/dark class，同步窗口底色（避免浅色主题下的黑边/闪烁）。 */
export function syncWindowBackground(): () => void {
  const root = document.documentElement
  const apply = () => {
    const [r, g, b] = root.classList.contains('dark') ? DARK : LIGHT
    void Window.SetBackgroundColour(r, g, b, 255)
  }
  apply()
  const observer = new MutationObserver(apply)
  observer.observe(root, { attributes: true, attributeFilter: ['class'] })
  return () => observer.disconnect()
}
```

- **前置**：`@wailsio/runtime` 必须真的装上（见 B2），否则 `Window.SetBackgroundColour` 不存在。
- **验收**：浅色/深色主题来回切换，窗口边缘与首帧无深色残留；重启后几何与最小尺寸符合契约表。

### B7 — 系统字体（终端字体下拉为空）

- **现状**：`src/components/preferences/panes/AppearancePane.tsx:4` `import { getSystemFonts } from 'tauri-plugin-system-fonts-api'`；Wails 侧无别名 → 异常被 catch → 空列表。
- **改哪里**：新增 `wails/internal/app/fonts.go`、`src/wails/shims/tauri-system-fonts.ts`、`wails/vite.config.ts` 别名。
- **怎么改**：
  1. Go：`func (a *App) GetSystemFonts() ([]string, error)`
     - Windows：`golang.org/x/sys/windows/registry` 读 `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts` 的 value 名（去掉 `(TrueType)` 后缀）；
     - macOS：`ls /System/Library/Fonts /Library/Fonts` + `system_profiler SPFontsDataType -json`（慢，建议缓存）；
     - Linux：`fc-list : family` 去重排序。
  2. shim：与 npm 包同签名 `export async function getSystemFonts(): Promise<string[]>`（错误转原始字符串，与其他 shim 一致）；
  3. `wails/vite.config.ts` 别名表加 `'tauri-plugin-system-fonts-api': shim('tauri-system-fonts.ts')`。
- **验收**：`AppearancePane` 字体下拉有内容；`npm run test:run` 不受影响（`src/test/setup.ts` 里的 mock 仍指向 npm 包）。

### B8 — 平台 shim（`tauri-os.ts` 硬编码 `windows`）

- **现状**：`platform()`/`family()` 恒返回 `windows`；`version()` 返回整条 UA。macOS/Linux 会渲染出 Windows 三键标题栏。
- **改哪里**：`src/wails/shims/tauri-os.ts`（重写）。
- **怎么改**：Wails runtime 已提供平台信息，**不要**再猜 UA：

```ts
type WailsOS = 'windows' | 'darwin' | 'linux' | 'ios' | 'android' | undefined

/** runtime 在页面脚本之前注入 window._wails.environment；未就绪时回退 UA。 */
function currentOS(): WailsOS {
  const injected = (window as { _wails?: { environment?: { OS?: string } } })
    ._wails?.environment?.OS
  if (injected) return injected as WailsOS
  const ua = navigator.userAgent
  if (/Mac/i.test(ua)) return 'darwin'
  if (/Linux/i.test(ua)) return 'linux'
  return 'windows'
}

export function platform(): 'windows' | 'macos' | 'linux' {
  const os = currentOS()
  if (os === 'darwin') return 'macos'
  if (os === 'linux') return 'linux'
  return 'windows'
}
```

- `locale()` 保留 `navigator.language`；`version()` 可改用 `System.Environment()`（异步）或用 Go 注入，但**不要让同步 API 变成异步**（`use-platform` 与 `TerminalView` 都是同步调用）。
- **验收**：macOS 上 `TitleBar` 走左侧红绿灯分支、Linux 走 `LinuxTitleBar`；`npm run test:run -- src/hooks/use-platform.test.ts` 通过。

### B9 — 拖拽区 opt-out 补全

- **现状**：`wails.css` 只 opt-out 了 6 个选择器；`TitleBarContent` 里的可点击 `div`/下拉项仍会被当成拖拽区。
- **改哪里**：`src/wails/wails.css`（可选：给共享组件的可点击 `div` 加 `data-no-drag`）。
- **怎么改**：

```css
[data-tauri-drag-region] button,
[data-tauri-drag-region] [role='button'],
[data-tauri-drag-region] [role='menuitem'],
[data-tauri-drag-region] [role='combobox'],
[data-tauri-drag-region] a,
[data-tauri-drag-region] input,
[data-tauri-drag-region] select,
[data-tauri-drag-region] textarea,
[data-tauri-drag-region] [tabindex]:not([tabindex='-1']),
[data-tauri-drag-region] [data-dropdown-menu-trigger],
[data-tauri-drag-region] [data-no-drag] {
  --wails-draggable: none;
}
```

- **验收**：在标题栏里的每个可点击元素上按下并移动鼠标 → 元素不跟随拖动、窗口不动；空白处拖动 → 窗口移动。

### B10 — updater 的最后一次决策 + 校验

- **现状**：`isUpdaterSupported()` 已落地（Wails 一律 false），但历史上有多处入口（Preferences → General、命令面板 dev 项、`services/updater.ts`）。
- **改哪里**：只做审计，不改行为（除非发现遗漏）。
- **怎么改**：`grep -rn "updater\|checkForUpdate\|getUpdateChannel" src --include=*.ts --include=*.tsx`，逐个确认被 `isUpdaterSupported()` 或 `IS_WAILS_BACKEND` 包住；结论写回 §4 表格与 `wails/README.md`。长期若要做：`pkg/updater` + `providers/github` + `latest.json` 通道，按 §4 序 9 另立需求。
- **验收**：Wails 下 UI 里不存在任何会抛 `unsupported command` 的入口；`wails/README.md` 明确写「更新器为 Tauri-only（设计例外）」。

### B11 — TUI / CLI 的决策记录（ADR）

- **现状**：Go 新功能不流入 TUI；`Export Templates` 已在 GUI 落地但 TUI 缺失（AGENTS.md 规则 9 实质已破）；`internal/service` 与 `dg` CLI 仍是 P2。
- **改哪里**：新增 `docs/developer/adr-00X-tui-fate.zh.md`（1 页） + 同步改 AGENTS.md 规则 9。
- **怎么改**：三条路线任选其一并写清代价：
  1. **冻结 Rust TUI**：只修 bug，功能声明为「GUI only」；AGENTS.md 规则 9 改为「GUI(Tauri+Wails) 同步，TUI 冻结」。
  2. **Go 重写 TUI**：抽 `wails/internal/service`（现在 `App` 方法直接拼装写盘），CLI/TUI 复用；成本最高但规则 9 可真正成立。
  3. **只做 CLI**：`cmd/dg` 提供 `--json` 能力，TUI 仍冻结；对 agent/脚本场景收益最高。
- **验收**：ADR 合入 + AGENTS.md 规则 9 改写 + `wails/PORTING.md` 增加「CLI 契约」小节。
