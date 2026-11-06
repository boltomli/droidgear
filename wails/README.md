# DroidGear Minimal (Wails v3)

A **GUI-only** configurator for the full DroidGear tool set — **Droid /
OpenCode / Codex / Claude / OpenClaw / Copilot / Hermes / Pi / OMP / Dsh**
plus **Channels** — built with [Wails v3](https://v3.wails.io) (Go backend +
embedded webview frontend).

The frontend is the **unmodified** `src/` component tree (shadcn/ui, zustand
stores, TanStack Query, react-i18next); only the transport differs — the
Tauri shell was removed (`fe22c57`), so every backend call now runs through
`src/lib/shell/*` into the Go service, and the parity table below is what
the single remaining shell ships.

> ⚠️ **Known gaps:** command-surface parity holds and the shell-level
> integrations from the audit roadmap (window-state persistence, single
> instance, opener shim, system-fonts shim, notifications service, single-file
> self-update, theme-following window background, cross-platform platform shim)
> are in place. Still missing: an in-window menu button for Windows/Linux
> (frameless — see below) and macOS/Linux packaging assets (icon bundles,
> installers). The headless CLI layer **has** landed — see `cmd/dg` below.
> Audit, evidence and the fix roadmap live in
> [`docs/developer/wails-port-guidelines.zh.md`](../docs/developer/wails-port-guidelines.zh.md)
> and its round2 follow-up.
>
> **Build prerequisites (a clean clone will NOT build):**
>
> ```bash
> npm ci                                                       # @wailsio/runtime etc.
> cd wails && wails3 generate bindings -clean=true -ts -i && cd ..   # gitignored bindings
> npm run build                                                   # → wails/frontend/dist (go:embed)
> ```
>
> **Layout (keep the guard):** Tailwind v4 detects sources from the Vite
> `root`; the config now uses the repo root, but the explicit
> `@source './**/*.{ts,tsx}'` in `src/App.css` must stay — dropping it once
> collapsed the CSS from 136 kB → 45 kB (no `h-screen`/`w-[280px]`, broken
> sidebar and buttons). The `Tailwind utilities guard` CI step greps the
> output for `.h-screen` / `w-\[280px\]` so this cannot regress silently.
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
React stores (unchanged)          Go service
  src/store/*-store.ts              wails/internal/app/*.go
        │                                   ▲
        ▼                                   │
  src/lib/commands/*               generated TS bindings
    invoke("save_dsh_provider", …)  wails/frontend/bindings/**/app.ts
        │                                   ▲
        └──► src/lib/shell/commands.ts (snake_case → PascalCase funnel)
```

1. **One choke point.** Every backend call in the frontend funnels through
   `invoke(cmd, args)` in `src/lib/bindings.ts`, and
   `src/lib/shell/commands.ts` is the only module that knows the snake_case →
   PascalCase mapping and the positional argument order (its `COMMANDS`
   table). Rejections are normalised back to a plain string and localised
   by `src/lib/shell/error-i18n.ts`.
2. **Generated bindings.** `wails3 generate bindings -ts` produces typed TS
   wrappers (`Call.ByID`) for the Go `App` service; the shell modules
   import them via the `@wails-bindings` Vite alias. **Regenerate them after
   adding or renaming Go methods** (the `build:frontend` task does this
   automatically).
3. **Same JSON contract.** The Go structs mirror the generated types in
   `src/lib/bindings.ts` field-for-field (camelCase, `baseURL` for Dsh,
   snake_case `AppPreferences`), including `json.MarshalJSONWithExtra`-style
   preservation of unknown fields so newer tool configs survive a
   read/modify/write cycle.
4. **Single Vite config.** The repo-root `vite.config.ts` is the only
   frontend build (React 19 + React Compiler, Tailwind v4, lightningcss); it
   outputs to `wails/frontend/dist` for `//go:embed` and aliases
   `@wails-bindings` to the generated bindings.
5. **One entry, no capability gates.** `src/main.tsx` renders `App`
   (TitleBar, LeftSideBar, MainWindowContent, command palette, preferences)
   unchanged; `src/lib/capabilities.ts` and the Tauri shims were removed
   together with the Tauri shell, so there is no per-backend feature switch
   left in the component tree.
