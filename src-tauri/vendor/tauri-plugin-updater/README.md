# Vendored: tauri-plugin-updater 3.0.0-alpha.3

This directory is a copy of the published crate
[`tauri-plugin-updater 3.0.0-alpha.3`](https://crates.io/crates/tauri-plugin-updater/3.0.0-alpha.3)
(MIT / Apache-2.0), referenced from the workspace root through `[patch.crates-io]`.

## Why

The published alpha depends on `zip = "4"` on Windows, and `zip 4.6.1` carries the
archive-extraction advisories

| Advisory | Issue |
| -------- | ----- |
| RUSTSEC-2026-0367 | SUID/SGID bits preserved during extraction |
| RUSTSEC-2026-0368 | symlinks whose targets escape the destination directory |
| RUSTSEC-2026-0369 | Unix permissions applied through symlinks |
| RUSTSEC-2026-0429 | process abort on a symlink entry with a large declared size |
| RUSTSEC-2026-0545 | AES entries accepted without checking the authentication code |
| RUSTSEC-2026-0548 | extracted entries use local-header names that differ from the central directory |

All six are fixed in `zip >= 9`. Upstream's `v3` branch still requires `zip = "4"`,
so there is no plugin release to upgrade to yet.

Reachability from DroidGear: the plugin only unpacks a `.zip` updater bundle on
Windows (`ZipArchive::extract` into a temp dir) — and it does so **after**
verifying the minisign signature, so exploiting these requires a validly signed
malicious archive, i.e. a compromised update signing key. The patch is therefore
hygiene, not an exploited path — but it removes a dependency advisory from the
audit gate and costs two lines.

## What changed

Only `Cargo.toml` (and `Cargo.toml.orig` for provenance):

```diff
 [target.'cfg(target_os = "windows")'.dependencies.zip]
-version = "4"
+version = "9"
 optional = true
 default-features = false
+features = ["deflate-flate2-zlib-rs"]
```

`deflate-flate2-zlib-rs` keeps deflate-compressed bundles readable with a pure
Rust backend (the crate defaults are disabled upstream).

Everything under `src/`, `permissions/`, `build.rs` and `api-iife.js` is
byte-identical to the published crate.

## How to remove

Once a published `tauri-plugin-updater` depends on `zip >= 9`:

1. delete this directory,
2. delete the `[patch.crates-io]` block in `src-tauri/Cargo.toml`,
3. `cd src-tauri && cargo update -p tauri-plugin-updater`,
4. `cd src-tauri && cargo audit` — it must stay clean.

Refresh after any upstream change:

```bash
CARGO_HOME=~/.cargo cargo vendor --versioned-dirs  # or re-copy from the registry cache
diff -ru <registry>/tauri-plugin-updater-3.0.0-alpha.3/src \
        src-tauri/vendor/tauri-plugin-updater/src
```
