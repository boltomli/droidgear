#!/usr/bin/env node
/**
 * Command-surface reconciliation for the Wails port (guidelines §8.1 /
 * appendix B, made executable). Run from the repo root:
 *
 *   node scripts/check-command-surface.mjs
 *
 * It asserts:
 *  1. Every Tauri command the frontend actually invokes has a Wails shim
 *     entry (allowlist: the Tauri-only updater trio).
 *  2. Every shim entry maps to a generated Go binding with matching
 *     argument count and order.
 *  3. Every Go service method is accounted for: routed through the shim,
 *     imported directly by a shim module (PTY/opener), or listed in the
 *     dead-methods baseline.
 *  4. The set of Rust binding commands the frontend never calls does not
 *     grow beyond the baseline (shrink it deliberately when dead commands
 *     are removed).
 *
 * Requires a current `wails3 generate bindings` output
 * (`wails3 task build` produces it).
 */

import fs from 'node:fs'

// --- baselines (update deliberately, never silently) ---------------------

/** Frontend-invoked, but Tauri-only by design (see capabilities.ts). */
const UPDATER_ALLOWLIST = new Set([
  'check_portable_update',
  'install_portable_update',
  'get_update_channel',
])

/** Go methods imported straight from `@wails-bindings` by shim modules. */
const DIRECT_GO_METHODS = new Set([
  'GetSystemFonts', // src/wails/shims/tauri-system-fonts.ts
  'OpenPath',
  'OpenUrl',
  'RevealItemInDir', // src/wails/shims/tauri-opener.ts
  'PtyKill', // src/wails/shims/tauri-pty.ts
  'PtyResize',
  'PtyStart',
  'PtyWrite',
])

// The P3 cleanup removed every dead command (appendix A) from Rust, the Go
// service and the shim. These Go methods survive without a shim entry
// because other (still live) Go commands call them internally — their Rust
// command wrappers are gone; new internal-only helpers should be
// unexported instead.
const INTERNAL_GO_METHODS = new Set([
  'GetHermesProfile',
  'GetOmpProfile',
  'GetOpencodeProfile',
  'GetPiProfile',
  'ListDshProfiles',
  'SetActiveOmpProfileId',
  'SetActivePiProfileId',
])
const DEAD_GO_METHODS = new Set()
const UNUSED_RUST_COMMANDS = new Set()

// --- load sources ---------------------------------------------------------

const read = p => {
  if (!fs.existsSync(p)) {
    console.error(
      `check-command-surface: missing ${p} — run \`wails3 generate bindings\` first`
    )
    process.exit(1)
  }
  return fs.readFileSync(p, 'utf8')
}

const bindings = read('src/lib/bindings.ts')
const shim = read('src/wails/shims/tauri-core.ts')
const generated = read(
  'wails/frontend/bindings/droidgear-wails/internal/app/app.ts'
)

// --- parse ----------------------------------------------------------------

// bindings.ts: camelCase fn -> invoke name
const fnToCmd = {}
for (const chunk of bindings.split(/\nasync /).slice(1)) {
  const name = /^([A-Za-z0-9_]+)\(/.exec(chunk)?.[1]
  const cmd = /TAURI_INVOKE[^(]*\("([a-z0-9_]+)"/.exec(chunk)?.[1]
  if (name && cmd) fnToCmd[name] = cmd
}

// generated bindings: Go method -> ordered param names
const goParams = {}
const goMethods = []
for (const m of generated.matchAll(
  /export function ([A-Za-z0-9_]+)\(([^)]*)\)/g
)) {
  goMethods.push(m[1])
  goParams[m[1]] =
    m[2].trim() === ''
      ? []
      : m[2]
          .split(',')
          .map(s => s.trim().split(':')[0].replace(/\?$/, ''))
          .filter(Boolean)
}

// shim: command -> { fn, args }
const specs = {}
for (const m of shim.matchAll(
  /([a-z0-9_]+): \{\s*fn: '([A-Za-z0-9_]+)',\s*args: \[([^\]]*)\]/g
)) {
  specs[m[1]] = {
    fn: m[2],
    args: m[3]
      .split(',')
      .map(s => s.trim().replace(/^'|'$/g, ''))
      .filter(Boolean),
  }
}

// --- walk -----------------------------------------------------------------

function* walk(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = `${dir}/${entry.name}`
    if (entry.isDirectory()) yield* walk(p)
    else if (/\.(ts|tsx)$/.test(entry.name)) yield p
  }
}

// Frontend usage: commands.*() call sites (mapped through bindings.ts) and
// direct invoke('literal') calls (src/wails/menu-bridge.ts). Comments are
// stripped first so dead sample code (logger.ts) does not count as usage.
const stripComments = text =>
  text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:])\/\/[^\n]*/g, '$1')

