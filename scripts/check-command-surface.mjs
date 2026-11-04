#!/usr/bin/env node
/**
 * Command-surface reconciliation for the single-shell frontend (wails
 * guidelines §8.1 / appendix B, made executable). Run from the repo root:
 *
 *   node scripts/check-command-surface.mjs
 *
 * It asserts:
 *  1. Every command the frontend actually invokes has a shell entry in
 *     `src/lib/shell/commands.ts`.
 *  2. Every shell entry maps to a generated Go binding with matching
 *     argument count and order.
 *  3. Every Go service method is accounted for: routed through the shell
 *     table, imported directly by a shell module (PTY/opener/fonts), or
 *     listed in a baseline (internal-only helpers).
 *  4. Baselines do not rot (a baselined method must still exist).
 *
 * Requires a current `wails3 generate bindings` output
 * (`wails3 task build` produces it).
 */

import fs from 'node:fs'

// --- baselines (update deliberately, never silently) ---------------------

/** Go methods imported straight from `@wails-bindings` by shell modules. */
const DIRECT_GO_METHODS = new Set([
  'GetSystemFonts', // src/lib/shell/fonts.ts
  'OpenPath',
  'OpenUrl',
  'RevealItemInDir', // src/lib/shell/opener.ts
  'PtyKill', // src/lib/shell/pty.ts
  'PtyResize',
  'PtyStart',
  'PtyWrite',
])

// The P3 cleanup removed every dead command (appendix A) from the Go service
// and the shell. These Go methods survive without a shell entry because
// other (still live) Go commands call them internally — new internal-only
// helpers should be unexported instead.
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
const shell = read('src/lib/shell/commands.ts')
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

// shell table: command -> { fn, args }
const specs = {}
for (const m of shell.matchAll(
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
// direct invoke('literal') calls (src/lib/shell/menu-bridge.ts). Comments are
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

// 1. frontend invocation -> shell entry
for (const cmd of used) {
  if (!specs[cmd]) {
    problems.push(`前端调用但 shell 未实现: ${cmd}`)
  }
}

// 2. shell entry -> generated binding (name, argument count and order)
for (const [cmd, spec] of Object.entries(specs)) {
  const params = goParams[spec.fn]
  if (!params) {
    problems.push(`shell → 缺生成绑定: ${cmd} (${spec.fn})`)
  } else if (params.length !== spec.args.length) {
    problems.push(`参数个数不一致: ${cmd} shell=${spec.args} go=${params}`)
  } else {
    spec.args.forEach((a, i) => {
      if (norm(a) !== norm(params[i]))
        problems.push(`参数名不一致: ${cmd}[${i}] shell=${a} go=${params[i]}`)
    })
  }
}

// 3. every Go method is routed through the shell, imported directly or baselined
const accounted = new Set([
  ...Object.values(specs).map(s => s.fn),
  ...DIRECT_GO_METHODS,
  ...INTERNAL_GO_METHODS,
  ...DEAD_GO_METHODS,
])
for (const fn of goMethods) {
  if (!accounted.has(fn)) {
    problems.push(
      `Go 方法无归属（新增命令需接 shell / 直连白名单 / 基线）: ${fn}`
    )
  }
}

// 4. stale baselines: a baselined method that no longer exists in the
// generated bindings means the baseline has rotted.
const goMethodSet = new Set(goMethods)
for (const name of DIRECT_GO_METHODS) {
  if (!goMethodSet.has(name)) {
    problems.push(`基线项已不存在 — 请从脚本移除: ${name}`)
  }
}

console.log(
  `shell=${Object.keys(specs).length} goMethods=${goMethods.length} facade=${Object.keys(fnToCmd).length} used=${used.size}`
)

if (problems.length) {
  console.error(problems.join('\n'))
  process.exit(1)
}
console.log('command surface OK')
