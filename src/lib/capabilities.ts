/**
 * Backend capability gating.
 *
 * Both builds expose every tool and sub-view: the Go service behind the
 * Wails build now mirrors the full Tauri command surface (channels, Dsh, Pi,
 * Hermes, OMP, Codex, OpenCode, Claude, OpenClaw, Copilot, Droid + sessions
 * viewers). Shared components consult this module so the two shells stay
 * identical in layout; the gating hooks remain for future Tauri-only
 * surfaces (e.g. the auto-updater).
 */

import type { NavigationView, ToolView } from '@/store/ui-store'

// Injected by the Wails Vite config (`define`); absent in the Tauri build.
declare const __WAILS_BACKEND__: boolean | undefined

/** Whether this bundle is backed by the Go service instead of Rust/Tauri. */
export const IS_WAILS_BACKEND =
  typeof __WAILS_BACKEND__ !== 'undefined' && __WAILS_BACKEND__ === true

/** Tools whose Go backend is complete (all ten shipped since the parity push). */
const WAILS_TOOLS = new Set<NavigationView>([
  'channels',
  'dsh',
  'pi',
  'hermes',
  'omp',
  'codex',
  'opencode',
  'claude',
  'openclaw',
  'copilot',
  'droid',
])

/** First tool shown when a persisted view is unsupported (Wails only). */
export const WAILS_FALLBACK_TOOL: ToolView = 'dsh'

// Every NavigationView has a Go backend now; keep the function for future
// capability gating (e.g. updater) without touching call sites.

/** Whether a top-level tool/view can be opened in this build. */
export function isToolSupported(view: NavigationView): boolean {
  return !IS_WAILS_BACKEND || WAILS_TOOLS.has(view)
}

/**
 * Whether the auto-updater is wired up in this build.
 *
 * The updater is Tauri-only by design (no update feed ships with the Wails
 * build — see wails/README.md and guidelines §3.7), so update-related
 * entries (Preferences → General check button, the debug update toast) are
 * hidden rather than left to fail with "unsupported command" at runtime.
 */
export function isUpdaterSupported(): boolean {
  return !IS_WAILS_BACKEND
}

/**
 * Whether the sessions viewer sub-pages are available. The Go service
 * implements read-only session scanning for every tool (Dsh, Pi, OMP,
 * Codex, Droid), so they are on in both builds.
 */
export function isSessionsSupported(tool: NavigationView): boolean {
  return tool !== 'channels'
}
