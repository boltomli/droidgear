---
name: release
description: Generate changelog from commits since last tag and release new version. Use when user wants to release a new version or update changelog.
---

# Release New Version

## Instructions

> **Current policy (2026-11, `spike/wails-v3-minimal`):** releases ship as
> `wails-v<version>` single-file builds only (draft release, no installers,
> no `latest.json`). Do not ask which pipeline to use — use the
> "single-binary preview" flow below and version `x.y.z.w` (4th part
> increments). Only switch to the official `v*` pipeline when the user
> explicitly asks for installers/updater.

1. Get the latest tag:

   ```bash
   git tag --list --sort=-version:refname | head -1
   ```

2. Get commits since last tag:

   ```bash
   git log <latest-tag>..HEAD --oneline
   ```

3. Calculate next version based on latest tag:
   - Parse current version vX.Y.Z (or x.y.z.w) from step 1
   - Default: increment patch (Z+1), e.g. v0.3.2 → v0.3.3
   - Minor: increment minor, reset patch (Y+1, Z=0), e.g. v0.3.2 → v0.4.0
   - Major: increment major, reset others (X+1, Y=0, Z=0), e.g. v0.3.2 → v1.0.0
   - Optional 4th part (`x.y.z.w`): a rebuild of an existing release that must
     produce a _newer_ version than the previous `x.y.z` build, e.g. 1.3.9 →
     1.3.9.4. Keep Z stable and increment W only; never reuse a W.

4. Show suggested version to user, allow modification before proceeding

5. Categorize commits by type:
   - `feat:` → **New Features / 新功能**
   - `fix:` → **Bug Fixes / 问题修复**

6. Update CHANGELOG.md (insert new version entry after `# Changelog` heading):
   - Use bilingual format (English / 中文) for each item
   - Format: `## vX.Y.Z` (or `## vX.Y.Z.W` for a 4th-part version — it must
     match the release tag exactly, since CI extracts notes by heading) followed
     by `**New Features / 新功能**` and `**Bug Fixes / 问题修复**` sections
   - Each item format: `- English description / 中文描述`
   - Translation terminology: "Channel" must be translated as "渠道" (NOT "频道")
   - This file is used by GitHub Actions to generate release notes for auto-updater

7. (Conditional) If this release includes significant new features or major changes, update the feature descriptions in both README files:
   - README.md — Update relevant feature sections in Chinese
   - README_EN.md — Update relevant feature sections in English
   - Only update feature/capability descriptions, NOT the changelog section (both READMEs already link to CHANGELOG.md for full changelog)

8. Commit changelog changes (required before release:prepare):

   ```bash
   # If README files were updated:
   git add CHANGELOG.md README.md README_EN.md
   # Otherwise:
   git add CHANGELOG.md
   git commit -m "docs: update changelog for <version>"
   ```

9. Run release workflow:

   ```bash
   npm run release:prepare <version>
   ```

10. Sync the Wails installer metadata from the new package.json version
    (otherwise `wails.yml`'s `sync-version --check` gate fails and the
    Windows installer metadata lags behind):

    ```bash
    node wails/scripts/sync-version.mjs
    ```

11. Execute git commands to complete release (include the synced Wails files).
    **Tag only after the version bump is committed** — the tag must always be
    created on the commit that already contains the matching `package.json`
    version, otherwise the built exe reports a different version than the tag
    and the updater cannot correlate them:

    ```bash
    git add .
    git commit -m "chore: release <version>"
    git tag <version>          # <version> == package.json version (v-prefix for official releases)
    git push origin main --tags
    ```

12. Verify the tag and version agree before pushing:

    ```bash
    # must print the same version (minus the tag prefix)
    git describe --tags --exact-match
    node -p "require('./package.json').version"
    node wails/scripts/sync-version.mjs --check   # no drift
    ```

## Optional: single-binary preview (Wails)

When the goal is just "3 个单文件给测试者"（no installers / no updater / no
`latest.json`），do NOT use a plain `v*` tag (that triggers release.yml, the
official installer pipeline). But **the version still comes first** — the
single-binary build reads its version from `package.json` via
`sync-version.mjs` (ldflags → About pane), so tagging without bumping
`package.json` ships an exe whose version does not match the tag:

```bash
# 1. Set the new version FIRST (4th part allowed: 1.3.9.4), then sync metadata:
npm pkg set version=1.3.9.4
npm install --package-lock-only
node wails/scripts/sync-version.mjs

# 2. Commit the bump before tagging (tag must point at this commit):
git add package.json package-lock.json wails/build
git commit -m "chore: release 1.3.9.4"

# 3. Tag with EXACTLY the package.json version (tag prefix `wails-v*` is
#    required: release.yml triggers on `v*`) and push:
git tag wails-v1.3.9.4 && git push origin HEAD --tags
```

Notes:

- The tag is `wails-v<version>` where `<version>` is the literal
  `package.json` version — no more `-1`/`-4` counter suffixes; the counter
  moved into the 4th version part (`x.y.z.w`), so exe version == tag.
- CHANGELOG entry heading must be `## v<version>` (e.g. `## v1.3.9.4`) so the
  workflow can extract release notes (it strips the `wails-` prefix).
- It runs from the tagged commit (no `main` merge needed) and creates a draft
  release. See `docs/agents/commands.md` §"CI / Release tags".
- **Channel**: every release is `x.y.z.w` — the version never encodes the
  channel. A tag push (no inputs) publishes a GitHub **pre-release** → the
  updater's `preview` channel, which is the default. To ship a **stable**
  release, dispatch the workflow with `prerelease=false` (or untick "Set as a
  pre-release" when publishing the draft); re-dispatching an existing tag with
  `prerelease=false` promotes that release to stable. See
  `docs/developer/self-update-single-file.zh.md` §3.0.

## Verification

- Confirm CHANGELOG.md is updated with English changelog (used for auto-updater)
- Confirm changelog entries are correctly formatted
- Confirm version number follows vX.Y.Z or vX.Y.Z.W pattern
- Confirm both README files are updated if significant features were added
- Confirm changelog is committed before running release:prepare
- Confirm `node wails/scripts/sync-version.mjs` ran after release:prepare
  (Wails installer metadata matches package.json)
- Confirm the tag was created **after** the version bump commit and equals
  `wails-v` + `package.json` version (single-binary) or `v` + version (official)
- Confirm `git describe --tags --exact-match` on the tagged commit matches
  `package.json` version — this is what keeps exe version ↔ tag ↔ updater aligned
- Confirm git push completes successfully
