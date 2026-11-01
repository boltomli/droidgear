#!/usr/bin/env node
// Sync the application version from the repo-root package.json — the single
// source of truth shared with the Tauri build — into every place the Wails
// build reads it from:
//
//   build/config.yml                  info.version (installer metadata source)
//   build/windows/info.json           fixed.file_version + ProductVersion (syso)
//   build/windows/nsis/wails_tools.nsh  INFO_PRODUCTVERSION (NSIS installer)
//   build/windows/msix/*.xml          Identity/Properties Version (MSIX)
//
// The Go binary picks the version up at link time via
// `-ldflags "-X droidgear-wails/internal/app.appVersion=<version>"` (see
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
if (!/^\d+\.\d+\.\d+(-[\w.]+)?$/.test(version)) {
  console.error(
    `sync-version: unexpected version format in package.json: ${version}`
  )
  process.exit(1)
}

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
  return text.replace(pattern, `  version: "${version}"`)
})

// build/windows/info.json — embedded into the Windows version resource.
apply('build/windows/info.json', text => {
  const json = JSON.parse(text)
  json.fixed.file_version = version
  for (const block of Object.values(json.info ?? {})) {
    block.ProductVersion = version
  }
  return `${JSON.stringify(json, null, '\t')}\n`
})

// build/windows/nsis/wails_tools.nsh — NSIS installer defines.
apply('build/windows/nsis/wails_tools.nsh', text =>
  replaceAll(
    'build/windows/nsis/wails_tools.nsh',
    text,
    '(!define INFO_PRODUCTVERSION ")[^"]*(")',
    `$1${version}$2`
  )
)

// build/windows/msix manifests — 4-part version (x.y.z.0). The lookbehind
// keeps MinVersion / MaxVersionTested untouched.
for (const file of [
  'build/windows/msix/template.xml',
  'build/windows/msix/app_manifest.xml',
]) {
  apply(file, text =>
    replaceAll(
      file,
      text,
      '(?<!\\w)Version="\\d+\\.\\d+\\.\\d+\\.\\d+"',
      `Version="${version}.0"`
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
