/**
 * Wails bridge for `@tauri-apps/api/webviewWindow` and
 * `@tauri-apps/api/window`.
 *
 * The minimal shell has no custom titlebar; window operations are no-ops so
 * components that reference the current window do not crash.
 */

export interface WindowOptions {
  title?: string
  width?: number
  height?: number
}

export class WebviewWindow {
  readonly label: string

  constructor(label?: string) {
    this.label = label ?? 'main'
  }

  async listen<T>(
    _event: string,
    _handler: (event: { event: string; payload: T }) => void
  ): Promise<() => void> {
    return () => undefined
  }

  async once<T>(
    _event: string,
    _handler: (event: { event: string; payload: T }) => void
  ): Promise<() => void> {
    return () => undefined
  }

  async emit(_event: string, _payload?: unknown): Promise<void> {
    // no-op
  }

  async show(): Promise<void> {
    // no-op
  }

  async hide(): Promise<void> {
    // no-op
  }

  async close(): Promise<void> {
    window.close()
  }

  async setFocus(): Promise<void> {
    // no-op
  }

  async setTitle(_title: string): Promise<void> {
    // no-op
  }
}

export function getCurrentWindow(): WebviewWindow {
  return new WebviewWindow('main')
}

export function getCurrentWebviewWindow(): WebviewWindow {
  return new WebviewWindow('main')
}
