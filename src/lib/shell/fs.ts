/**
 * File-system helpers used by the JSON import/export flows.
 *
 * Only the surface used by the channels list page (JSON import/export) is
 * implemented, routed through the Go `read_text_file` / `write_text_file`
 * commands. The Go side rejects relative paths, keeping the exposure narrow.
 */

import { invoke } from './commands'

export async function readTextFile(path: string): Promise<string> {
  return invoke<string>('read_text_file', { path })
}

export async function writeTextFile(
  path: string,
  contents: string
): Promise<void> {
  await invoke<null>('write_text_file', { path, contents })
}
