# Wails 壳诊断：启动没有窗口 / 黑屏怎么查

- **状态**：已落地
- **日期**：2026-11
- **关联**：
  - [`wails/main.go`](../../wails/main.go) — 主窗口（`Hidden: true`）、看门狗
  - [`wails/preflight_windows.go`](../../wails/preflight_windows.go) — WebView2 预检 + 原生对话框
  - [`wails/webview_env.go`](../../wails/webview_env.go) — VM/RDP 用的两个环境变量开关
  - [`wails/internal/app/logging.go`](../../wails/internal/app/logging.go) — 日志落盘
  - [`wails/internal/core/paths.go`](../../wails/internal/core/paths.go) — `LogPath()`
  - 相关：[`wails-port-guidelines.zh.md`](./wails-port-guidelines.zh.md)（§P3 平台差异）

## 0. 症状 → 原因对照

窗口是**创建时隐藏、由前端显示**的（`main.go` 的 `Hidden: true`，`src/App.tsx`
调用 `CurrentWindow.Show()`）。所以「WebView 渲染失败」的典型表现是：

| 现象                              | 机制                                                                                                   |
| --------------------------------- | ------------------------------------------------------------------------------------------------------ |
| 第一次启动完全没反应              | 进程活着、窗口已创建，但 WebView2 没把前端渲染出来 → `App.tsx` 走不到 `Show()` → 窗口一直隐形          |
| 第二次启动是一块全黑的窗口        | 单实例逻辑命中 `OnSecondInstanceLaunch` → `win.Show()` 把那个空窗口显示出来，里面只有 `#0f0f0f` 背景色 |
| 关掉窗口后进程还在 / 起不来第二份 | 单实例 Named lock 仍被第一个进程持有                                                                   |

**结论**：这种症状从来不是 Go/二进制的问题（能建窗口、能拿到单实例锁），而是
**WebView2 环境**问题。GUI 是 `-H windowsgui` 子系统、没有控制台，所以以前
「什么都没打印」——现在日志落盘了。

## 1. 日志：`~/.droidgear/logs/app.log`

Wails 的系统日志（含它解析到的 WebView2 版本）与 `internal/app` 的 `log`
输出都镜像到这里，同时仍写 stderr：

```
时间 level=INFO msg="log started" path=... version=1.3.10.1 level=INFO goos=windows
时间 level=INFO msg="WebView2 runtime available" version=157.0.4341.42 arch=arm64
时间 level=INFO msg="Platform Info:" ... WebView2=157.0.4341.42 Name="Windows 11 ..."
时间 level=ERROR msg="frontend did not reveal the window before the grace period; ..."
```

- 位置：`~/.droidgear/logs/app.log`；超过 **4 MiB** 轮转一次到 `app.log.1`（只留一代）
- `DROIDGEAR_LOG_DIR=<dir>` 改写目录；`DROIDGEAR_LOG_LEVEL=debug|info|warn|error`（默认 `info`）
- `dg doctor` 的 `files.log` 报告同一路径（非 GUI 消费者，ADR-001）
- 日志文件打不开不会影响启动：退化为只写 stderr 并打一条 warn

`app.log` 里最值得看的三行：

| 关键行                                  | 含义                                                        |
| --------------------------------------- | ----------------------------------------------------------- |
| `WebView2 runtime available` / `Error:` | 预检结果；缺 runtime 时还会弹一个原生对话框                 |
| `Platform Info: ... WebView2=<版本>`    | Wails 自己解析到的 runtime 版本（与预检对不上说明路径问题） |
| `[AssetFileServerFS] Handling request`  | 前端资源请求；停在这里说明页面没发起 / 资源没找到           |

## 2. 启动预检（Windows）

`application.New` 之前跑 `webviewloader.GetAvailableCoreWebView2BrowserVersionString`：

- 有版本 → 记一条 `INFO`，正常启动
- 没有 → 记 `ERROR` + 弹原生 MessageBox（SystemModal），写明要装**匹配架构**的
  Evergreen Runtime（ARM64 二进制要 ARM64 runtime），给出下载页与 `--disable-gpu` 提示

预检**不会**中断启动：探针可能误判（per-user 安装、固定版本 runtime），
能跑起来就不该被拦。

## 3. 看门狗（8 秒）

`main.go` 在注册 `events.Common.WindowShow` 之后起一个 goroutine：8 秒内窗口
没有被 `Show()`，就由 Go 侧强制 `Show() + Focus()` 并记一条 `ERROR`。

效果：从「完全无反馈」变成「看得到窗口 + 日志里有原因」。注意它只能让失败可见，
真正的原因仍要看日志（通常是 WebView2 缺失 / 渲染进程崩溃）。

## 4. VM / 远程桌面：两个环境变量开关

| 变量                               | 作用                                                                                                                               |
| ---------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `DROIDGEAR_WEBVIEW_ARGS`           | 追加 Chromium 开关，空格分隔，例如 `--disable-gpu --disable-gpu-compositing`（映射到 `WindowsOptions.AdditionalBrowserArgs`）      |
| `DROIDGEAR_WEBVIEW_VISUAL_HOSTING` | `1/true/yes/on` → `WindowsOptions.UseVisualHosting`（Visual hosting 取代 HWND child，消除远程/虚拟显示下 DPI 重新 marshal 的卡顿） |

WebView2 自己也读 `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS`，所以**不重新构建**也能先试：

```powershell
$env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS = "--disable-gpu --disable-gpu-compositing"
.\droidgear-wails-windows-arm64.exe
```

## 5. Windows on ARM（Parallels / UTM 里的 Windows 11）

- 二进制是原生 ARM64（`windows/arm64`，`CGO_ENABLED=0` 交叉编译），
  `go-webview2` 自带 `webviewloader/arm64/WebView2Loader.dll`
- **运行时必须是 ARM64 的 WebView2 Runtime**：只装了 x64 版等于没装。
  检查：`dir "$env:ProgramFiles(x86)\Microsoft\EdgeWebView\Application"`，
  没有就装 <https://developer.microsoft.com/microsoft-edge/webview2/> 的 ARM64 Evergreen 包
- 虚拟机没有 GPU 加速时优先试 `--disable-gpu`；仍黑屏再试
  `DROIDGEAR_WEBVIEW_VISUAL_HOSTING=1`
- 症状与排查顺序见 §0 → §1

## 6. 排查顺序（照这个顺序走，基本一次定位）

1. `~/.droidgear/logs/app.log` 最后几行 —— 有没有 `WebView2 ... Error`
2. `dir "%ProgramFiles(x86)%\Microsoft\EdgeWebView\Application"` —— runtime 在不在、架构对不对
3. `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--disable-gpu` 再启动 —— 是否是合成/GPU 问题
4. `DROIDGEAR_LOG_LEVEL=debug` —— 看 `[AssetFileServerFS]` 与 `Platform Info`
5. 仍无头绪：`dg doctor --json` 贴路径与版本，配合日志提 issue
