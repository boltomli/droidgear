# 单文件 Release 自更新（Self-update）

- **状态**：已落地（GUI + `dg update`）
- **日期**：2026-11
- **关联**：
  - [`.github/workflows/release-single.yml`](../../.github/workflows/release-single.yml) — 单文件流水线 + `SHA256SUMS.txt`
  - [`wails/internal/service/updater`](../../wails/internal/service/updater) — 实现（发现 → 下载 → 校验 → 替换 → 重启）
  - [`wails/internal/app/update.go`](../../wails/internal/app/update.go) — GUI 适配（事件 / 重启 / 启动清理）
  - [`wails/cmd/dg/update.go`](../../wails/cmd/dg/update.go) — `dg update`（ADR-001 的非 GUI 消费者）
  - [`wails-tui-cli-design.zh.md`](./wails-tui-cli-design.zh.md) §2（分层）、§4.4（CLI 契约）
  - [`tauri-removal-design.zh.md`](./tauri-removal-design.zh.md) D3（Tauri 更新器删除的历史决策）

## 0. 一句话

发布形态是**单文件可执行文件**（无安装器、无归档），所以「更新」= 从 GitHub 的
`wails-v*` Release 里取回**当前平台那一个文件**，用 Release 附带的
`SHA256SUMS.txt` 校验，然后把自己替换掉；GUI 再自动重启，`dg` 下次运行即生效。

## 1. 为什么是「单文件 release」

| 事实                                | 后果                                                    |
| ----------------------------------- | ------------------------------------------------------- |
| `go:embed all:frontend/dist`        | 前端 bundle 在 Go 二进制里，`go build` 就是一个完整应用 |
| 每平台只有一个资产                  | 没有安装器/解包/签名清单，更新逻辑只需处理「一个文件」  |
| 资产名稳定（见 §2）                 | 更新器不需要 manifest，直接从 Release 资产名匹配平台    |
| 运行时依赖仍是系统 WebView（见 §2） | 更新不涉及依赖安装，「替换二进制」就是全部              |

## 2. 资产命名与平台矩阵

`release-single.yml` 的 matrix 与 `internal/service/updater` 的 `releaseAssets`
必须同步（改一处必须改两处，PR 里一起改）：

| AssetKind | windows/amd64                     | windows/arm64                       | darwin/arm64                  | linux/amd64                 |
| --------- | --------------------------------- | ----------------------------------- | ----------------------------- | --------------------------- |
| `gui`     | `droidgear-wails-windows-x64.exe` | `droidgear-wails-windows-arm64.exe` | `droidgear-wails-macos-arm64` | `droidgear-wails-linux-x64` |
| `cli`     | `dg-windows-x64.exe`              | `dg-windows-arm64.exe`              | `dg-macos-arm64`              | `dg-linux-x64`              |

未列出的组合返回 `ErrUnsupportedPlatform`（不猜、不错装）。

> **macOS 目前默认不发布（2026-11 起）**：`build_macos` 默认 `false`，标签触发
> （`push: tags: wails-v*`）拿不到 inputs，因此**只构建 windows x64 / windows arm64 /
> linux x64**。需要在某个版本带上 macOS 时，用 `workflow_dispatch` 手动指定
> `build_macos=true`（会走 `macos-15` runner，产物仍是 ad-hoc 签名、未公证）。
> 资产表（`releaseAssets`）里 darwin/arm64 一行保持不变——重新开启发布不需要改 Go
> 代码；在 macOS 上跑 `dg update` 只会看到「本平台资产缺失」的明确错误。

Windows arm64 与 windows amd64 在**同一个 x64 runner 上交叉编译**（Windows 后端无
CGO，`go-webview2` 自带 `webviewloader/arm64/WebView2Loader.dll`），所以不需要
`windows-11-arm` runner。运行时依赖是 **ARM64 WebView2 Runtime**（Windows 11 arm64
预装）；在 x64 Windows 上请继续用 x64 资产。

