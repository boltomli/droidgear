# Releases

Release process, version management, and auto-update system.

## Overview

The release system provides:

- Automated GitHub Actions workflow for building releases
- Version management script for updating all version files
- Auto-updater for seamless user updates
- Cross-platform builds (macOS, Windows, Linux)

## Initial Setup

### 1. Generate Signing Keys

```bash
npm install -g @tauri-apps/cli
tauri signer generate -w ~/.tauri/myapp.key
# Outputs private key (saved) and public key (displayed)
```

### 2. Configure GitHub Repository

Add these secrets (Settings → Secrets and variables → Actions):

- `TAURI_PRIVATE_KEY`: Content of `~/.tauri/myapp.key`
- `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`: Password you set (if any)

### 3. Update Configuration

**`src-tauri/tauri.conf.json`:**

```json
{
  "plugins": {
    "updater": {
      "active": true,
      "endpoints": [
        "https://github.com/YOUR_USERNAME/YOUR_REPO/releases/latest/download/latest.json"
      ],
      "dialog": false,
      "pubkey": "YOUR_PUBLIC_KEY_FROM_STEP_1"
    }
  }
}
```

**Bundle info in `tauri.conf.json`:**

- Update `publisher`, `shortDescription`, `longDescription`
- Update `productName` and `identifier`

## Release Process

### Simple Method

```bash
npm run release:prepare v1.0.0
```

This will:

1. Check git status is clean
2. Run all quality checks (`npm run check:all`)
3. Update versions in `package.json`, `Cargo.toml`, `tauri.conf.json`
4. Ask if you want to commit and push

Then GitHub Actions will:

1. Build the app for all platforms
2. Create a draft release
3. Generate `latest.json` for auto-updates
4. Upload all installers and signatures

Finally, manually publish the draft release on GitHub.

### Windows portable 自定义更新链路

- `latest-portable.json.signature` 必须直接写入 `.sig` 文件正文；这段正文本身就是 base64 文本，不能再额外包一层 base64，也不能写路径或 URL。
- 当前仓库会先读取 `droidgear_windows_x64.exe.sig`，再生成 `latest-portable.json`，并在工作流中校验：原始 `signature` 必须是单行 base64，解码后必须以 `untrusted comment:` 开头。
- 如果 CI 日志里出现 `Public signature:`、`Your file was signed successfully` 或 `Make sure to include this into the signature field of your update server` 混入 `signature`，说明签名来源取错了，应直接失败而不是继续上传 release 资产。

### Manual Method

```bash
# Update versions in package.json, Cargo.toml, tauri.conf.json
npm run check:all
git add .
git commit -m "chore: release v1.0.0"
git tag v1.0.0
git push origin main --tags
```

## Version Strategy

Semantic versioning (`v1.0.0`):

- **Major** (1.x.x): Breaking changes
- **Minor** (x.1.x): New features, backwards compatible
- **Patch** (x.x.1): Bug fixes

All three files must have matching versions:

- `package.json` → `"version": "1.0.0"`
- `src-tauri/Cargo.toml` → `version = "1.0.0"`
- `src-tauri/tauri.conf.json` → `"version": "1.0.0"`

## Auto-Update System

### Behavior

- Checks for updates 5 seconds after app launch
- Shows confirmation dialog when update is available
- Downloads and installs in background
- Offers to restart when complete
- Fails silently on network issues

### Update Flow

```
App Launch → (5s delay) → Check GitHub → Show Dialog → Download → Install → Restart
```

### Implementation

```typescript
// src/App.tsx
import { check } from '@tauri-apps/plugin-updater'
import { relaunch } from '@tauri-apps/plugin-process'

useEffect(() => {
  const checkForUpdates = async () => {
    try {
      const update = await check()
      if (update) {
        const shouldUpdate = confirm(`Update available: ${update.version}...`)
        if (shouldUpdate) {
          await update.downloadAndInstall()
          if (confirm('Restart to apply update?')) {
            await relaunch()
          }
        }
      }
    } catch {
      // Silent fail - don't bother user with network issues
    }
  }

  const timer = setTimeout(checkForUpdates, 5000)
  return () => clearTimeout(timer)
}, [])
```

### Manual Update Check

Users can manually check via:

- **Menu**: App → Check for Updates
- **Command Palette**: Cmd+K → "Check for Updates"

## Release Artifacts

Each release creates:

- **macOS**: `.dmg` installer
- **Windows**: `.msi` installer (when configured)
- **Linux**: `.deb` and `.AppImage` (when configured)
- **Auto-updater**: `latest.json` manifest and `.sig` signature files
- **Wails GUI (Windows only)**: `droidgear-wails_windows_x64.exe` (portable)
  and `droidgear-wails_windows_x64-setup.exe` (NSIS installer), built by the
  `publish-wails` job in `.github/workflows/release.yml`. Windows-only because
  `wails/internal/app` uses Windows-only syscall fields and the macOS/Linux
  packaging assets are not built yet (see
  `docs/developer/wails-port-guidelines.zh.md` §P3). The Wails version comes
  from `package.json` (e.g. `1.3.9+wails1`); installer metadata is stripped to
  numeric `x.y.z` by `wails/scripts/sync-version.mjs`, and the full version
  with suffix is reported by the app itself via ldflags.

