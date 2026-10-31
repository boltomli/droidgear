/**
 * Wails bridge for `@tauri-apps/plugin-os`.
 *
 * Only `locale()` is used by the frontend (language auto-detection); the
 * browser's navigator.language is a faithful substitute inside the webview.
 */

export async function locale(): Promise<string | null> {
  return navigator.language ?? null
}

export async function platform(): Promise<string> {
  return 'windows'
}

export async function arch(): Promise<string> {
  return navigator.userAgent.includes('ARM') ? 'aarch64' : 'x86_64'
}

export async function version(): Promise<string> {
  return navigator.userAgent
}

export function family(): string {
  return 'windows'
}

export function hostname(): string {
  return window.location.hostname
}
