# Wails v3 Shell — Agent Quick Reference

One-pager for the Wails shell (`wails/` Go service + `src/` frontend bridge).
The full audit lives in
[`../developer/wails-port-guidelines.zh.md`](../developer/wails-port-guidelines.zh.md)
(46 KB, porting contract) and its round2 follow-up
[`../developer/wails-round2-layout-and-gaps.zh.md`](../developer/wails-round2-layout-and-gaps.zh.md)
(layout regression, window contract, fix cards). **Read this file first** —
it lists only the traps that actually bite agents.

**Scope: Wails-only.** The Tauri shell (`src-tauri/`) and the Rust workspace
were removed in `fe22c57`; there is no `src/wails/` bridge layer anymore —
the frontend talks to Go through `src/lib/shell/*` and the generated
bindings. Non-GUI work goes to `internal/service` + `cmd/dg` (contract:
[`../developer/wails-tui-cli-design.zh.md`](../developer/wails-tui-cli-design.zh.md)).

## Build order (a clean clone does NOT build)

```bash
npm ci                                                          # @wailsio/runtime, tsgo …
cd wails && wails3 generate bindings -clean=true -ts -i && cd ..  # gitignored — required first
npm run build                                                   # → wails/frontend/dist (go:embed)
cd wails && go vet ./... && go test ./...
```

CI (`.github/workflows/wails.yml`) runs these in this exact order — the Go
steps **must** follow the frontend build because of `//go:embed`.

## The traps

1. **Bindings are generated and gitignored.** `wails/frontend/bindings/**`
   never exists in a clone; `src/lib/shell/*` resolves them through the
   `@wails-bindings` Vite alias (`src/lib/app-bindings.ts` re-exports the
   typed `commands`). Skip generation → the frontend build fails on the first
   import.

2. **`frontend/dist` is required by `go vet`/`go test`.** `wails/main.go`
   has `//go:embed all:frontend/dist`. The directory is kept present by a
   tracked `.gitkeep` (see `wails/.gitignore`); the real bundle is built by
   CI. A missing directory fails every Go command.

3. **Tailwind v4 `@source` guard.** Tailwind's automatic content detection
   follows the Vite root; `src/App.css` must keep the explicit
   `@source './**/*.{ts,tsx}'` so utilities used in `src/` never silently
   vanish (the failure mode is a CSS collapse from 136 kB → 45 kB with the
   layout broken everywhere). Guard: the `Tailwind utilities guard` CI step
   greps the output for `.h-screen` and `w-\[280px\]`.

4. **Error contract**: Go `error` → the shell rejects with the **raw string**
   (matching the original tauri-specta `Err(String)` contract), never a JSON
   envelope. Use the `toRawError` pattern from the existing shell modules;
   localise known messages in `src/lib/shell/error-i18n.ts`.

5. **Unknown fields must survive round-trips.** Config reads/writes use
   `core.UnmarshalJSONWithExtra` / `MarshalJSONWithExtra` (or keep the map
   order) so unknown keys and key order are preserved — user configs are
   rewritten in place. See `wails/PORTING.md` and a round-trip test.

6. **After adding/removing commands**: run
   `wails3 generate bindings -clean=true -ts -i`, then
   `node scripts/check-command-surface.mjs`. Every Go method must be routed
   through the shell table (`src/lib/shell/commands.ts`), imported directly
   by a shell module (whitelist in `DIRECT_GO_METHODS`), or explicitly
   baselined.

7. **No Tauri APIs, ever.** `@tauri-apps/*` imports, shims and
   `src/lib/capabilities.ts` are gone; `npm run check:no-tauri` fails the
   build if one comes back. Runtime access always goes through
   `src/lib/shell/*`.

## Shell-specific code

| Area          | Implementation                                                   |
| ------------- | ---------------------------------------------------------------- |
| Entry         | `src/main.tsx` → `App`                                           |
| Bridge        | `src/lib/shell/*` (commands, events, menu, pty, opener, fonts …) |
| Menu          | `internal/app/menu.go` + `src/lib/shell/menu-bridge.ts`          |
| Window        | `wails/main.go` + contract in `wails/PORTING.md`                 |
| Shared data   | vendored via `node wails/scripts/sync-registry.mjs`              |
| CLI (non-GUI) | `wails/cmd/dg` — must not import `internal/app` (Wails deps)     |

## Validation checklist

```bash
node scripts/check-command-surface.mjs    # command surface (names + arg order)
node scripts/check-go-layering.mjs        # service must not import wails; cmd/dg must not import internal/app
node wails/scripts/sync-version.mjs --check
cmp src/lib/model-registry-data.json wails/internal/core/registrydata/model-registry-data.json  # shared data
npm run typecheck && npm run lint && npm run test:run
cd wails && go vet ./... && go test ./...   # includes cmd/dg contract tests
```

Note: `npm run check:all` does **not** cover the Wails frontend build or the
Go steps — run them explicitly (see `commands.md` → _Wails (Go shell)_).