6. **Terminal without a plugin.** The PTY API lives in
   `src/lib/shell/pty.ts` on top of the Go PTY service
   (`internal/app/pty.go`, ConPTY via `aymanbagabas/go-pty`); output and exit
   stream over `pty:data` / `pty:exit` Wails events as base64 chunks, decoded
   with a streaming `TextDecoder`.

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
| Shared   | Preferences (`~/.droidgear/preferences.json`, shared with the `dg` CLI), model registry enrichment, **model connectivity tests** (ping + inference, single/all, diagnostics summary), BYOK custom models/favorites in `~/.factory/settings.json`, channels CRUD + New API/**Sub2API** token import + model fetching, channel JSON import/export, **channel export templates** (`~/.droidgear/export-templates.yaml`, JSON/YAML/TOML output), Hermes WSL path fallback, **Preferences → Paths pane** (per-tool overrides + WSL distro picker), **in-app terminal** (Go PTY: ConPTY on Windows via `aymanbagabas/go-pty`, streamed over `pty:data`/`pty:exit` Wails events), **crash-recovery data**, native notifications (Wails notifications service), shell env detection, localised backend errors (en/zh) |

## What is not implemented

- **Installer-based updates** — updates only work for the single-file
  `wails-v*` release line (`release-single.yml`). The app checks GitHub,
  downloads the one asset for the running platform, verifies it against the
  release's `SHA256SUMS.txt`, swaps the executable and restarts
  (`internal/service/updater`, `internal/app/update.go`, `dg update`). The
  legacy `v*` installer pipeline ships no self-update assets. Design:
  `docs/developer/self-update-single-file.zh.md`.

- **macOS/Linux packaging** — `release-single.yml` ships bare single-file
  binaries for both, but there is no `.icns` bundle, DMG or `.deb` yet
  (Windows-first: NSIS installer + icon + version resource).

Porting conventions for adding further command surfaces live in
[`PORTING.md`](./PORTING.md).

## Backend error localisation

`src/lib/shell/error-i18n.ts` translates the common Go error strings to en/zh
(`errors.*` locale keys) inside the command bridge; unmatched messages pass
through unchanged. The `DSH_CONFIG_CONFLICT` / `DSH_CONFIG_SHADOWED`
prefixes are preserved verbatim because the store matches them.

## Headless CLI (`dg`)

`cmd/dg` is the non-GUI consumer required by AGENTS.md rule 9: it shares
`internal/service` + `internal/core` with the GUI and links **no** Wails
runtime, so it runs over SSH or in a container (`CGO_ENABLED=0 go build -o
dg ./cmd/dg` — no WebKit/Cocoa needed; CI job `wails.yml → cli-headless`
builds and tests it on Ubuntu with no GUI toolchain installed).

```bash
cd wails
go run ./cmd/dg help

go run ./cmd/dg --json doctor          # effective paths, WSL, config files (read-only)
go run ./cmd/dg path show              # the nine config paths
go run ./cmd/dg path set codex /srv/codex
go run ./cmd/dg channels list --json
go run ./cmd/dg channels tokens ch1    # keys redacted unless --reveal
go run ./cmd/dg export run t1 --dry-run  # ChangeSet preview, nothing written
go run ./cmd/dg export run t1           # writes the output file
```

Contract: **`--json` is the machine interface** (one JSON document on
stdout, error envelope on stderr, camelCase fields identical to the
bindings), `--dry-run` never touches the filesystem, secrets are redacted
unless `--reveal`. Exit codes: `0` ok · `1` runtime/not-found · `2` usage ·
`3` conflict (pending) · `4` upstream/provider. Full specification, the
layering rules (service must not import Wails, `dg` must not import
`internal/app`) and the batch plan live in
[`../developer/wails-tui-cli-design.zh.md`](../developer/wails-tui-cli-design.zh.md);
the gate is `node scripts/check-go-layering.mjs`.

Covered today: `version`, `doctor`, `path`, `channels`, `codex`, `export`.
Profile tools (`pi`, `hermes`, `omp`, `openclaw`, …) follow once their
domains move into `internal/service`.

**Release builds** ship `dg` next to the GUI single-file executable:
`release-single.yml` produces `dg-windows-x64.exe` / `dg-macos-arm64` /
`dg-linux-x64` alongside the three `droidgear-wails-*` assets, built with
`CGO_ENABLED=0` and **without** `-H windowsgui` — the CLI is a console
binary that needs none of the GUI's WebView2/GTK/WebKit runtime.

Two release workflows exist on purpose: `release-single.yml`
(`wails-v*` tags) is the single-binary line — one GUI file per platform plus
`dg`, no installers — while `release.yml` (`v*` tags) drives the official
release: changelog-driven draft plus the Windows NSIS installer
(`wails3 task package`). The Tauri jobs were removed in `fe22c57`, so
`release.yml` is Wails-only too; the tag prefixes stay disjoint so a
`wails-v…` push can never fire it. **On this branch always tag `wails-v*`**
(see `docs/agents/commands.md` → _CI / Release tags_).

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
2. `build:frontend` — `npm run build` (repo-root `vite.config.ts`), output
   to `wails/frontend/dist`
3. `go build -tags production` with `//go:embed all:frontend/dist`

**Version**: the single source of truth is the repo-root `package.json`.
`scripts/sync-version.mjs` syncs it into
`build/config.yml`, `build/windows/info.json` and the NSIS/MSIX installer
assets, and prints it for the `-ldflags "-X
droidgear-wails/internal/version.Version=…"` injection, so `GetAppVersion`
(About / General panes) reports the same version everywhere. Drift check (CI):
`node wails/scripts/sync-version.mjs --check`.

## Files added by the spike

```
wails/
├── main.go                     # Wails app bootstrap (frameless window, embedded assets)
├── PORTING.md                  # contract for porting more Tauri commands → Go service
├── Taskfile.yml / build/       # customized Wails build pipeline (Windows)
├── index.html                  # dev-mode entry served by `wails3 dev`
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

src/lib/shell/                  # frontend ↔ Go bridge (typechecked + linted with the repo)
├── commands.ts                 # snake_case → PascalCase command funnel
├── events.ts / menu-bridge.ts  # Wails event bus + native menu wiring
├── pty.ts / opener.ts / fonts.ts  # terminal, opener, system fonts
└── error-i18n.ts               # Go error → en/zh localisation rules
```

`src/lib/model-registry-data.json` is vendored into
`wails/internal/core/registrydata/` because Go cannot `//go:embed` files
outside the module; run `node wails/scripts/sync-registry.mjs` after editing
the registry.

## Housekeeping

- Frontend deps: `@wailsio/runtime` was added to the root `package.json`
  (used by the generated bindings).
- `.prettierignore` excludes `wails/frontend/` (generated bindings + dist).
- The Tauri shell is gone (`fe22c57`); `npm run check:no-tauri` keeps it
  from coming back.
