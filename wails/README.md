# DroidGear Minimal (Wails v3)

A **GUI-only** configurator for the full DroidGear tool set — **Droid /
OpenCode / Codex / Claude / OpenClaw / Copilot / Hermes / Pi / OMP / Dsh**
plus **Channels** — built with [Wails v3](https://v3.wails.io) (Go backend +
embedded webview frontend).

The frontend is the **unmodified** `src/` component tree (shadcn/ui, zustand
stores, TanStack Query, react-i18next); only the `invoke` transport is swapped
out. Since the parity push, the Go service mirrors the full Tauri command
surface, so the two shells expose the same features and render the same
component tree (the only intentional exceptions are the Tauri-only
auto-updater and the shell-level window chrome described below).

> ⚠️ **Known gaps:** command-surface parity holds and the shell-level
> integrations from the audit roadmap (window-state persistence, single
> instance, opener shim, system-fonts shim, notifications service, updater
> gating, theme-following window background, cross-platform platform shim)
> are in place. Still missing: an in-window menu button for Windows/Linux
> (frameless — see below), macOS/Linux build assets and the TUI/CLI layer.
> Audit, evidence and the fix roadmap live in
> [`docs/developer/wails-port-guidelines.zh.md`](../docs/developer/wails-port-guidelines.zh.md)
> and its round2 follow-up.
>
> **Build prerequisites (a clean clone will NOT build):**
>
> ```bash
> npm ci                                                       # @wailsio/runtime etc.
> cd wails && wails3 generate bindings -clean=true -ts -i && cd ..   # gitignored bindings
> npx vite build --config wails/vite.config.ts --mode production     # go:embed needs frontend/dist
> ```
>
> **Layout (fixed):** Tailwind v4 detects sources from the Vite `root`, and
> this config sets `root: wails/`, so `src/**` classes were never generated
> (CSS 45 kB instead of 136 kB → no `h-screen`/`w-[280px]`, broken sidebar
> and buttons). Fixed by the explicit `@source './**/*.{ts,tsx}'` in
> `src/App.css` — **keep it**; the `Tailwind utilities guard` CI step greps
> the output for `.h-screen` / `w-\[280px\]` so this cannot regress silently.
>
> **Frameless menu (known, accepted):** with `Frameless: true` Wails does
> **not** attach the application menu on Windows/Linux (source:
> `if !options.Frameless && !options.Windows.DisableMenu`), so
> `internal/app/menu.go` + `menu-bridge.ts` only take effect on macOS.
> Decision (round2 B5, 2026-11-02): keep frameless — an in-window menu
> button on the title bar is the planned replacement and is deliberately
> **not** implemented yet. See
> [`docs/developer/wails-round2-layout-and-gaps.zh.md`](../docs/developer/wails-round2-layout-and-gaps.zh.md)
> §3.2.

## How the reuse works

```
React stores (unchanged)          Go service (new)
  src/store/*-store.ts              wails/internal/app/*.go
        │                                   ▲
        ▼                                   │
  src/lib/bindings.ts (unchanged)   generated TS bindings
    TAURI_INVOKE("save_dsh_provider", …)   wails/frontend/bindings/**/app.ts
        │                                   ▲
        └── alias ──► src/wails/shims/tauri-core.ts (invoke bridge)
```

1. **One choke point.** Every backend call in the frontend funnels through
   `TAURI_INVOKE(cmd, args)` from the tauri-specta generated
   `src/lib/bindings.ts`. The Wails build aliases `@tauri-apps/api/core` to
   `src/wails/shims/tauri-core.ts`, which maps each snake_case Tauri command
   to the PascalCase Go method (see `COMMANDS` for the argument order) and
   normalises rejections back to the raw error string that bindings.ts
   expects (`Err(String)` in Rust ↔ Go `error`).
2. **Generated bindings.** `wails3 generate bindings -ts` produces typed TS
   wrappers (`Call.ByID`) for the Go `App` service; the shim imports them via
   the `@wails-bindings` Vite alias. **Regenerate them after adding or
   renaming Go methods** (the `build:frontend` task does this automatically).
3. **Same JSON contract.** The Go structs mirror the tauri-specta types in
   `src/lib/bindings.ts` field-for-field (camelCase, `baseURL` for Dsh,
   snake_case `AppPreferences`), including `#[serde(flatten)]`-style
   preservation of unknown fields so newer tool configs survive a
   read/modify/write cycle.
4. **Single Vite config.** `wails/vite.config.ts` reuses the repo-root
   toolchain (React 19 + React Compiler, Tailwind v4, lightningcss) and only
   overrides entry HTML, aliases (`@tauri-apps/*`, `tauri-pty`) and output dir.
5. **Same shell, gated capabilities.** `src/wails/AppShell.tsx` renders the
   _shared_ `MainWindow` (TitleBar, LeftSideBar, MainWindowContent, command
   palette, preferences) unchanged. Tools and sub-views whose Go backend does
   not exist yet are gated through `src/lib/capabilities.ts` (the Wails
   build sets `__WAILS_BACKEND__`): unsupported tools render as disabled
   placeholders in the switcher and the sessions entries are hidden, so the
   two shells differ only in what is _enabled_, never in layout.
