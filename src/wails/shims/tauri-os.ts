/**
 * Wails bridge for `@tauri-apps/plugin-os`.
 *
 * Mirrors the Tauri plugin's sync API: `platform()` / `family()` / `version()`
 * / `arch()` are synchronous (TerminalView and use-platform call them
 * synchronously), `locale()` stays async. The spike is Windows-only, so the
 * platform answers are static; the language auto-detect uses the browser's
 * navigator.language as a faithful substitute inside the webview.
 */

export function platform(): 'windows' {
  return 'windows'
}

export function family(): 'windows' {
  return 'windows'
}

export function version(): string {
  return navigator.userAgent
}

export function arch(): 'x86_64' | 'aarch64' {
  return navigator.userAgent.includes('ARM') ? 'aarch64' : 'x86_64'
}

export async function locale(): Promise<string | null> {
  return navigator.language ?? null
}

export function hostname(): string {
  return window.location.hostname
}
