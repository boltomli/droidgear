/**
 * Re-export the command facade with project conventions
 *
 * This file provides type-safe access to every backend command. The
 * facade itself (`src/lib/bindings.ts`) carries the snake_case command
 * shapes; `src/lib/shell/commands.ts` maps them onto the Go service.
 *
 * @example
 * ```typescript
 * import { commands, unwrapResult } from '@/lib/app-bindings'
 *
 * // In TanStack Query - let errors propagate
 * const prefs = unwrapResult(await commands.loadPreferences())
 *
 * // In event handlers - explicit error handling
 * const result = await commands.savePreferences(prefs)
 * if (result.status === 'error') {
 *   toast.error(result.error)
 * }
 * ```
 *
 * @see docs/developer/tauri-commands.md for full documentation
 */

export { commands, type Result } from './bindings'
export type {
  AppPreferences,
  JsonValue,
  RecoveryError,
  SettingsFileInfo,
  TrustedFolder,
} from './bindings'

/**
 * Helper to unwrap a Result type, throwing on error
 */
export function unwrapResult<T, E>(
  result: { status: 'ok'; data: T } | { status: 'error'; error: E }
): T {
  if (result.status === 'ok') {
    return result.data
  }
  throw result.error
}