`SHA256SUMS.txt` 由流水线的 `checksums` job 在**所有平台构建完成之后**统一生成：
三个 runner 互相看不到文件，所以下载已上传的资产、单点计算 `sha256sum`、再上传。
没有 `SHA256SUMS.txt` 的 Release **不会被提供**——无法校验的字节不允许替换可执行文件。

## 3. 更新流程

```
Check   ── GET /repos/{repo}/releases?per_page=30
           ├─ 跳过 draft / 非 `wails-v*` tag
           ├─ 渠道过滤：stable 跳过 GitHub 标记的 prerelease；preview（默认）两者都要
           ├─ 跳过没有「本平台资产 + SHA256SUMS.txt」的 Release
           └─ 取 semver 最大者（1.3.9.4 < 1.3.9.11：按数字段比较，句点段补齐 0）
              —— 开启 allowDowngrade 时，更旧的目标渠道最新版也算可选项（见 §3.0.1）

Download ── 取同一 Release 的 SHA256SUMS.txt → 得到期望哈希
           ├─ 流式下载到 <cache>/<version>/<asset>.part（进度事件 ~10/s）
           ├─ 字节数 == API 报告的 size
           ├─ sha256 == SHA256SUMS.txt
           └─ 落盘为 <cache>/<version>/<asset>，写入 state.json 的 pending

Apply   ── 重新校验 pending 文件的哈希
           ├─ 版本门槛：pending 必须比当前新；更旧只在 AllowDowngrade（切回旧渠道）时允许
           ├─ 复制到 <exe>.new（同目录，写满并 fsync，半成品永远不会变成可执行文件）
           ├─ 交换：
           │    Windows: exe → exe.old，exe.new → exe（失败则回滚 old）
           │    POSIX  : rename(exe.new, exe) —— 原子替换，运行中的进程继续用旧 inode
           ├─ GUI: 以 `--apply-update-relaunch <pid>` 启动新二进制，500ms 后退出
           └─ 清空 pending，删除暂存目录
```

缓存目录：`os.UserCacheDir()/droidgear/updates`（可用 `DROIDGEAR_UPDATE_CACHE` 覆盖）。
`state.json` 里同时记录 `dismissedVersion`（用户忽略过的版本，启动横幅不再打扰）。

### 3.0 渠道：preview（默认）/ stable

GitHub Release 的 `prerelease` 标记就是渠道标记。版本号不编码渠道（预览版和正式版
都是 `x.y.z.w`），它是**发布时的决定**（见 §5 与 `release-single.yml`）：

| 发布方式                    | 例子             | GitHub `prerelease` | 渠道      |
| --------------------------- | ---------------- | ------------------- | --------- |
| tag push（无 inputs）       | `wails-v1.4.0.1` | `true`（默认）      | `preview` |
| dispatch `prerelease=true`  | `wails-v1.4.0.1` | `true`              | `preview` |
| dispatch `prerelease=false` | `wails-v1.4.1.0` | `false`             | `stable`  |

| 渠道              | 接受哪些 Release    | 说明                                           |
| ----------------- | ------------------- | ---------------------------------------------- |
| `preview`（默认） | prerelease + 正式版 | 与流水线实际节奏一致（日常 tag push 就是预览） |
| `stable`          | 仅正式版            | 只有预览版时表现为「已是最新」                 |

把已发布的预览版转成正式版：对同一 tag 再跑一次 `workflow_dispatch` 并设
`prerelease=false`（`release` job 会 `gh release edit --prerelease=false` 并重传资产），
或在发布草稿时手动取消「Set as a pre-release」。

新旧比较仍然只是 semver：渠道只决定「看不看得到」，不改变「谁更新」。因此 preview
用户也会正常升级到更新的 stable 版本；stable 用户永远不会收到 prerelease。

取值优先级（`updater.resolveChannel`）：

