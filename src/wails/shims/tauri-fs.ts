/**
 * Wails bridge for `@tauri-apps/plugin-fs`.
 *
 * Direct filesystem access is not exposed in this minimal build. The only
 * consumer (the channels list page) is not part of the minimal shell.
 */

export async function readTextFile(_path: string): Promise<string> {
  throw new Error('Filesystem access is not available in this minimal build')
}

export async function writeTextFile(
  _path: string,
  _contents: string
): Promise<void> {
  throw new Error('Filesystem access is not available in this minimal build')
}
