/**
 * Wails bridge for `@tauri-apps/api/webviewWindow` and
 * `@tauri-apps/api/window`.
 *
 * Backed by the `@wailsio/runtime` Window instance (single main window) and
 * the Wails event bus. Only the surface used by the shared Tauri components
 * is implemented: window commands (`window-commands.ts`), the Windows
 * title bar controls, the close-confirmation flow and the drag region.
 */

import { Window, Events } from '@wailsio/runtime'

export interface WindowOptions {
  title?: string
  width?: number
  height?: number
}

/** Window events mirrored from the Wails event bus (platform + common). */
const RESIZE_EVENTS = [
  'windows:WindowDidResize',
  'mac:WindowDidResize',
  'linux:WindowDidResize',
  'common:WindowDidResize',
]

const FOCUS_EVENTS = [
  'windows:WindowFocus',
  'mac:WindowFocus',
  'linux:WindowFocus',
  'common:WindowFocus',
]

export class WebviewWindow {
  readonly label: string

  constructor(label?: string) {
    this.label = label ?? 'main'
  }

  async listen<T>(
    event: string,
    handler: (event: { event: string; payload: T }) => void
  ): Promise<() => void> {
    const unlisten = Events.On(event, ev => {
      handler({ event, payload: ev.data as T })
    })
    return () => unlisten()
  }

  async once<T>(
    event: string,
    handler: (event: { event: string; payload: T }) => void
  ): Promise<() => void> {
    const unlisten = Events.Once(event, ev => {
      handler({ event, payload: ev.data as T })
    })
    return () => unlisten()
  }

  async emit(_event: string, _payload?: unknown): Promise<void> {
    // No cross-window flows in the Wails build; theming applies locally.
  }

  async show(): Promise<void> {
    Window.Show()
  }

  async hide(): Promise<void> {
    Window.Hide()
  }

  async close(): Promise<void> {
    Window.Close()
  }

  async destroy(): Promise<void> {
    Window.Close()
  }

  async setFocus(): Promise<void> {
    Window.Focus()
  }

  async center(): Promise<void> {
    Window.Center()
  }

  async setTitle(title: string): Promise<void> {
    Window.SetTitle(title)
  }

  async setSize(width: number, height: number): Promise<void> {
    Window.SetSize(width, height)
  }

  async minimize(): Promise<void> {
    Window.Minimise()
  }

  async toggleMaximize(): Promise<void> {
    Window.ToggleMaximise()
  }

  async maximize(): Promise<void> {
    Window.Maximise()
  }

  async unmaximize(): Promise<void> {
    Window.UnMaximise()
  }

  async isMaximized(): Promise<boolean> {
    return Window.IsMaximised()
  }

  async setFullscreen(fullscreen: boolean): Promise<void> {
    if (fullscreen) {
      Window.Fullscreen()
    } else {
      Window.UnFullscreen()
    }
  }

  async isFullscreen(): Promise<boolean> {
    return Window.IsFullscreen()
  }

  /**
   * Resolves on the next window resize. Wails emits per-platform event
   * names plus a common alias; subscribe to all so the handler fires on
   * every backend.
   */
  async onResized(handler: () => void): Promise<() => void> {
    const unlistens = RESIZE_EVENTS.map(name =>
      Events.On(name, () => handler())
    )
    return () => unlistens.forEach(unlisten => unlisten())
  }

  /** Resolves on the next focus change (used to sync maximised state). */
  async onFocusChanged(handler: () => void): Promise<() => void> {
    const unlistens = FOCUS_EVENTS.map(name => Events.On(name, () => handler()))
    return () => unlistens.forEach(unlisten => unlisten())
  }
}

export function getCurrentWindow(): WebviewWindow {
  return new WebviewWindow('main')
}

export function getCurrentWebviewWindow(): WebviewWindow {
  return new WebviewWindow('main')
}