1. 显式值（`dg update --channel` / 调用方传入的 `Config.Channel`）
2. 环境变量 `DROIDGEAR_UPDATE_CHANNEL`（`preview` | `stable`）
3. `preferences.json` 的 `updates_channel`（Preferences → General 的渠道下拉、TUI
   Preferences 的 `c` 键）——未设置/未知值 = `preview`

> 为什么默认是 preview：单文件流水线日常只发预览构建，而 GitHub 对非 semver 的 tag
> 默认勾上「Set as a pre-release」。早期实现「prerelease 一律跳过」，结果所有
> `wails-v*` Release 都被跳过，检查更新永远返回「已是最新」（1.4.0.1 对 1.3.10.1 也不
> 可见）。渠道化之后：默认能更新，想稳的人显式选 stable。

#### 3.0.1 切回旧渠道：允许回退（`allowDowngrade`）

只按 semver 比较会有一个死结：预览版跑在 stable 前面（例如已装 `1.5.0.0`，stable 最新
只有 `1.4.0.1`）时，用户把渠道改成 `stable` 也永远看到「已是最新」——回不去了。

所以降级只作为一个**显式选项**存在，默认关闭：

| 入口                          | 行为                                                                                           |
| ----------------------------- | ---------------------------------------------------------------------------------------------- |
| `Config.AllowDowngrade`       | `Check` 在「没有更新」时把目标渠道的最新版（低于当前版本）报为 `updateAvailable` + `downgrade` |
| `Apply`                       | 未开该标志时拒绍降级（`ErrDowngrade`）；开了才允许替换成更旧的版本                             |
| Preferences → General         | 更新卡片显示「本渠道最新版为 x.y.z.w（当前 a.b.c.d）」+「切换到该版本」按钮                    |
| `dg update --allow-downgrade` | 同上；`--json` 的计划/回执带 `downgrade: true`                                                 |

启动横幅与原生菜单**不会**推送降级（始终 `allowDowngrade: false`）：降级只发生在用户
主动要求「回到这个渠道」的时候。

### 3.1 单实例与重启顺序

Wails 的 `SingleInstance` 在 `application.New` 里抢锁，第二个进程会
`notifyFirstInstance()` 后**直接 `os.Exit`**。所以重启不能简单地 `Start()` 新进程：

1. 旧进程先替换文件、再带 `--apply-update-relaunch <pid>` 启动新进程；
2. 新进程在 `main()` 最前面 `app.HandleUpdateRelaunch(os.Args[1:])` → 轮询等待旧
   pid 退出（上限 60s）→ 清理 `exe.old` → 继续正常启动，此时锁已释放。

这条路径必须在 `application.New` **之前**执行，否则会退化成「第二个实例闪退」。

### 3.2 权限与只读目录

- Windows：运行中的 exe 可以 `rename`，但不能 delete/覆盖 → 先改名的两步交换。
  `.old` 在旧进程存活期间删不掉，由下次启动的 `CleanupStale` 清除。
- POSIX：`rename` 覆盖运行中的可执行文件是允许的（旧 inode 仍被进程持有）。
- 目标目录只读（如 `/usr/local/bin`）→ `ErrPermission` 包装成可执行提示：
  「移到可写目录或以管理员权限运行」。

## 4. 校验为什么是 SHA-256 + Release 资产

| 选项                   | 结论                                                                     |
| ---------------------- | ------------------------------------------------------------------------ |
| 只用 HTTPS             | 够防窃听，但不足以防「同一 Release 被换资产」或镜像/代理篡改             |
| GitHub API 的 `digest` | 字段并非所有实例都返回，且与 `browser_download_url` 下载内容不做绑定校验 |
| **`SHA256SUMS.txt`**   | 与二进制同一个 Release、CI 生成、可人工核对 → 采纳                       |
| 签名（minisign/GPG）   | 需要密钥管理；单文件流水线目前不做签名，留作后续增强                     |

失败即停：哈希不匹配不会留下 `pending`，也不会触碰现有可执行文件。

## 5. 触发与 UX

