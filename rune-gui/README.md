# droidgear-gui — the Rune GUI spike (P0 + P1)

A native GUI for DroidGear built with [Rune](https://github.com/anomalyco/rune), rendering with
wgpu instead of a WebView. It calls `droidgear-core` **directly** — no Tauri commands, no specta
bindings, no zustand, no TanStack Query. This is the harness for deciding whether the React UI can
move to Rust; it is not wired into the app's build, packaging or CI yet.

```bash
cd rune-gui
cargo run                    # opens the window
cargo run --release -- --snap shot.png --theme dark --size 1280x820   # headless PNG
cargo test                   # the spike's own tests (pure helpers + schema checks)
```

`--scene models|add-model|edit-model|delete-model|specs|pi|codex|copilot|light|english` renders a
specific page or overlay, which is how the screenshots below were made without driving the pointer.

## What works

| Page                                                                                                                                    | State            | Notes                                                                                                                          |
| --------------------------------------------------------------------------------------------------------------------------------------- | ---------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| Shell: sidebar of tools + their pages, breadcrumb, theme/language switches                                                              | ported           | Rows, icons, scroll, focus rings from `rune-kit`; the tools/pages table is in `src/app.rs`                                     |
| Droid → BYOK models: list                                                                                                               | ported           | Reads `~/.factory/settings.json` through `factory_settings::load_custom_models`                                                |
| Droid → BYOK models: add/edit dialog, delete confirmation                                                                               | ported           | Writes through `factory_settings::save_custom_models`; validation, secure API-key field, provider select, image-support switch |
| Droid → Specs: file list + Markdown                                                                                                     | ported           | `specs::list_specs` + `rune-kit`'s `Markdown` (headings, lists, inline code, CJK)                                              |
| Pi → settings: default provider / model / thinking level                                                                                | ported           | `pi::read_pi_settings` / `pi::save_pi_settings`                                                                                |
| Codex → settings: profile chips + model selection + auth                                                                                | ported (scalars) | `codex::*_profile`; the `providers` map editor is P2                                                                           |
| Copilot → settings: profile chips + provider + limits                                                                                   | ported (scalars) | `copilot::*_profile`                                                                                                           |
| Claude / OpenClaw / Hermes / OpenCode / Omp / Dsh / Channels / MCP / Sessions / Missions / Terminal / Trusted folders / Legacy versions | placeholder      | Each says which phase brings it                                                                                                |

## How it is put together

```
src/main.rs     AppSpec (window, fonts, scenes, key handling), `--snap` support comes from Rune
src/app.rs      App state, View/PageId navigation, appearance + language preferences
src/theme.rs    shadcn tokens from src/theme-variables.css -> rune-kit's Kit
src/i18n.rs     locales/*.json -> rune-i18n catalog; `t!("models.title")` is checked at build time
src/ui.rs       sidebar, page header, the shared building blocks
src/pages/      one module per page: models.rs, specs.rs, and form.rs's schema-driven settings
build.rs        lays out the catalogs, runs rune-i18n's codegen, reads package.json's version
```

**No IPC.** A handler is a closure that gets `&mut App` and calls core in the same frame:

```rust
Button::new(t!("common.save"))
    .on_press(|app: &mut App| app.form_mut().save())   // -> pi::save_pi_settings(..) internally
    .build(k)
```

The Tauri build needs `commands/*.rs` (34 wrappers) + `bindings.rs` + generated TypeScript + the
zustand stores + `services/` for the same thing. All of that disappears here; the page keeps a
`serde_json::Value` of the profile and writes it back through core, so fields the schema doesn't
show survive an edit.

**One form engine, one schema per tool.** Every tool in core exposes
`list_<tool>_profiles()` / `get_<tool>_profile(id)` / `save_<tool>_profile(profile)`, and profiles
are `serde` structs, so `src/pages/form.rs` edits them as JSON driven by a `Schema` of
`FieldSpec`s (`Text`, `Secret`, `Number`, `Bool`, `Choice`, `TextArea`, addressed by dotted path in
titled sections). Adding a tool is a schema plus two `match` arms.

**Translations.** `build.rs` turns `locales/{en,zh}.json` into Rune's catalog. `t!("key")` resolves
in a `const`, so a typo is a compile error rather than a blank label; `t!(dynamic_key)` exists for
label keys held in `View`/`PageId`. The `gui.*` entries in `locales/*.json` are the handful of
strings only this GUI needs.

**Theme.** `src/theme-variables.css` is already OKLCH shadcn tokens, so they copy across as Rune's
`Kit` seed (`--background`/`--foreground`/`--primary`) with overrides for borders, muted text, the
destructive colour, the 10 px radius, 13/14 px type and a visible input border.

**Preferences.** Appearance and language are read from and written to the same
`<app data dir>/com.droidgear.app/preferences.json` the Tauri build uses, so the two GUIs agree.

## Measured on this machine (Windows 11, release)

See [PERFORMANCE.md](PERFORMANCE.md) for a like-for-like comparison against the Tauri build
(startup, memory, idle CPU, size) and how it was measured.

|                                                            |                                          |
| ---------------------------------------------------------- | ---------------------------------------- |
| Dependency build (Rune + rune-kit + markdown + core, cold) | ~4 min (`cargo test` cold run)           |
| One-file rebuild (release)                                 | ~60 s                                    |
| Binary                                                     | 25.1 MB unstripped (`droidgear-gui.exe`) |
| First frame (headless, 1280x820, models page)              | 103 ms, 699 draw instances               |
| First frame (Pi form, 1280x820)                            | 18 ms, 98 layout nodes                   |
| GPU device init                                            | 1.0-2.4 s, once per process              |
| Process RSS, ~6 s after launch                             | 139 MB (one sample, window open)         |
| Tests                                                      | 6, instant once built                    |

For scale: the smallest Rune app (`examples/hello`, built out of tree against the same rev) takes
6m29s from a cold `target/` and produces a 20.9 MB binary; `rune-kit` plus markdown adds ~4 min.

## Known gaps

- **List and map editors** (P2): a tool's `providers` map, profile create/duplicate/delete/apply,
  MCP servers, channels, models-from-API discovery. `form.rs` handles scalars only.
- **Terminal** (P3): Rune has no VT emulator. Either write one over `portable-pty` or keep the
  existing "open in external terminal" flow.
- **Packaging and updates** (P4): no updater, single-instance, window-state or installers here.
  `tauri-plugin-updater` + minisign + GitHub Releases have no counterpart in Rune; all of it has to
  be written (or a launcher/Tauri shell kept around it).
- **Sessions, MCP, Missions, Channels, export** and the rest of the pages are placeholders.
- **Accessibility** is wired through Rune/AccessKit (roles, labels, switch state are set), but has
  not been checked with a screen reader.
- **No visual regression suite** yet: `--snap` is there for it, nothing pins the goldens.

## Why a separate workspace

Rune is pre-release: it is pinned to one `dev` rev (`9531586`, see `Cargo.toml`) and brings its own
GPU stack. Keeping this crate out of `src-tauri`'s workspace leaves the Tauri build's lockfile,
`npm run check:all` and CI untouched while the spike is still an experiment.

## Style

Same rules as `src-tauri`: `cargo fmt`, `cargo clippy --all-targets` clean (warnings are fixed, not
allowed), 2021 edition, no `unwrap` on fallible parsing paths that face user data.
