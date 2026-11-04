#!/usr/bin/env node
/**
 * "No Tauri" gate — keeps the repo from growing Tauri references again.
 *
 * Run from the repo root:
 *
 *   node scripts/check-no-tauri.mjs        (also: npm run check:no-tauri)
 *
 * It fails when a tracked file outside the historical documentation still
 * mentions the removed Tauri toolchain, and when the deleted directories
 * reappear. Scope is deliberately narrow: `docs/**` and `wails/PORTING.md`
 * keep the historical record, and Go sources keep their "ported from
 * src-tauri/..." provenance comments.
 *
 * File list comes from `git ls-files` so node_modules / target / dist are
 * never scanned.
 */

import fs from 'node:fs'
import { execFileSync } from 'node:child_process'

/** Directories that must not exist any more. */
const FORBIDDEN_PATHS = ['src-tauri', 'rust-toolchain.toml', 'justfile']

/** Files/dirs whose content is scanned. */
const SCOPES = [
  { prefix: 'src/' },
  { prefix: '.github/workflows/' },
  { prefix: 'scripts/' },
  { exact: 'package.json' },
  { exact: 'package-lock.json' },
  { exact: 'vite.config.ts' },
  { exact: 'vitest.config.ts' },
  { exact: 'index.html' },
  { exact: 'eslint.config.js' },
  { exact: 'knip.json' },
  { exact: 'tsconfig.json' },
  { prefix: 'wails/build/' },
  { exact: 'wails/Taskfile.yml' },
]

/** Substrings that must not appear in a scoped file. */
const FORBIDDEN = [
  '@tauri-apps/',
  'tauri-pty',
  'tauri-plugin-',
  'tauri-specta',
  'src-tauri',
  'droidgear-tui',
  'rust-toolchain',
  'rustup',
  'cargo build',
  'cargo test',
  'cargo zigbuild',
]

const files = execFileSync('git', ['ls-files'], { encoding: 'utf8' })
  .split('\n')
  .filter(Boolean)

// This checker names the forbidden strings itself — it must not scan itself.
const SELF = 'scripts/check-no-tauri.mjs'

const inScope = file =>
  file !== SELF &&
  SCOPES.some(scope =>
    scope.exact !== undefined
      ? file === scope.exact
      : file.startsWith(scope.prefix)
  )

const problems = []

for (const path of FORBIDDEN_PATHS) {
  if (fs.existsSync(path)) {
    problems.push(`禁止存在的路径仍然存在: ${path}`)
  }
}

let scanned = 0
for (const file of files.filter(inScope)) {
  // Deletions that are not staged yet show up in `git ls-files` but not on
  // disk — nothing to scan there.
  if (!fs.existsSync(file)) continue
  scanned += 1
  const text = fs.readFileSync(file, 'utf8')
  for (const needle of FORBIDDEN) {
    if (text.includes(needle)) {
      const line = text.split('\n').findIndex(l => l.includes(needle))
      problems.push(`${file}:${line + 1} 仍引用 "${needle}"`)
    }
  }
}

console.log(`check-no-tauri: scanned ${scanned} files`)

if (problems.length > 0) {
  console.error(problems.join('\n'))
  console.error(
    '\nTauri 已移除（docs/ 与 wails/PORTING.md 的历史记录除外）。' +
      '新代码请写进 src/lib/shell/* 或 wails/internal/service。'
  )
  process.exit(1)
}
console.log('check-no-tauri OK')
