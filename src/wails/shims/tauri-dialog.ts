/**
 * Wails bridge for `@tauri-apps/plugin-dialog`.
 *
 * Native file dialogs are not wired up in this minimal build. The only
 * consumer (the channels list page) is not part of the minimal shell, so a
 * thrown error is an acceptable degradation.
 */

export interface DialogFilter {
  name: string
  extensions: string[]
}

export interface OpenDialogOptions {
  multiple?: boolean
  filters?: DialogFilter[]
  title?: string
  defaultPath?: string
}

export interface SaveDialogOptions {
  filters?: DialogFilter[]
  title?: string
  defaultPath?: string
}

export async function open(
  _options?: OpenDialogOptions
): Promise<string | null> {
  throw new Error('Native file dialogs are not available in this minimal build')
}

export async function save(
  _options?: SaveDialogOptions
): Promise<string | null> {
  throw new Error('Native file dialogs are not available in this minimal build')
}
