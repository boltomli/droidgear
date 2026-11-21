# Vendored crates

Four crates live here because there is no release on crates.io that works with
`tauri 3.0.0-alpha`. Three of them are unmodified pre-release ports of plugins
that upstream only publishes for Tauri 2; one is a two-line patch for a `zip`
advisory. All four are wired in as path dependencies (`src-tauri/Cargo.toml`) or
through `[patch.crates-io]`, and the workspace `exclude = ["vendor"]` keeps them
out of `cargo test` / `cargo clippy`.

| Directory | Upstream | Published version | Why vendored |
| --------- | -------- | ----------------- | ------------ |
| `tauri-plugin-updater/` | crates.io `tauri-plugin-updater 3.0.0-alpha.3` | 3.0.0-alpha.3 | `[patch.crates-io]`: the published alpha pins `zip = "4"` on Windows → RUSTSEC-2026-0367/0368/0369/0429/0545/0548. Only `Cargo.toml` differs (`zip = "9"` + `deflate-flate2-zlib-rs`). See its own README for removal steps. |
| `tauri-plugin-pty/` | github.com/Tnze/tauri-plugin-pty @ `d7d8d66` | 0.3.1 (Tauri 2 only) | The Tauri 3 port is committed on the author's branch but never published: crates.io 0.3.1 depends on `tauri ^2`. Only difference: `tauri` / `tauri-plugin` bumped to 3.0.0-alpha. |
| `tauri-plugin-system-fonts/` | github.com/ayangweb/tauri-plugin-system-fonts @ `f48e581` | 2.0.2 (Tauri 2 only) | Same story: crates.io 2.0.2 depends on `tauri ^2`; the local working copy carries an uncommitted two-line bump to 3.0.0-alpha. Only `Cargo.toml` differs. |
| `tauri-specta/` | github.com/specta-rs/tauri-specta @ `14f834f` | 2.0.0-rc.29 (Tauri 2 only) | crates.io rc.29 depends on `tauri ^2`. The fork retargets the workspace to 3.0.0-alpha; the vendored copy inlines the workspace-inherited versions and drops the `[workspace]`/`[lints]` tables so it builds standalone. `specta` / `specta-typescript` / macros are unchanged. |

## Ground rules

- **Do not edit vendored code.** If a fix is needed, carry it as a separate,
  documented patch (like the updater `zip` bump) so the diff against upstream
  stays a couple of lines.
- Regenerate `src/lib/bindings.ts` after touching `tauri-specta/`
  (`npm run rust:bindings`) and include the result in the same commit.
- `.prettierignore` excludes this directory so the files stay byte-identical.

## Removing a crate

1. Check whether the upstream release now targets Tauri 3
   (`https://crates.io/api/v1/crates/<name>/<version>/dependencies` — look at the
   `tauri` requirement).
2. If yes: delete the directory, point the dependency back at crates.io in
   `src-tauri/Cargo.toml`, drop any `[patch.crates-io]` entry, then
   `cd src-tauri && cargo update -p <name>` and re-run
   `npm run check:all` + `cargo audit`.
