# Dependency Updates

Policy and procedure for updating dependencies in this repo.

## Branch Policy

The `tauri-v3` branch exists to test new versions. Dependencies must stay as current
as possible:

- **Tauri-related** (npm `@tauri-apps/*`, Rust `tauri`/`tauri-plugin-*`): always use the
  newest `next`/alpha releases. Never hold these back.
- **Everything else**: update aggressively. Majors are welcome as long as
  `npm run check:all` passes. If a major genuinely breaks, keep the old version and
  record the reason in the commit message — don't silently skip it.

`main` tracks stable releases; this branch intentionally runs ahead.

## Tauri Packages

### npm (exact pins)

Tauri npm packages are pinned to **exact** alpha versions in `package.json`
(`"3.0.0-alpha.3"`), so `npm update` will never move them. Check and bump manually:

```bash
npm view @tauri-apps/api dist-tags     # "next" = newest alpha
npm view @tauri-apps/cli dist-tags
npm view @tauri-apps/plugin-dialog dist-tags
```

Bump the `@tauri-apps/*` pins to the newest alpha reported by the `next` dist-tag.
Keep `src-tauri/Cargo.toml` crate versions on the matching alpha line.

### Rust (caret ranges)

`tauri` / `tauri-plugin-*` in `src-tauri/Cargo.toml` use caret ranges on
`3.0.0-alpha.x`, so `cargo update` picks up new alphas automatically:

```bash
cargo update --dry-run     # in src-tauri — check for tauri alpha bumps
```

## Routine Update Procedure

```bash
npm outdated                # what's available
npm update                  # in-range npm updates (lockfile only)
cd src-tauri && cargo update # in-range crate updates
npm audit fix
npm run format              # newer Prettier often reformats files
npm run check:all           # MUST pass — typecheck, lint, tests, build
```

For packages where `npm outdated` shows `Latest > Wanted`, bump the range in
`package.json` explicitly (e.g. `npm install -D pkg@latest`), then re-verify.

## Rules

- ✅ Run the full `npm run check:all` after updating — it includes the production
  build, which catches missing/renamed package exports
- ✅ Commit as `chore: update npm and cargo dependencies`
- ❌ Leaving a major behind "for later" without noting why
- ❌ Bumping Tauri npm pins without checking the `next` dist-tag first

## Further Reading

- `docs/agents/commands.md` — all check/fix commands
- `docs/developer/static-analysis.md` — what `check:all` runs
