/**
 * Wails bridge for `tauri-plugin-system-fonts-api`.
 *
 * Routes `getSystemFonts()` to the Go font enumerator
 * (`internal/app/fonts.go` → registry on Windows, `fc-list`/font-directory
 * scan on POSIX), keeping the npm package's `SystemFont[]` shape so
 * `AppearancePane` (terminal-font dropdown) works unchanged in both
 * shells. Errors surface as raw Go strings, like the other shims.
 */

import { GetSystemFonts, type SystemFont } from '@wails-bindings'

export type { SystemFont }

/** Rejections carry a Wails RuntimeError; surface the raw Go error string. */
function toRawError(error: unknown): unknown {
  if (error && typeof error === 'object' && 'cause' in error) {
    const cause = (error as { cause?: unknown }).cause
    if (typeof cause === 'string') return cause
  }
  return error instanceof Error ? error.message : error
}

export async function getSystemFonts(): Promise<SystemFont[]> {
  try {
    // The generated binding types a Go slice as `T[] | null` (nil slice
    // marshals to null); the npm API always resolves an array.
    return (await GetSystemFonts()) ?? []
  } catch (error) {
    throw toRawError(error)
  }
}