const used = new Set()
for (const file of walk('src')) {
  if (file.endsWith('bindings.ts')) continue
  const text = stripComments(fs.readFileSync(file, 'utf8'))
  for (const m of text.matchAll(/commands\.([A-Za-z0-9_]+)\(/g)) {
    if (fnToCmd[m[1]]) used.add(fnToCmd[m[1]])
  }
  for (const m of text.matchAll(/invoke\(\s*'([a-z0-9_]+)'/g)) {
    used.add(m[1])
  }
}

// --- checks ---------------------------------------------------------------

const problems = []
const norm = s => s.toLowerCase()

// 1. frontend invocation -> shim
for (const cmd of used) {
  if (!specs[cmd] && !UPDATER_ALLOWLIST.has(cmd)) {
    problems.push(`前端调用但 shim 未实现: ${cmd}`)
  }
}

// 2. shim -> generated binding (name, argument count and order)
for (const [cmd, spec] of Object.entries(specs)) {
  const params = goParams[spec.fn]
  if (!params) {
    problems.push(`shim → 缺生成绑定: ${cmd} (${spec.fn})`)
  } else if (params.length !== spec.args.length) {
    problems.push(`参数个数不一致: ${cmd} shim=${spec.args} go=${params}`)
  } else {
    spec.args.forEach((a, i) => {
      if (norm(a) !== norm(params[i]))
        problems.push(`参数名不一致: ${cmd}[${i}] shim=${a} go=${params[i]}`)
    })
  }
}

// 3. every Go method is shimmed, directly imported or baselined as dead
const accounted = new Set([
  ...Object.values(specs).map(s => s.fn),
  ...DIRECT_GO_METHODS,
  ...INTERNAL_GO_METHODS,
  ...DEAD_GO_METHODS,
])
for (const fn of goMethods) {
  if (!accounted.has(fn)) {
    problems.push(
      `Go 方法无归属（新增命令需接 shim / 直连白名单 / 死命令基线）: ${fn}`
    )
  }
}

// 4. Rust-side dead commands must not grow (fnToCmd keys are camelCase
// binding names; the values are the snake_case invoke commands).
const unusedRust = new Set(Object.values(fnToCmd).filter(cmd => !used.has(cmd)))
const added = [...unusedRust].filter(c => !UNUSED_RUST_COMMANDS.has(c))
const removed = [...UNUSED_RUST_COMMANDS].filter(c => !unusedRust.has(c))
if (added.length > 0)
  problems.push(
    `Rust 死命令清单增长（先确认是否为废弃面）: ${added.join(', ')}`
  )
if (removed.length > 0)
  problems.push(`Rust 死命令已消失 — 请从脚本基线移除: ${removed.join(', ')}`)

console.log(
  `shim=${Object.keys(specs).length} goMethods=${goMethods.length} rustBindings=${Object.keys(fnToCmd).length} used=${used.size}`
)

if (problems.length) {
  console.error(problems.join('\n'))
  process.exit(1)
}
console.log('command surface OK')
