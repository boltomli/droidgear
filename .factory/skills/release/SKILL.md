---
name: release
description: Generate changelog from commits since last tag and release new version. Use when user wants to release a new version or update changelog.
---

# Release New Version

## Instructions

1. Get the latest tag:

   ```bash
   git tag --list --sort=-version:refname | head -1
   ```

2. Get commits since last tag:

   ```bash
   git log <latest-tag>..HEAD --oneline
   ```

3. Calculate next version automatically:

   - Parse current version vX.Y.Z
   - Increment: Z+1, if Z=9 then Z=0 and Y+1, if Y=9 then Y=0 and X+1
   - Examples: v0.0.8 → v0.0.9, v0.0.9 → v0.1.0, v0.9.9 → v1.0.0

4. Show suggested version to user, allow modification before proceeding

5. Categorize commits by type:

   - `feat:` → **新功能** / **New Features**
   - `fix:` → **问题修复** / **Bug Fixes**

6. Update README.md changelog section (insert new version entry after `## 更新日志` heading)

7. Update README_EN.md changelog section if exists (insert after `## Changelog` heading)

8. Commit changelog changes (required before release:prepare):

   ```bash
   git add README.md README_EN.md
   git commit -m "docs: update changelog for <version>"
   ```

9. Run release workflow:

   ```bash
   npm run release:prepare <version>
   ```

10. Execute git commands to complete release:

    ```bash
    git add .
    git commit -m "chore: release <version>"
    git tag <version>
    git push origin main --tags
    ```

## Verification

- Confirm changelog entries are correctly formatted
- Confirm version number follows vX.Y.Z pattern
- Confirm both README files are updated (if applicable)
- Confirm changelog is committed before running release:prepare
- Confirm git push completes successfully
