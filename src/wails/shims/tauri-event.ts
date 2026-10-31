/**
 * Wails bridge for `@tauri-apps/api/event`.
 *
 * The minimal build has no cross-window event flows; listeners register but
 * never fire and `emit` is a no-op (theme changes still apply locally via
 * the ThemeProvider state).
 */

export interface Event<T = unknown> {
  event: string
  id: number
  payload: T
}

export type EventCallback<T> = (event: Event<T>) => void

export type UnlistenFn = () => void

export async function listen<T>(
  _event: string,
  _handler: EventCallback<T>
): Promise<UnlistenFn> {
  return () => undefined
}

export async function once<T>(
  _event: string,
  _handler: EventCallback<T>
): Promise<UnlistenFn> {
  return () => undefined
}

export async function emit(_event: string, _payload?: unknown): Promise<void> {
  // no-op in the minimal Wails build
}
