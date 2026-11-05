# Archive — Tauri / Rust era

Documents that only describe the **Tauri shell and the Rust workspace**,
removed from this repository in `fe22c57`. They are kept for history;
**none of the commands in them run anymore** (`cargo …`, `npm run tauri:*`,
`npm run rust:*`, `npm run tui*`, `src-tauri/…`, `@tauri-apps/*`).

Current architecture and commands:

- [`../../agents/wails-port.md`](../../agents/wails-port.md) — Wails shell traps + validation
- [`../../agents/commands.md`](../../agents/commands.md) — build / lint / test / CI tags
- [`../../../wails/README.md`](../../../wails/README.md) — shell overview and release lines

| Document                                           | Was about                                       |
| -------------------------------------------------- | ----------------------------------------------- |
| [tauri-commands.md](./tauri-commands.md)           | Type-safe Rust–TypeScript bridge (tauri-specta) |
| [tauri-plugins.md](./tauri-plugins.md)             | Tauri plugin usage and configuration            |
| [rust-architecture.md](./rust-architecture.md)     | Rust module organization and patterns           |
| [code-style-rust.md](./code-style-rust.md)         | Rust code style and cargo commands              |
| [bundle-optimization.md](./bundle-optimization.md) | Tauri bundle/binary size tuning                 |
| [external-apis.md](./external-apis.md)             | HTTP via reqwest in the Rust backend            |
| [logging.md](./logging.md)                         | `tauri-plugin-log`                              |
| [releases.md](./releases.md)                       | Tauri signing, installers and updater manifest  |
