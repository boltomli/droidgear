#!/usr/bin/env node
/**
 * Go layering gate (docs/developer/wails-tui-cli-design.zh.md §2 / §7).
 * Run from the repo root:
 *
 *   node scripts/check-go-layering.mjs
 *
 * It asserts the dependency direction
 *
 *   core ← service ← { internal/app, cmd/dg }
 *
 * by walking `go list -deps` for each layer:
 *
 *   R1  internal/service must not depend on wails v3 or internal/app
 *       (a service that emits events or builds menus forces every
 *       consumer — including the CLI — to link the GUI framework)
 *   R3  cmd/dg must not depend on internal/app or wails v3
 *       (internal/app carries Windows-only syscall fields and
 *       //go:embed assets; importing it makes the CLI unbuildable
 *       outside Windows and drags in the whole Wails runtime)
 *   R4  internal/tui must not depend on internal/app or wails v3
 *       (the TUI is the parity consumer of internal/service — see
 *       docs/developer/tui-parity.zh.md §6)
 *
 * Layers that do not exist yet are reported as skipped, so the gate can
 * land in CI before the extraction work starts. Requires a Go toolchain.
 */

import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'

const WAILS_DIR = path.resolve('wails')

/** forbidden import prefixes, per layer */
const RULES = [
  {
    layer: './internal/service/...',
    dir: path.join(WAILS_DIR, 'internal', 'service'),
    name: 'internal/service',
    rules: [
      {
        prefix: 'github.com/wailsapp/wails',
        why: 'R1: service 必须零 Wails 依赖',
      },
      {
        prefix: 'droidgear-wails/internal/app',
        why: 'R1: service 在 app 之下，不得反向依赖',
      },
    ],
  },
  {
    layer: './cmd/dg/...',
    dir: path.join(WAILS_DIR, 'cmd', 'dg'),
    name: 'cmd/dg',
    rules: [
      {
        prefix: 'droidgear-wails/internal/app',
        why: 'R3: CLI 不得 import internal/app（Windows-only 字段 + go:embed）',
      },
      {
        prefix: 'github.com/wailsapp/wails',
        why: 'R3: CLI 必须在无 WebKit/Cocoa 的环境可构建',
      },
    ],
  },
  {
    layer: './internal/tui/...',
    dir: path.join(WAILS_DIR, 'internal', 'tui'),
    name: 'internal/tui',
    rules: [
      {
        prefix: 'droidgear-wails/internal/app',
        why: 'R4: TUI 只能消费 service（对齐矩阵的前置条件）',
      },
      {
        prefix: 'github.com/wailsapp/wails',
        why: 'R4: TUI 必须在无 WebKit/Cocoa 的环境可构建',
      },
    ],
  },
]

/** true when `imp` equals or lives under `prefix` */
function matches(imp, prefix) {
  return imp === prefix || imp.startsWith(prefix + '/')
}

function listPackages(pattern) {
  // -deps prints the whole closure; the template exposes each package's
  // direct imports so a violation can name the importer, not just the dep.
  const out = execFileSync(
    'go',
    ['list', '-deps', '-f', '{{.ImportPath}}\t{{join .Imports ","}}', pattern],
    { cwd: WAILS_DIR, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }
  )
  const packages = []
  for (const line of out.split('\n')) {
    if (!line.trim()) continue
    const [importPath, imports = ''] = line.split('\t')
    packages.push({ importPath, imports: imports ? imports.split(',') : [] })
  }
  return packages
}

let failed = false

for (const rule of RULES) {
  if (!fs.existsSync(rule.dir)) {
    console.log(`skip  ${rule.name} — 不存在（尚未落地）`)
    continue
  }

  let packages
  try {
    packages = listPackages(rule.layer)
  } catch (err) {
    console.error(
      `fail  ${rule.name} — go list 失败：${String(err.stderr || err.message).trim()}`
    )
    failed = true
    continue
  }

  let layerFailed = false
  for (const pkg of packages) {
    // Only report edges between our own packages: the framework's internal
    // graph is noise, the importer is what we can act on.
    if (!pkg.importPath.startsWith('droidgear-wails/')) continue
    for (const imp of pkg.imports) {
      for (const r of rule.rules) {
        if (matches(imp, r.prefix)) {
          console.error(`fail  ${rule.name}: ${pkg.importPath} → ${imp}`)
          console.error(`      ${r.why}`)
          layerFailed = true
        }
      }
    }
  }
  if (layerFailed) failed = true
  else console.log(`ok    ${rule.name} — ${packages.length} 个包，依赖方向正确`)
}

if (failed) {
  console.error(
    '\n分层门禁失败：把违规依赖下沉到 service/core，或把 GUI 专属逻辑留在 internal/app。'
  )
  process.exit(1)
}
console.log('Go layering gate passed')
