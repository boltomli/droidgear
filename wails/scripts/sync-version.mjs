#!/usr/bin/env node
// Sync the application version from the repo-root package.json — the single
// source of truth shared with the Tauri build — into every place the Wails
// build reads it from:
//
//   build/config.yml                  info.version (wails templates source)
//   build/windows/info.json           fixed.file_version + ProductVersion (syso)
//   build/windows/nsis/wails_tools.nsh  INFO_PRODUCTVERSION (NSIS display version)
//   build/windows/nsis/project.nsi    VIProductVersion / VIFileVersion literals
//   build/windows/msix/*.xml          Identity/Properties Version (MSIX)
//
// Version shapes (package.json is the single source of truth; the release tag
// is `wails-v<package.json version>` and must match it exactly):
//
//   full     1.3.9.4            printed to stdout → Go ldflags → About pane
//   numeric  1.3.9.4            prerelease/build metadata stripped, 4th part kept
//   base     1.3.9              first three parts — config.yml only, because the
//                               wails msix template appends `.0` itself and
//                               darwin CFBundleShortVersionString wants x.y.z
//   win      1.3.9.4            exactly four parts — NSIS VIProductVersion /
//                               VIFileVersion and MSIX Version require x.y.z.w
//
// For a three-part version (`1.3.10`) `win` pads `.0` → `1.3.10.0`, so a
// four-part version like `1.3.9.4` must NEVER get another `.0` appended.
//
// The Go binary picks the version up at link time via
// `-ldflags "-X droidgear-wails/internal/version.Version=<version>"` (see
// build/windows/Taskfile.yml and build/Taskfile.yml), which invokes this
// script to capture the value. `GetAppVersion` therefore reports the same
// version as the Tauri app (About/General panes).
//
// Usage:
//   node wails/scripts/sync-version.mjs           # sync files, print version
//   node wails/scripts/sync-version.mjs --check   # exit 1 on drift (CI), no writes

import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const wailsDir = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '..'
)
const repoRoot = path.resolve(wailsDir, '..')
const check = process.argv.includes('--check')

const { version } = JSON.parse(
  fs.readFileSync(path.join(repoRoot, 'package.json'), 'utf8')
)
if (!/^\d+\.\d+\.\d+(\.\d+)?(-[\w.]+)?(\+[\w.]+)?$/.test(version)) {
  console.error(
    `sync-version: unexpected version format in package.json: ${version}`
  )
  process.exit(1)
}

// Numeric form (strips `-prerelease` / `+buildmeta`, keeps an optional 4th
// part) — feeds info.json and the NSIS display version.
const numericVersion = version.replace(
  /^([0-9]+\.[0-9]+\.[0-9]+(?:\.[0-9]+)?).*$/,
  '$1'
)

// First three parts only — feeds config.yml: the wails msix template appends
// `.0` to info.version itself (a 4th part would become a 5-part, invalid
// Version) and darwin CFBundleShortVersionString wants x.y.z.
const baseVersion = numericVersion.split('.').slice(0, 3).join('.')

// Exactly four parts — feeds NSIS VIProductVersion/VIFileVersion and MSIX
// Version, which require x.y.z.w.
const winVersion =
  numericVersion.split('.').length === 4
    ? numericVersion
    : `${numericVersion}.0`

const changes = []

function apply(relativePath, transform) {
  const file = path.join(wailsDir, relativePath)
  const before = fs.readFileSync(file, 'utf8')
  const after = transform(before)
  if (after === before) return
  changes.push(relativePath)
  if (!check) fs.writeFileSync(file, after)
}

function replaceAll(label, text, pattern, replacement) {
  const matches = text.match(new RegExp(pattern, 'g')) ?? []
  if (matches.length === 0) {
    console.error(`sync-version: pattern not found in ${label}: ${pattern}`)
    process.exit(1)
  }
  return text.replace(new RegExp(pattern, 'g'), replacement)
}

// build/config.yml — the info.version entry (two-space indented, double
// quoted; the top-level taskfile `version: '3'` and commented ios override
// use different shapes and are left alone).
apply('build/config.yml', text => {
  const pattern = /^ {2}version: "[^"]*"/gm
  const matches = text.match(pattern) ?? []
  if (matches.length !== 1) {
    console.error(
      `sync-version: expected exactly one info.version entry in build/config.yml, found ${matches.length}`
    )
    process.exit(1)
  }
  return text.replace(pattern, `  version: "${baseVersion}"`)
})

// build/windows/info.json — embedded into the Windows version resource.
apply('build/windows/info.json', text => {
  const json = JSON.parse(text)
  json.fixed.file_version = numericVersion
  for (const block of Object.values(json.info ?? {})) {
    block.ProductVersion = numericVersion
  }
  return `${JSON.stringify(json, null, '\t')}\n`
})

// build/windows/nsis/wails_tools.nsh — NSIS installer defines (display
// version: VIAddVersionKey ProductVersion + registry DisplayVersion keep the
// full numeric version, 4th part included).
apply('build/windows/nsis/wails_tools.nsh', text =>
  replaceAll(
    'build/windows/nsis/wails_tools.nsh',
    text,
    '(!define INFO_PRODUCTVERSION ")[^"]*(")',
    `$1${numericVersion}$2`
  )
)

// build/windows/nsis/project.nsi — VIProductVersion/VIFileVersion must be
// exactly x.y.z.w, so they are managed as literals here instead of relying on
// a `${INFO_PRODUCTVERSION}.0` append (which would turn 1.3.9.4 into
// 1.3.9.4.0 and makensis would reject it). The `\s+` keeps the original
// alignment; `[^\"]*` also matches a regenerated `${INFO_PRODUCTVERSION}.0`
// macro, so a wails template refresh is re-synced on the next run.
apply('build/windows/nsis/project.nsi', text =>
  replaceAll(
    'build/windows/nsis/project.nsi',
    text,
    '(VIProductVersion|VIFileVersion)(\\s+)"[^"]*"',
    `$1$2"${winVersion}"`
  )
)

// build/windows/msix manifests — exactly 4-part version (x.y.z.w; for a
// three-part package.json version `winVersion` carries the padded `.0`). The
// lookbehind keeps MinVersion / MaxVersionTested untouched.
for (const file of [
  'build/windows/msix/template.xml',
  'build/windows/msix/app_manifest.xml',
]) {
  apply(file, text =>
    replaceAll(
      file,
      text,
      '(?<!\\w)Version="\\d+\\.\\d+\\.\\d+(?:\\.\\d+)?"',
      `Version="${winVersion}"`
    )
  )
}

if (check && changes.length > 0) {
  console.error(
    `sync-version: version drift in: ${changes.join(', ')} — run \`node wails/scripts/sync-version.mjs\``
  )
  process.exit(1)
}

if (!check && changes.length > 0) {
  console.error(`sync-version: updated ${changes.join(', ')}`)
}
process.stdout.write(version)
