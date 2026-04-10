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
