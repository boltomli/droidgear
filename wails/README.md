# DroidGear Minimal (Wails v3 spike)

A minimal, **GUI-only** configurator for **Dsh / Pi / Hermes / OMP**, built with
[Wails v3](https://v3.wails.io) (Go backend + embedded webview frontend).

The spike answers one question: _can we reuse DroidGear's entire React UI on
top of a Go backend with Wails v3?_ The answer is yes — the frontend is the
**unmodified** `src/` component tree (shadcn/ui, zustand stores, TanStack
Query, react-i18next), and only the `invoke` transport is swapped out.

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

| Area   | Support                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| ------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Dsh    | Profiles (desktop/web), provider CRUD in `cordis.patch.yml` / legacy `settings.yaml`, credentials (`~/.dsh/.credentials.yaml`), model fetching with registry enrichment, **advanced config editor** (schema composition, `$js`/`!js` round-trip, secret redaction, DSH profile lock, optimistic-concurrency hash)                                                                                                                                                                                                                                                                                                                                                                                    |
| Pi     | Profile CRUD, apply → `~/.pi/agent/models.json`, active profile, connection test via the `pi` CLI                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| Hermes | Profile CRUD (incl. legacy single-model migration), apply → `~/.hermes/config.yaml` (read-modify-write), current-config merge                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| OMP    | Profile CRUD, apply → `~/.omp/agent/config.yml`, custom providers in `models.yml`, read-only `models.db`/`agent.db` (pure-Go SQLite), HTTP connection test                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| Shared | Preferences (shared with the Tauri app via the same `preferences.json`), model registry enrichment, BYOK custom models/favorites in `~/.factory/settings.json`, channels CRUD + New API token import + model fetching, channel JSON import/export, **channel export templates** (`~/.droidgear/export-templates.yaml`, JSON/YAML/TOML output), Hermes WSL path fallback, **Preferences → Paths pane** (per-tool overrides + WSL distro picker), **in-app terminal** (Go PTY: ConPTY on Windows via `aymanbagabas/go-pty`, streamed over `pty:data`/`pty:exit` Wails events), **crash-recovery data**, native notifications (PowerShell toast), shell env detection, localised backend errors (en/zh) |

## What is not implemented (degrades gracefully)

- **Sessions viewer** pages (per-tool session history) — the Go service has
  no session-store readers yet; the sidebar entries are hidden by
  `src/lib/capabilities.ts`.
- **Tools without a Go backend** (Droid, OpenCode, Codex, Claude, OpenClaw,
  Copilot) render as disabled placeholders in the tool switcher.
- **Sub2API token import**.
- macOS/Linux build assets (Windows only).

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

## Files added by the spike

```
wails/
├── main.go                     # Wails app bootstrap (frameless window, embedded assets)
├── Taskfile.yml / build/       # customized Wails build pipeline (Windows)
├── vite.config.ts / index.html # frontend build reusing the repo toolchain
├── scripts/sync-registry.mjs   # refresh the vendored model registry
└── internal/
    ├── app/                    # Go service methods (96, mirroring Tauri commands)
    │   ├── app.go dsh.go pi.go hermes.go omp.go channels.go factory.go …
    │   ├── pty.go              # in-app terminal (ConPTY, event streaming)
    │   ├── export.go           # channel export templates + engine
    │   └── paths.go recovery.go notifications.go env.go fsio.go window.go
    └── core/                   # paths / atomic writes / YAML+JSON helpers / registry

src/wails/                      # frontend glue (typechecked + linted with the repo)
├── main.tsx / AppShell.tsx     # renders the shared MainWindow + startup init
├── wails.css                   # height-chain + frameless drag-region overrides
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