## Single-binary release (Wails)

`.github/workflows/release-single.yml` is the lean alternative to the tag
pipeline above: **one executable per platform, nothing else** — no installer,
no `.sig`, no `latest.json`, no archives.

| Platform    | Asset                             | Runtime requirement                         |
| ----------- | --------------------------------- | ------------------------------------------- |
| Windows x64 | `droidgear-wails-windows-x64.exe` | WebView2 runtime (preinstalled on Win10/11) |
| macOS arm64 | `droidgear-wails-macos-arm64`     | none (system WebKit); ad-hoc signed         |
| Linux x64   | `droidgear-wails-linux-x64`       | GTK3 + WebKitGTK 4.1                        |

### Trigger 1 — tag push (works from any branch)

```bash
git push origin spike/wails-v3-minimal          # the workflow file must be committed
git tag wails-v1.3.9-1 && git push origin wails-v1.3.9-1
```

- A tag push runs the workflow **from the tagged commit**, so no `main` merge is
  required to build a feature branch.
- The `wails-` prefix is mandatory, not cosmetic: `release.yml` triggers on
  `v*`, so a plain `v1.3.9-…` tag would also start the full Tauri
  installer/updater pipeline. The workflow rejects tags without the prefix.
- Tag pushes create a **draft** release — inspect the three files, then publish.
- Re-running (new tag, or same tag after deleting the assets) replaces assets
  via `gh release upload --clobber`.

### Trigger 2 — manual dispatch (needs the file on `main`)

GitHub only shows "Run workflow" for a `workflow_dispatch` workflow that exists
on the **default branch**, so this entry point is only usable after the file
lands on `main`. Once it does:

```bash
gh workflow run release-single.yml --ref spike/wails-v3-minimal \
  -f tag=wails-v1.3.9-1 -f draft=true
```

`tag` must be an existing `wails-v…` tag; `draft` defaults to `true`.

### Why the Wails shell, not Tauri

`wails/main.go` embeds the Vite bundle (`//go:embed all:frontend/dist`), so
`go build` alone produces the complete app as a single file. The Tauri shell
cannot do that on macOS (`.app`/`.dmg` only), so it keeps using `release.yml`.

### Build steps per platform

`wails3 generate bindings` → `vite build --config wails/vite.config.ts`
(creates `wails/frontend/dist` for the embed) → `wails3 generate syso`
(Windows icon/version resource only) → `go build -tags "<tags>" -trimpath
-ldflags "-w -s … -X droidgear-wails/internal/app.appVersion=$APP_VERSION"`.
The version comes from `wails/scripts/sync-version.mjs`, i.e. `package.json`.

Build tags per platform: `production` on Windows/macOS, `production,gtk3` on
Linux. Wails v3 defaults to **GTK4 + webkitgtk-6.0** on Linux; `gtk3` switches
to **GTK3 + WebKitGTK 4.1** (`linux_cgo_gtk3.go` → `pkg-config gtk+-3.0
webkit2gtk-4.1 gdk-3.0`). That keeps the runner on ubuntu-22.04 (the gtk4
`libwebkitgtk-6.0-dev` only exists on 24.04+) and keeps the binary runnable on
older distros. The Linux job also sets `GOFLAGS=-tags=gtk3` so `go install
wails3` and `wails3 generate bindings` use the same backend.

### Caveats

- **A single file is not a static app**: the platform webview is still a
  runtime dependency (see table).
- **macOS** binaries are ad-hoc signed (Go does this automatically for
  arm64; the workflow re-signs explicitly) but **not notarized** — Gatekeeper
  may require `xattr -d com.apple.quarantine <file>`.
- **Windows** exe is unsigned → SmartScreen may warn.
- **macOS/Linux are compile-complete, not port-complete**: the title bar still
  renders the Windows control layout and the native menu is Windows-shaped
  (`docs/developer/wails-port-guidelines.zh.md` §P3). Treat them as previews.
- The package must stay cross-platform compilable: use
  `hideConsoleWindow` / `useNewConsole` from `wails/internal/app/procattr.go`
  instead of Windows-only `syscall.SysProcAttr` literals (`wails/PORTING.md`).

### Future CLI / TUI

Per ADR-001 new non-GUI surfaces live in the Go service layer, so they can ride
the same workflow: add a matrix entry (or an extra `go build ./cmd/<name>` step)
and upload the resulting binary the same way — `go build` of a CLI/TUI is
single-file by construction.

## Security

All updates are cryptographically signed:

1. Private key signs releases during build
2. Public key in config verifies downloads
3. Invalid signatures are automatically rejected

## Troubleshooting

| Issue                    | Solution                                              |
| ------------------------ | ----------------------------------------------------- |
| Workflow doesn't trigger | Ensure tag starts with `v` and is pushed              |
| Build fails              | Check GitHub secrets, run `npm run check:all` locally |
| Updates not detected     | Verify endpoint URL and public key match              |
| Download fails           | Check signatures, file permissions, disk space        |
