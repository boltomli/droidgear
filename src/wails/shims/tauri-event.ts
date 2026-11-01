/**
 * Wails bridge for `@tauri-apps/api/event`.
 *
 * Subscribes to the Wails event bus. The PTY shim listens for `pty:data` /
 * `pty:exit` through this module; the theming layer emits nothing (single
 * window, local state).
 */

import { Events } from '@wailsio/runtime'

export interface Event<T = unknown> {
  event: string
  id: number
  payload: T
}

export type EventCallback<T> = (event: Event<T>) => void

export type UnlistenFn = () => void

let nextListenerId = 1

export async function listen<T>(
  event: string,
  handler: EventCallback<T>
): Promise<UnlistenFn> {
  const id = nextListenerId++
  const unlisten = Events.On(event, ev => {
    handler({ event, id, payload: ev.data as T })
  })
  return () => unlisten()
}

export async function once<T>(
  event: string,
  handler: EventCallback<T>
): Promise<UnlistenFn> {
  const id = nextListenerId++
  const unlisten = Events.Once(event, ev => {
    handler({ event, id, payload: ev.data as T })
  })
  return () => unlisten()
}

export async function emit(_event: string, _payload?: unknown): Promise<void> {
  // No frontend→backend event flows are needed in this build.
}
