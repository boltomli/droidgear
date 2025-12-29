---
allowed-tools: [Read, Edit, Execute, Glob, TodoWrite]
description: 'Generate changelog from commits since last tag and release new version'
---

# /release - Release New Version

## Purpose

Automatically generate changelog from commits since the last tag, update README.md, and prepare a new release.

## Usage

```
/release [version]
```

If version is omitted, auto-increment from latest tag using rule: vX.Y.Z where Z+1, if Z=9 then Z=0 and Y+1, if Y=9 then Y=0 and X+1.

Example: `/release` or `/release v0.1.0`

## Execution

1. Get the latest tag: `git tag --list --sort=-version:refname | head -1`
2. Get commits since last tag: `git log <latest-tag>..HEAD --oneline`
3. Calculate next version automatically (or use provided version):
   - Parse current version vX.Y.Z
   - Increment: Z+1, overflow to Y+1 (reset Z=0), overflow to X+1 (reset Y=0, Z=0)
   - Examples: v0.0.8 → v0.0.9, v0.0.9 → v0.1.0, v0.9.9 → v1.0.0
4. Show suggested version to user, allow modification before proceeding
5. Categorize commits by type:
   - `feat:` → **新功能** / **New Features**
   - `fix:` → **问题修复** / **Bug Fixes**
6. Update README.md changelog section (insert new version entry after `## 更新日志` heading)
7. Update README_EN.md changelog section if it exists (insert after `## Changelog` heading)
8. Run `npm run release:prepare <version>` to execute the release workflow