6. **Terminal without Rust.** The `tauri-pty` npm API is re-implemented in
   `src/wails/shims/tauri-pty.ts` on top of the Go PTY service
   (`internal/app/pty.go`, ConPTY via `aymanbagabas/go-pty`); output and exit
   stream over `pty:data` / `pty:exit` Wails events as base64 chunks, decoded
   with a streaming `TextDecoder` in the shim.

## What is implemented

| Area     | Support                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Dsh      | Profiles (desktop/web), provider CRUD in `cordis.patch.yml` / legacy `settings.yaml`, credentials (`~/.dsh/.credentials.yaml`), model fetching with registry enrichment, **advanced config editor** (schema composition, `$js`/`!js` round-trip, secret redaction, DSH profile lock, optimistic-concurrency hash), **sessions viewer** (zstd-compressed concatenated-frame `.jsonl.zst` decoding, poll watcher)                                                                                                                                                                                                                                                                                                                                                                                               |
| Pi       | Profile CRUD, apply → `~/.pi/agent/models.json`, active profile, connection test via the `pi` CLI, **sessions viewer**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| Hermes   | Profile CRUD (incl. legacy single-model migration), apply → `~/.hermes/config.yaml` (read-modify-write), current-config merge                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| OMP      | Profile CRUD, apply → `~/.omp/agent/config.yml`, custom providers in `models.yml`, read-only `models.db`/`agent.db` (pure-Go SQLite), HTTP connection test, **sessions viewer**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| Codex    | Profile CRUD, apply → `~/.codex/config.toml` (go-toml round-trip preserving unknown keys), auth.json handling (official vs BYOK), DeepSeek/MiMo model catalogs, CLI capability probe, **auth profiles** (manifest-based save/switch/rename/delete with backup + conflict detection), **sessions viewer** (rollout `.jsonl` head/tail windows, `session_index.jsonl` titles, sqlite thread-index sync), terminal launch                                                                                                                                                                                                                                                                                                                                                                                        |
| OpenCode | Profile CRUD, apply-to-config (read-modify-write preserving unknown fields), JSONC comment stripping, `baseUrl`→`baseURL` normalisation, provider templates, `/v1/models` connection probe, terminal launch                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| Claude   | Profile CRUD, apply → `~/.claude/settings.json`, **settings-file manager** (Global + custom files, active pointer, legacy-profile migration, merge-to-global, load-from-live), temporary-run preview, settings launch (in-process runtime materialisation + terminal spawn)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| OpenClaw | Profile CRUD, apply → `~/.openclaw/openclaw.json` (order-preserving document merge that keeps unknown fields, key order, and number formatting byte-for-byte), **subagents** read/save (deep-merge, `main`+`allowAgents` synthesis)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| Copilot  | Profile CRUD + sibling `.env` files, validation (single-line fields, positive token limits, provider types, WHATWG endpoint normalisation), channel-import preview, live `config.env` parse with `is_byok` detection, run-plan with secret-env separation, terminal launch                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| Droid    | **Settings files** (Global + custom, link/unlink external, active pointer), **trusted folders**, **Specs** (CRUD + poll watcher), **MCP servers** (load/save/delete/toggle + 百知 preset), **factory auth profiles**, run-settings values (diff mode, cloud session sync, reasoning effort, compaction limits, todo display, show-thinking, co-authored-by, mission models), launch                                                                                                                                                                                                                                                                                                                                                                                                                           |
| Shared   | Preferences (shared with the Tauri app via the same `preferences.json`), model registry enrichment, **model connectivity tests** (ping + inference, single/all, diagnostics summary), BYOK custom models/favorites in `~/.factory/settings.json`, channels CRUD + New API/**Sub2API** token import + model fetching, channel JSON import/export, **channel export templates** (`~/.droidgear/export-templates.yaml`, JSON/YAML/TOML output), Hermes WSL path fallback, **Preferences → Paths pane** (per-tool overrides + WSL distro picker), **in-app terminal** (Go PTY: ConPTY on Windows via `aymanbagabas/go-pty`, streamed over `pty:data`/`pty:exit` Wails events), **crash-recovery data**, native notifications (Wails notifications service), shell env detection, localised backend errors (en/zh) |

## What is not implemented

- **Auto-updater** — Tauri-only by design (see
  `src/wails/shims/tauri-core.ts` `Resource` stub); the Wails build ships
  without an update feed. Audited 2026-11-02 (round2 §B10): every entry
  point that could reach `services/updater.ts` is gated, so nothing in the
  Wails UI can throw `unsupported command`:

  | Entry                                            | Gate                                                                            |
  | ------------------------------------------------ | ------------------------------------------------------------------------------- |
  | `App.tsx` auto-check timers                      | Tauri entry only (`src/main.tsx`), not bundled                                  |
  | `lib/menu.ts` “Check for Updates”                | Tauri entry only, not bundled                                                   |
  | `GeneralPane` check button / auto-update toggle  | `isUpdaterSupported()`                                                          |
  | `GeneralPane` download button / pending update   | unreachable: `pendingUpdate` is never persisted or set without the gated button |
  | `notification.test-update-toast` palette command | `isAvailable: DEV && isUpdaterSupported()`                                      |
  | `showUpdateNotification` callers                 | only the gated paths above                                                      |

  Long term: either keep Tauri-only or rebuild on Wails `pkg/updater`
  (round2 §4 序 9).

- **macOS/Linux build assets** (Windows-first; the Go service itself is
  portable and the terminal/launch code has POSIX fallbacks).

Porting conventions for adding further command surfaces live in
[`PORTING.md`](./PORTING.md).

## Backend error localisation

`src/wails/error-i18n.ts` translates the common Go error strings to en/zh
(`errors.*` locale keys) inside the invoke shim; unmatched messages pass
through unchanged. The `DSH_CONFIG_CONFLICT` / `DSH_CONFIG_SHADOWED`
prefixes are preserved verbatim because the store matches them.

## Building

Prerequisites: Go ≥ 1.26, Node (repo-root `node_modules` installed via
`npm ci`), `wails3` CLI v3.

```bash
cd wails
wails3 build          # production build → bin/droidgear-minimal.exe
wails3 task dev       # rebuild + run on Go/TS changes (no dev server)
wails3 task run       # run the last build
```

The build pipeline (`build/Taskfile.yml`):

1. `generate:bindings` — `wails3 generate bindings -clean=true -ts -i`
2. `build:frontend` — `npx vite build --config wails/vite.config.ts` from the
   repo root, output to `wails/frontend/dist`
3. `go build -tags production` with `//go:embed all:frontend/dist`

**Version**: the single source of truth is the repo-root `package.json`
(same value the Tauri build embeds). `scripts/sync-version.mjs` syncs it into
`build/config.yml`, `build/windows/info.json` and the NSIS/MSIX installer
assets, and prints it for the `-ldflags "-X
droidgear-wails/internal/app.appVersion=…"` injection, so `GetAppVersion`
(About / General panes) reports the same version everywhere. Drift check (CI):
`node wails/scripts/sync-version.mjs --check`.

## Files added by the spike

```
wails/
├── main.go                     # Wails app bootstrap (frameless window, embedded assets)
├── PORTING.md                  # contract for porting more Tauri commands → Go service
├── Taskfile.yml / build/       # customized Wails build pipeline (Windows)
├── vite.config.ts / index.html # frontend build reusing the repo toolchain
├── scripts/sync-registry.mjs   # refresh the vendored model registry
├── scripts/sync-version.mjs    # sync package.json version → build assets + ldflags
└── internal/
    ├── app/                    # Go service methods (266, mirroring the Tauri command surface)
    │   ├── app.go channels.go factory.go …
    │   ├── dsh.go dsh_config*.go               # Dsh (incl. advanced config editor)
    │   ├── pi.go hermes.go omp.go              # Pi / Hermes / OMP
    │   ├── codex.go codex_auth_profiles.go codex_sessions.go
    │   ├── opencode.go copilot.go claude.go claude_settings.go
    │   ├── openclaw.go openclaw_subagents.go
    │   ├── droid_settings.go droid_trusted.go specs.go mcp.go factory_auth_profiles.go
    │   ├── sessions.go dsh_sessions.go pi_sessions.go omp_sessions.go legacy_config.go
    │   ├── connectivity.go fetch_models.go    # model connectivity tests + model discovery
    │   ├── pty.go           # in-app terminal (ConPTY, event streaming)
    │   ├── export.go        # channel export templates + engine
    │   ├── httpfetch.go     # channel token import (New API + Sub2API) + model fetch
    │   └── paths.go recovery.go notifications.go env.go fsio.go window.go
    └── core/                   # paths / atomic writes / YAML+JSON helpers / registry

src/wails/                      # frontend glue (typechecked + linted with the repo)
├── main.tsx / AppShell.tsx     # renders the shared MainWindow + startup init
├── wails.css                   # height-chain + frameless drag-region overrides
├── error-i18n.ts               # Go error → en/zh localisation rules
└── shims/                      # @tauri-apps/* + tauri-pty replacements routing to Wails

src/lib/capabilities.ts         # backend capability gating (shared with the Tauri build)
```

`src/lib/model-registry-data.json` is vendored into
`wails/internal/core/registrydata/` because Go cannot `//go:embed` files
outside the module; run `node wails/scripts/sync-registry.mjs` after editing
the registry.

## Housekeeping

- Frontend deps: `@wailsio/runtime` was added to the root `package.json`
  (used by the generated bindings).
- `.prettierignore` excludes `wails/frontend/` (generated bindings + dist).
- No Tauri code paths were modified; the Tauri app builds exactly as before.
