#!/usr/bin/env node
/**
 * Syncs the DroidGear model registry into the Wails spike module.
 *
 * The Rust core embeds src/lib/model-registry-data.json via include_str!; the
 * Go port cannot reach outside its module for //go:embed, so the file is
 * vendored into wails/internal/core/registrydata/. Run this script after
 * editing the registry:
 *
 *   node wails/scripts/sync-registry.mjs
 */
import { copyFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const source = resolve(here, '../../src/lib/model-registry-data.json')
const target = resolve(
  here,
  '../internal/core/registrydata/model-registry-data.json'
)

copyFileSync(source, target)
console.log(`Synced registry: ${source} -> ${target}`)
