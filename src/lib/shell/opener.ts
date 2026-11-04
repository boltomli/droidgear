/**
 * Opening URLs, paths and file-manager reveals.
 *
 * Routes `openUrl` / `openPath` / `revealItemInDir` to the Go opener service
 * (`internal/app/opener.go`): rundll32 on Windows, `open` on macOS and
 * `xdg-open` on Linux. Used by AboutPane, McpPage, LegacyVersionsPage,
 * ExportTemplatesPage and GeneralPane's releases link.
 */

import { OpenPath, OpenUrl, RevealItemInDir } from '@wails-bindings'

/** Rejections carry a Wails RuntimeError; surface the raw Go error string. */
function toRawError(error: unknown): unknown {
  if (error && typeof error === 'object' && 'cause' in error) {
    const cause = (error as { cause?: unknown }).cause
    if (typeof cause === 'string') return cause
  }
  return error instanceof Error ? error.message : error
}

export async function openUrl(url: string, _options?: unknown): Promise<void> {
  try {
    await OpenUrl(url)
  } catch (error) {
    throw toRawError(error)
  }
}

export async function openPath(
  path: string,
  _options?: unknown
): Promise<void> {
  try {
    await OpenPath(path)
  } catch (error) {
    throw toRawError(error)
  }
}

export async function revealItemInDir(item: string): Promise<void> {
  try {
    await RevealItemInDir(item)
  } catch (error) {
    throw toRawError(error)
  }
}
