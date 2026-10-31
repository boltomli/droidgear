#!/usr/bin/env node
/**
 * Syncs vendored data files into the Wails spike module.
 *
 * The Go module cannot `//go:embed` files outside its directory, so two
 * assets are vendored. Run this script after editing either source:
 *
 *   node wails/scripts/sync-registry.mjs
 *
 * - src/lib/model-registry-data.json        → internal/core/registrydata/
 * - src-tauri/crates/droidgear-core/res/
 *     dsh-config-schema.json                → internal/app/schema/
 *
 * Both copies must stay byte-identical to their sources (the schema is also
 * checked by Go unit tests; .prettierignore excludes the vendored copies).
 */
import { copyFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const repoRoot = resolve(here, '../..')

const copies = [
  [
    'src/lib/model-registry-data.json',
    'wails/internal/core/registrydata/model-registry-data.json',
  ],
  [
    'src-tauri/crates/droidgear-core/res/dsh-config-schema.json',
    'wails/internal/app/schema/dsh-config-schema.json',
  ],
]

for (const [source, target] of copies) {
  copyFileSync(resolve(repoRoot, source), resolve(repoRoot, target))
  console.log(`Synced ${source} -> ${target}`)
}
