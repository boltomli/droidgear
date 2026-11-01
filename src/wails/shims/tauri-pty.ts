/**
 * Wails bridge for the `tauri-pty` npm package.
 *
 * The Rust plugin (spawn/read/resize/write/kill over Tauri IPC) is replaced
 * by the Go PTY service (`PtyStart` etc. in wails/internal/app/pty.go).
 * Output and exit are streamed over the Wails event bus as base64 chunks,
 * reassembled here with a streaming TextDecoder so multibyte characters
 * split across reads stay intact.
 *
 * The public surface mirrors tauri-pty's `spawn()` / `IPty` so
 * `TerminalView.tsx` needs no changes.
 */

import { Events } from '@wailsio/runtime'
import {
  PtyStart,
  PtyWrite,
  PtyResize,
  PtyKill,
  type PtyStartOptions,
} from '@wails-bindings'

export interface IPtyForkOptions {
  name?: string
  cols?: number
  rows?: number
  cwd?: string
  env?: Record<string, string | undefined>
  encoding?: string | null
  handleFlowControl?: boolean
  flowControlPause?: string
  flowControlResume?: string
}

export interface IDisposable {
  dispose(): void
}

export type IEvent<T> = (listener: (e: T) => void) => IDisposable

export interface IPty {
  readonly pid: number
  readonly cols: number
  readonly rows: number
  readonly process: string
  handleFlowControl: boolean
  readonly onData: IEvent<string>
  readonly onExit: IEvent<{ exitCode: number; signal?: number }>
  resize(columns: number, rows: number): void
  clear(): void
  write(data: string): void
  kill(signal?: string): void
  pause(): void
  resume(): void
}

export type ArgvOrCommandLine = string[] | string

class EventEmitter<T> {
  private listeners: [(e: T) => void, IDisposable][] = []

  readonly event: IEvent<T> = (listener: (e: T) => void) => {
    const disposable: IDisposable = {
      dispose: () => {
        const index = this.listeners.findIndex(([fn]) => fn === listener)
        if (index >= 0) {
          this.listeners.splice(index, 1)
        }
      },
    }
    this.listeners.push([listener, disposable])
    return disposable
  }

  fire(data: T): void {
    for (const [listener] of [...this.listeners]) {
      listener(data)
    }
  }

  get size(): number {
    return this.listeners.length
  }
}

interface PtyEventPayload {
  id: number
  data?: string
  exitCode?: number
}

// Global bus subscriptions, created lazily and shared by all sessions.
// Events that arrive before a session's start() resolves are buffered by id.
let dataSubscription: (() => void) | null = null
let exitSubscription: (() => void) | null = null
const dataHandlers = new Map<number, (data: string) => void>()
const exitHandlers = new Map<number, (exitCode: number) => void>()
const pendingData = new Map<number, string[]>()
const pendingExit = new Map<number, number>()

function ensureSubscriptions(): void {
  if (!dataSubscription) {
    dataSubscription = Events.On('pty:data', ev => {
      const payload = ev.data as PtyEventPayload
      const handler = dataHandlers.get(payload.id)
      if (handler && payload.data !== undefined) {
        handler(payload.data)
      } else if (payload.data !== undefined) {
        const buffer = pendingData.get(payload.id) ?? []
        buffer.push(payload.data)
        pendingData.set(payload.id, buffer)
      }
    })
  }
  if (!exitSubscription) {
    exitSubscription = Events.On('pty:exit', ev => {
      const payload = ev.data as PtyEventPayload
      const exitCode = payload.exitCode ?? -1
      const handler = exitHandlers.get(payload.id)
      if (handler) {
        handler(exitCode)
      } else {
        pendingExit.set(payload.id, exitCode)
      }
    })
  }
}

class WailsPty implements IPty {
  readonly pid: number
  cols: number
  rows: number
  readonly process: string
  handleFlowControl = false

  private readonly _onData = new EventEmitter<string>()
  private readonly _onExit = new EventEmitter<{
    exitCode: number
    signal?: number
  }>()
  private readonly decoder = new TextDecoder()
  private exited = false

  private readonly init: Promise<void>

  constructor(file: string, args: string[], opt?: IPtyForkOptions) {
    this.cols = opt?.cols ?? 80
    this.rows = opt?.rows ?? 24
    this.process = file
    // Placeholder id until start resolves; sessions are keyed server-side.
    this.pid = -1

    const env: Record<string, string> = {}
    for (const [key, value] of Object.entries(opt?.env ?? {})) {
      if (value !== undefined) {
        env[key] = value
      }
    }

    const options: PtyStartOptions = {
      file,
      args,
      name: opt?.name ?? null,
      cols: opt?.cols ?? null,
      rows: opt?.rows ?? null,
      cwd: opt?.cwd ?? null,
      env,
    }

    ensureSubscriptions()
    this.init = PtyStart(options).then((id: number) => {
      ;(this as { pid: number }).pid = id
      dataHandlers.set(id, data => this.handleData(data))
      exitHandlers.set(id, code => this.handleExit(code))
      const buffered = pendingData.get(id)
      if (buffered) {
        pendingData.delete(id)
        for (const chunk of buffered) {
          this.handleData(chunk)
        }
      }
      const bufferedExit = pendingExit.get(id)
      if (bufferedExit !== undefined) {
        pendingExit.delete(id)
        this.handleExit(bufferedExit)
      }
    })
  }

  get onData(): IEvent<string> {
    return this._onData.event
  }

  get onExit(): IEvent<{ exitCode: number; signal?: number }> {
    return this._onExit.event
  }

  private handleData(base64: string): void {
    if (this.exited) {
      return
    }
    try {
      const binary = atob(base64)
      const bytes = new Uint8Array(binary.length)
      for (let i = 0; i < binary.length; i++) {
        bytes[i] = binary.charCodeAt(i)
      }
      // stream: true keeps incomplete multibyte sequences for the next chunk.
      const text = this.decoder.decode(bytes, { stream: true })
      if (text) {
        this._onData.fire(text)
      }
    } catch {
      // Malformed chunk — drop rather than corrupt the terminal buffer.
    }
  }

  private handleExit(exitCode: number): void {
    if (this.exited) {
      return
    }
    this.exited = true
    this._onExit.fire({ exitCode })
  }

  resize(columns: number, rows: number): void {
    this.cols = columns
    this.rows = rows
    void this.init
      .then(() => PtyResize(this.pid, columns, rows))
      .catch(e => {
        console.error('Resize error: ', e)
      })
  }

  clear(): void {
    console.warn('clear is un implemented!')
  }

  write(data: string): void {
    void this.init
      .then(() => PtyWrite(this.pid, data))
      .catch(e => {
        console.error('Writing error: ', e)
      })
  }

  kill(_signal?: string): void {
    void this.init
      .then(() => PtyKill(this.pid))
      .catch(e => {
        console.error('Kill error: ', e)
      })
  }

  pause(): void {
    throw new Error('Method not implemented.')
  }

  resume(): void {
    throw new Error('Method not implemented.')
  }
}

export function spawn(
  file: string,
  args: ArgvOrCommandLine,
  options?: IPtyForkOptions
): IPty {
  const argv = typeof args === 'string' ? [args] : (args ?? [])
  return new WailsPty(file, argv, options)
}
