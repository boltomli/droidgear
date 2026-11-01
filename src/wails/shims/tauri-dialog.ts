/**
 * Wails bridge for `@tauri-apps/plugin-dialog`.
 *
 * Backed by the Wails runtime dialogs (WebView2 native pickers on Windows).
 * `open()` maps to OpenFile (single or multiple), `save()` to SaveFile.
 */

import { Dialogs } from '@wailsio/runtime'

export interface DialogFilter {
  name: string
  extensions: string[]
}

export interface OpenDialogOptions {
  multiple?: boolean
  filters?: DialogFilter[]
  title?: string
  defaultPath?: string
  directory?: boolean
}

export interface SaveDialogOptions {
  filters?: DialogFilter[]
  title?: string
  defaultPath?: string
}

interface WailsFileFilter {
  DisplayName: string
  Pattern: string
}

function mapFilters(filters?: DialogFilter[]): WailsFileFilter[] {
  return (filters ?? []).map(filter => ({
    DisplayName: filter.name,
    Pattern: filter.extensions.map(ext => `*.${ext}`).join(';'),
  }))
}

export async function open(
  options?: OpenDialogOptions
): Promise<string | string[] | null> {
  const result = await Dialogs.OpenFile({
    Title: options?.title ?? '',
    Directory: options?.defaultPath ?? '',
    CanChooseDirectories: options?.directory === true,
    CanChooseFiles: options?.directory !== true,
    AllowsMultipleSelection: options?.multiple === true,
    Filters: mapFilters(options?.filters),
  })
  const paths = Array.isArray(result) ? result : result ? [result] : []
  const first = paths[0]
  if (first === undefined) {
    return null
  }
  return options?.multiple ? paths : first
}

export async function save(
  options?: SaveDialogOptions
): Promise<string | null> {
  const result = await Dialogs.SaveFile({
    Title: options?.title ?? '',
    Directory: options?.defaultPath ?? '',
    Filename: '',
    Filters: mapFilters(options?.filters),
  })
  return typeof result === 'string' && result !== '' ? result : null
}
