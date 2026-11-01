# Porting Guide: Rust/Tauri commands → Wails Go service

This guide is the contract for porting a slice of the Tauri backend into the
Go service under `wails/internal/app/`. Read it fully before writing code.

## Goal

The Wails build reuses the **unmodified** React frontend. Every backend call
funnels through `TAURI_INVOKE(cmd, args)` (see `src/lib/bindings.ts`), which
`src/wails/shims/tauri-core.ts` dispatches to a PascalCase Go method on the
`app.App` service. Your job for one tool/feature area:

1. Implement the Go methods (behaviour mirroring the Rust source of truth).
2. Report the command→method mapping (the maintainer wires the shim).

## Source of truth (in this order)

1. **Contract**: `src/lib/bindings.ts` — the generated Tauri-specta file.
   - Command names are snake_case inside `TAURI_INVOKE("…", { … })`; the
     wrapper methods are camelCase (`commands.listCodexProfiles`).
   - **Types**: `interface CodexProfile { … }` etc. define the exact JSON
     field names and optionality your Go structs must reproduce.
   - **Who calls what**: grep `src/store/*.ts`, `src/components/**`,
     `src/services/*.ts` for `commands.<camelCase>(` to see the real
     behavioural contract (result handling, error matching).
2. **Behaviour**: `src-tauri/crates/droidgear-core/src/<mod>.rs` (business
   logic, file formats, migration paths) and `src-tauri/src/commands/<mod>.rs`
   (thin command layer: arg marshalling, error strings).
3. **Style reference**: existing Go code — `wails/internal/app/hermes.go`,
   `pi.go`, `omp.go`, `factory.go`, `dsh*.go`; helpers in
   `wails/internal/core/` (`AtomicWrite`, `AtomicWritePrivate`,
   `UnmarshalJSONWithExtra`, `EnsureDir`, paths, WSL).

## Hard rules

- **Only create new files** in `wails/internal/app/` (prefix them with your
  tool name). Never edit existing Go files, `go.mod`/`go.sum`,
  `src/wails/shims/*`, `src/lib/capabilities.ts`, `src/lib/bindings.ts`, or
  anything under `wails/frontend/`. No new third-party dependencies — if you
  truly need one, note it in your report and work around it (stdlib,
  `gopkg.in/yaml.v3`, `github.com/pelletier/go-toml/v2`,
  `modernc.org/sqlite`, `github.com/google/uuid` are available).
- **Methods on `*App`**: `func (a *App) MethodName(arg type) (T, error)`.
  Wails auto-binds exported methods; nothing to register.
- **No symbol collisions**: everything is `package app`. Prefix unexported
  helpers/state with your tool name (`codexParse…`, `openclawProfiles`).
  Exported types use the Rust/bindings names (`CodexProfile`). Never redefine
  a type that already exists in the package (grep first); if you need a
  shared type owned by another in-flight port, redefine it under a
  tool-specific name instead of importing.
- **JSON contract**: struct tags must match `bindings.ts` field-for-field —
  camelCase, `*T` for optional fields, `map[string]X` for record types.
  Where Rust preserves unknown fields (`#[serde(flatten)] extra`), use
  `core.UnmarshalJSONWithExtra` / `MarshalJSONWithExtra` (see `factory.go`).
- **Result wrapper**: Go methods return `(T, error)`; the
  `Result<T,E>` union is produced by bindings.ts. Return `nil, nil` for
  `Result<null, string>` success.
- **Error strings**: keep them byte-identical with the Rust
  `format!(…)`/`.to_string()` messages whenever the frontend matches or
  displays them (grep `src/wails/error-i18n.ts` for the known set — a
  mapping exists for en/zh; do not invent new phrasings for common cases
  like "failed to read …"). For structured errors returned as JSON object
  strings (see `recovery.go`), reproduce the same shape.
- **Events**: where the Rust side emits Tauri events the frontend
  `listen()`s (grep the pages, e.g. `'codex-sessions-changed'`), emit the
  same event names/payloads via the pattern in `pty.go`
  (`application.Get().Event.Emit(name, payload)`), wrapped in a small
  `emit<Tool>Event` helper + optional `…EventSink` var for tests.
- **Watchers** (sessions/specs): Rust uses notify-style file watchers. A
  cheap poll loop (fs.Stat mtime + interval, e.g. 500ms–1s, started
  lazily on `start…Watcher`, stopped on `stop…Watcher`, refcounted) is an
  acceptable mirror; emit the change event only on actual change. Keep
  state in a package-level tool-prefixed struct guarded by `sync.Mutex`.
- **Platform**: Windows is the primary target (same as existing code); use
  `core.GetEffectivePaths`-style path resolution so the Preferences → Paths
  overrides and WSL handling keep working.
- **Stateless where Rust is**: methods resolve paths on demand; no cached
  config. `App` stays an empty struct — package-level vars only for
  watcher/session registries.

## Tests

Add `_test.go` files covering pure logic (parsers, round-trips, migration
of legacy shapes) with `t.TempDir()` fixtures — mirror `hermes_test.go` and
`dsh_config_test.go` style. Never read or write the real user profile.

## Validation (must pass before you report)

```bash
cd wails
gofmt -l .                      # must print nothing for your files
go build ./...
go vet ./...
go test ./...
```

Do **not** run `wails3 generate bindings` or any frontend build — bindings
are regenerated once after all ports land.

## Report format

End your report with a fenced block containing, for every ported command:

```
COMMAND_MAPPING
<tauri_snake_command> -> <PascalMethod> [arg1, arg2, …]   # arg keys as in bindings.ts invoke object, Go positional order
```

plus: any commands deliberately skipped (and why), any error strings that
should be added to `src/wails/error-i18n.ts`, any new Go dependencies you
would have needed, and any behavioural divergences from Rust.
