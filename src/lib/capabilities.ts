/**
 * Backend capability gating.
 *
 * The Tauri build exposes every tool and sub-view; the Wails build only has
 * Go implementations for a subset (channels, Dsh, Pi, Hermes, OMP — no
 * sessions viewers, no Codex/OpenCode/Claude/OpenClaw/Copilot/Droid yet).
 * Shared components consult this module so the two shells differ only in
 * what is *enabled*, never in layout: unsupported tools render as disabled
 * placeholders.
 */

import type { NavigationView, ToolView } from '@/store/ui-store'

// Injected by the Wails Vite config (`define`); absent in the Tauri build.
declare const __WAILS_BACKEND__: boolean | undefined

/** Whether this bundle is backed by the Go service instead of Rust/Tauri. */
export const IS_WAILS_BACKEND =
  typeof __WAILS_BACKEND__ !== 'undefined' && __WAILS_BACKEND__ === true

/** Tools whose Go backend is complete enough to enable in the Wails build. */
const WAILS_TOOLS = new Set<NavigationView>([
  'channels',
  'dsh',
  'pi',
  'hermes',
  'omp',
])

/** First tool shown when a persisted view is unsupported (Wails only). */
export const WAILS_FALLBACK_TOOL: ToolView = 'dsh'

/** Whether a top-level tool/view can be opened in this build. */
export function isToolSupported(view: NavigationView): boolean {
  return !IS_WAILS_BACKEND || WAILS_TOOLS.has(view)
}

/**
 * Whether the sessions viewer sub-pages are available. They need read access
 * to per-tool session stores that the Go service does not implement yet.
 */
export function isSessionsSupported(tool: NavigationView): boolean {
  return !IS_WAILS_BACKEND && tool !== 'channels'
}
