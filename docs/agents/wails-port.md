# Wails v3 Shell — Agent Quick Reference

One-pager for the second shell (`wails/` Go service + `src/wails/` frontend
bridge). The full audit lives in
[`../developer/wails-port-guidelines.zh.md`](../developer/wails-port-guidelines.zh.md)
(46 KB, porting contract) and its round2 follow-up
[`../developer/wails-round2-layout-and-gaps.zh.md`](../developer/wails-round2-layout-and-gaps.zh.md)
(layout regression, window contract, fix cards). **Read this file first** —
it lists only the traps that actually bite agents.

**Branch scope (`spike/wails-v3-minimal`): Wails-only.** `src-tauri/` and the
Rust TUI are frozen — no new Tauri commands, plugins or shim mappings. Non-GUI
work goes to `internal/service` + `cmd/dg` (contract:
[`../developer/wails-tui-cli-design.zh.md`](../developer/wails-tui-cli-design.zh.md)).

## Build order (a clean clone does NOT build)

```bash
npm ci                                                          # @wailsio/runtime, tsgo …
cd wails && wails3 generate bindings -clean=true -ts -i && cd ..  # gitignored — required first
npx vite build --config wails/vite.config.ts --mode production  # go:embed needs frontend/dist
cd wails && go vet ./... && go test ./...
```

CI (`.github/workflows/wails.yml`) runs these in this exact order — the Go
steps **must** follow the frontend build because of `//go:embed`.

## The traps

1. **Bindings are generated and gitignored.** `wails/frontend/bindings/**`
   never exists in a clone; `src/wails/shims/tauri-core.ts` and the direct
   importers (`tauri-opener.ts`, `tauri-pty.ts`, `tauri-system-fonts.ts`)
   resolve them through the `@wails-bindings` Vite alias. Skip generation →
   the frontend build fails on the first import.

2. **`frontend/dist` is required by `go vet`/`go test`.** `wails/main.go`
   has `//go:embed all:frontend/dist`. The directory is kept present by a
   tracked `.gitkeep` (see `wails/.gitignore`); the real bundle is built by
   CI. A missing directory fails every Go command.

3. **Tailwind v4 / Vite-root trap.** The Wails build sets Vite `root` to
   `wails/`, and Tailwind's automatic content detection follows the root —
   so `src/` classes silently vanish (CSS collapses 136 kB → 45 kB, layout
   breaks everywhere). `src/App.css` must keep the explicit
   `@source './**/*.{ts,tsx}'`. **Do not move it into `src/wails/wails.css`**
   (a file without `@import 'tailwindcss'` does not participate in
   generation). Guard: the `Tailwind utilities guard` CI step greps the
   output for `.h-screen` and `w-\[280px\]`.

4. **Error contract**: Go `error` → the shim rejects with the **raw string**
   (matching tauri-specta's `Err(String)`), never a JSON envelope. Use the
   `toRawError` pattern from the existing shims; localise known messages in
   `src/wails/error-i18n.ts`.

5. **Unknown fields must survive round-trips.** Config reads/writes use
   `core.UnmarshalJSONWithExtra` / `MarshalJSONWithExtra` (or keep the map
   order) so unknown keys and key order are preserved — user configs are
   rewritten in place. See `wails/PORTING.md` and a round-trip test.

6. **After adding/removing commands**: run
   `wails3 generate bindings -clean=true -ts -i`, then
   `node scripts/check-command-surface.mjs`. Every Go method must be routed
   through the shim, imported directly by a shim module (whitelist in
   `DIRECT_GO_METHODS`), or explicitly baselined.

7. **No no-op shims.** A `listen()` with the wrong event name or an `emit()`
   that silently drops data is a defect, not a placeholder. If a Tauri
   capability has no Wails equivalent, gate the call site with
   `src/lib/capabilities.ts` (`IS_WAILS_BACKEND`, `isUpdaterSupported()` …)
   instead of pretending it works.

## Shell-specific code (not shared)

| Area          | Tauri                    | Wails                                                                                    |
| ------------- | ------------------------ | ---------------------------------------------------------------------------------------- |
| Entry         | `src/main.tsx` → `App`   | `src/wails/main.tsx` → `AppShell`                                                        |
| Plugin bridge | real `@tauri-apps/*`     | `src/wails/shims/*` (Vite aliases)                                                       |
| Menu          | `src/lib/menu.ts`        | `internal/app/menu.go` + `menu-bridge.ts` (macOS only while frameless — see round2 §3.2) |
| Window        | `src-tauri/src/lib.rs`   | `wails/main.go` + contract in `wails/PORTING.md`                                         |
| Shared data   | `src/lib/*.json`         | vendored via `node wails/scripts/sync-registry.mjs`                                      |
| CLI (non-GUI) | `droidgear-tui` (frozen) | `wails/cmd/dg` — planned, must not import `internal/app` (Wails deps)                    |

## Validation checklist

```bash
node scripts/check-command-surface.mjs    # command surface
node wails/scripts/sync-version.mjs --check
cmp src/lib/model-registry-data.json wails/internal/core/registrydata/model-registry-data.json  # shared data
npm run typecheck && npm run lint && npm run test:run
```

Note: `npm run check:all` does **not** cover the Wails frontend build or the
Go steps — run them explicitly (see `commands.md` → _Wails (Go shell)_).
