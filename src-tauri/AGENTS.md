# src-tauri — Rust Backend (Tauri v3)

## OVERVIEW

Tauri v3 application shell with 3 crates: `droidgear` (app), `droidgear-core` (business logic), `droidgear-tui` (terminal UI). 175 Rust source files, ~236K LOC. Edition 2021, MSRV 1.95.

## STRUCTURE

```
src-tauri/
├── src/                    # Tauri app shell
│   ├── lib.rs              # Plugin registration, window setup
│   ├── bindings.rs         # tauri-specta TS binding generation
│   ├── types.rs            # Shared command types
│   ├── commands/           # 29 command modules (thin wrappers)
│   └── utils/              # Platform helpers, terminal launch
├── crates/
│   ├── droidgear-core/     # Business logic library (28 modules)
│   │   ├── src/            # One file per domain concept
│   │   ├── tests/          # Characterization tests
│   │   └── res/            # Bundled JSON model registries
│   └── droidgear-tui/      # Terminal UI (ratatui + crossterm)
│       └── src/            # App state, UI rendering, key handlers
├── capabilities/           # Tauri permission manifests
└── Cargo.toml              # Workspace root
```

## WHERE TO LOOK

| Task                | Location                                       | Notes                       |
| ------------------- | ---------------------------------------------- | --------------------------- |
| Add new command     | `src/commands/` + `crates/droidgear-core/src/` | Commands are thin wrappers  |
| Business logic      | `crates/droidgear-core/src/<domain>.rs`        | One file per domain         |
| TUI screens         | `crates/droidgear-tui/src/ui.rs`               | 4K LOC draw() function      |
| TUI modals          | `crates/droidgear-tui/src/tui/modal.rs`        | 4.8K LOC                    |
| Shared types        | `src/types.rs`                                 | Specta-derived for TS       |
| Platform utils      | `src/utils/platform.rs`                        | cfg-gated per OS            |
| TypeScript bindings | `src/bindings.rs`                              | Run `npm run rust:bindings` |
| Plugin registration | `src/lib.rs`                                   | Order matters               |

## CONVENTIONS

- **Command pattern**: `#[tauri::command] #[specta::specta]` → thin wrapper → core function
- **Dual-function**: `fn foo_for_home(home_dir, ...)` + `fn foo(...)` wrapper for testability
- **Error returns**: Core returns `Result<T, String>`, TUI uses `anyhow::Result`
- **Module layout**: Flat in core (one file/domain), commands mirror core 1:1
- **String formatting**: `format!("{variable}")` not `format!("{}", variable)`
- **Platform conditionals**: `#[cfg(desktop)]`, `#[cfg(target_os = "...")]`
- **After adding commands**: Run `npm run rust:bindings` to regenerate TS bindings

## ANTI-PATTERNS

- ❌ Editing `src/lib/bindings.ts` or `src/lib/tauri-bindings.ts` — auto-generated
- ❌ String-based `invoke()` from TS — use `commands` from tauri-bindings
- ❌ Raw `Window` parameter — prefer `AppHandle` when possible
- ❌ Skipping `npm run rust:bindings` after adding commands
