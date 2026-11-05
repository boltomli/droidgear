#!/usr/bin/env node
/**
 * TUI parity gate — docs/developer/tui-parity.zh.md §6.
 *
 * Parses the GUI sub-view unions in src/store/ui-store.ts and asserts every
 * sub-view has a row in the alignment matrix (§2). A new GUI sub-view that
 * nobody classifies (对齐 / GUI-only 豁免) fails the build, which is exactly
 * the drift class the matrix exists to prevent.
 *
 * Run: node scripts/check-tui-parity.mjs
 */

import fs from 'node:fs'

const STORE = 'src/store/ui-store.ts'
const MATRIX = 'docs/developer/tui-parity.zh.md'

const read = p => {
  if (!fs.existsSync(p)) {
    console.error(`check-tui-parity: missing ${p}`)
    process.exit(1)
  }
  return fs.readFileSync(p, 'utf-8')
}

// GUI-only sub-views that never get a TUI counterpart (tui-parity §1: PTY
// panes live inside every tool view). Listed here as the script's exemption
// registry — the matrix marks them GUI-only in prose, this list keeps the
// check mechanical.
const EXEMPT_SUBVIEWS = new Set(['terminal'])

// NavigationView id → the name the matrix uses in its first column.
const NAV_NAMES = {
  droid: 'Droid',
  channels: 'Channels',
  opencode: 'OpenCode',
  codex: 'Codex',
  claude: 'Claude',
  openclaw: 'OpenClaw',
  hermes: 'Hermes',
  pi: 'Pi',
  omp: 'OMP',
  copilot: 'Copilot',
  dsh: 'Dsh',
}

// --- parse ui-store.ts -----------------------------------------------------

const store = read(STORE)

/** Extract a union type: `export type X = 'a' | 'b'` (possibly multi-line). */
function unionOf(name) {
  const start = store.indexOf(`export type ${name} =`)
  if (start < 0) return null
  let body = store.slice(start + `export type ${name} =`.length)
  const stop = body.search(/\n(export |interface |const |\/\/ ---)/)
  if (stop >= 0) body = body.slice(0, stop)
  return body
    .split('\n')
    .map(line => line.replace(/\s*\/\/.*$/, ''))
    .join(' ')
    .split('|')
    .map(part => part.trim())
    .flatMap(part => {
      const m = /^'([a-z-]+)'$/.exec(part)
      return m ? [m[1]] : []
    })
}

const navView = unionOf('NavigationView')
if (!navView || navView.length === 0) {
  console.error(`check-tui-parity: NavigationView not found in ${STORE}`)
  process.exit(1)
}

const subviewTypes = [...store.matchAll(/export type (\w+SubView) =/g)].map(
  m => m[1]
)
if (subviewTypes.length === 0) {
  console.error(`check-tui-parity: no *SubView unions found in ${STORE}`)
  process.exit(1)
}

const subviews = new Map() // type -> tokens
for (const type of subviewTypes) {
  const tokens = unionOf(type)
  if (!tokens || tokens.length === 0) {
    console.error(`check-tui-parity: ${type} parsed empty`)
    process.exit(1)
  }
  subviews.set(type, tokens)
}

// --- parse the matrix ------------------------------------------------------

const md = read(MATRIX)
const section2 = /^## 2\. 对齐矩阵\s*$([\s\S]*?)(?=^## )/m.exec(md)
if (!section2) {
  console.error(`check-tui-parity: §2 对齐矩阵 not found in ${MATRIX}`)
  process.exit(1)
}

const rows = section2[1]
  .split('\n')
  .filter(line => line.startsWith('|'))
  .filter(line => !/^\|\s*-+/.test(line)) // separator row
  .slice(1) // header row
  .map(line =>
    line
      .split('|')
      .slice(1, -1)
      .map(cell => cell.trim())
  )

if (rows.length === 0) {
  console.error('check-tui-parity: the matrix table parsed to zero rows')
  process.exit(1)
}

// Every backticked token in a row counts as covered (子视图 column, mostly).
const covered = new Set()
const entryColumn = []
for (const cells of rows) {
  if (cells.length < 2) continue
  entryColumn.push(cells[0])
  for (const cell of cells) {
    for (const m of cell.matchAll(/`([a-z][a-z0-9-]*)`/g)) covered.add(m[1])
  }
}

// --- checks ----------------------------------------------------------------

const problems = []

for (const [type, tokens] of subviews) {
  for (const token of tokens) {
    if (EXEMPT_SUBVIEWS.has(token)) continue
    if (!covered.has(token)) {
      problems.push(
        `${type} 的子视图 \`${token}\` 不在对齐矩阵里 — ` +
          `要么加一行（对齐），要么按 §1 标为 GUI-only 豁免`
      )
    }
  }
}

const allEntries = entryColumn.join('\n')
for (const id of navView) {
  const name = NAV_NAMES[id]
  if (!name) {
    problems.push(
      `NavigationView 未知入口 ${id} — 在 NAV_NAMES 里登记它的矩阵名称`
    )
    continue
  }
  const re = new RegExp(`(^|[^A-Za-z])${name}([^A-Za-z]|$)`, 'm')
  if (!re.test(allEntries)) {
    problems.push(`导航入口 ${id} 在矩阵第一列没有对应行`)
  }
}

if (problems.length > 0) {
  console.error(
    `check-tui-parity: ${problems.length} 个问题 — ${MATRIX} §2 与 ${STORE} 脱节`
  )
  for (const p of problems) console.error(`fail  ${p}`)
  process.exit(1)
}

console.log(
  `ok    TUI parity gate passed — ${subviewTypes.length} 个子视图类型 / ` +
    `${navView.length} 个导航入口 / ${rows.length} 行矩阵全部对上`
)