| 入口                                                                            | 行为                                                                                                                                                           |
| ------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 启动检查（默认开）                                                              | `disable_auto_update !== true` 时后台 `Check`，有新版本 → 标题栏下方横幅（可忽略该版本）                                                                       |
| Preferences → General                                                           | 手动「检查更新」、下载进度条、`重启并安装`、自动检查开关                                                                                                       |
| 原生菜单「检查更新...」                                                         | 一次性检查，结果用 toast 呈现（可用版本时提供「立即更新」跳转设置）                                                                                            |
| `dg update [--check] [--force] [--channel preview\|stable] [--allow-downgrade]` | headless：`--check`/`--dry-run` 只出计划；默认下载+校验+自替换（不重启）；`--channel` 只影响本次调用、不写偏好；`--allow-downgrade` 允许回退到该渠道更旧的版本 |

`dev` 版本（源码构建，未注入 ldflags）**不提供更新**：避免把开发产物换成发布二进制。
`dg update` 可用 `--force` 显式覆盖（按 `0.0.0` 处理）。

## 6. 环境变量

| 变量                       | 用途                                                                      |
| -------------------------- | ------------------------------------------------------------------------- |
| `DROIDGEAR_UPDATE_REPO`    | 覆盖 `owner/name`（默认 `boltomli/droidgear`）                            |
| `DROIDGEAR_UPDATE_API`     | 覆盖 API 根（GitHub Enterprise / 测试用 httptest）                        |
| `DROIDGEAR_UPDATE_TOKEN`   | GitHub token，抬高 60 req/h 的匿名限额（回退 `GH_TOKEN`/`GITHUB_TOKEN`）  |
| `DROIDGEAR_UPDATE_CACHE`   | 覆盖暂存目录                                                              |
| `DROIDGEAR_UPDATE_CHANNEL` | `preview`（默认）/ `stable`，覆盖 `preferences.json` 的 `updates_channel` |
| `DROIDGEAR_UPDATE_EXE`     | 覆盖被替换的可执行文件路径（测试/包装启动器）                             |

## 7. 命令面 / 契约同步

新增命令必须同时出现在四处，否则门禁失败：

1. `wails/internal/app/update.go` 的 Go 方法（`CheckForUpdates` / `DownloadUpdate` /
   `ApplyUpdate` / `GetPendingUpdate` / `DismissUpdate`）—— 前三个都带一个
   `allowDowngrade` 参数（与 `dg update --allow-downgrade` 同义）
2. `src/lib/shell/commands.ts` 的 snake_case 表
3. `src/lib/bindings.ts` 的 facade（含 `update:progress` 事件的 payload 类型）
4. `wails/frontend/bindings/**`（`wails3 generate bindings`，gitignored）

`node scripts/check-command-surface.mjs` 校验 1–3 的名称/参数顺序一致。

## 8. 测试

| 层      | 覆盖                                                                                                                       |
| ------- | -------------------------------------------------------------------------------------------------------------------------- |
| service | 版本比较、资产选择、Release 选择（draft/prerelease/无校验资产/安装器 tag）、哈希失败、替换与回滚、跨二进制 kind 校验、清理 |
| CLI     | `dg update --check` / 真替换 / `dev` 需 `--force` / 上游不可达 → exit 4（httptest 假 GitHub）                              |
| 前端    | `npm run typecheck` + `lint`（UI 无独立单测，逻辑都在 Go 侧）                                                              |

## 9. 已知限制

- 只支持单文件发布线（`wails-v*`）。安装器流水线（`v*`）不产出可自更新资产。
- macOS 的 ad-hoc 签名：替换后的文件由 CI 签名，更新器不改签名；若用户把二进制放进
  `.app` 包内则不在支持范围（本项目单文件分发）。
- 无本地回滚 UI：Windows 上残留的 `exe.old` 是手动回滚材料（重命名即可），下次启动会被清理。
- 无灰度/分阶段推送：只有 `preview` / `stable` 两个渠道（见 §3.0），且渠道只做过滤，不做百分比放量。
