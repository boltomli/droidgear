/**
 * Clipboard access (Wails runtime clipboard).
 *
 * Used by the terminal for copy-on-select, Ctrl+Shift+C/V and snippet
 * insertion.
 */

import { Clipboard } from '@wailsio/runtime'

export async function writeText(text: string): Promise<void> {
  await Clipboard.SetText(text)
}

export async function readText(): Promise<string> {
  const text = await Clipboard.Text()
  return typeof text === 'string' ? text : ''
}
