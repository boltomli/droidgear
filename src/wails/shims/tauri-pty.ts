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

/** Upper bound for data buffered while paused (decoded characters). */
const MAX_PAUSED_CHARS = 4 * 1024 * 1024

// Global bus subscriptions, created lazily and shared by all sessions.
// Events that arrive before a session's start() resolves are buffered by id
// (bounded: pty events are only emitted for live sessions, so a failed
// start must not accumulate forever — the cap drops the oldest chunks).
let dataSubscription: (() => void) | null = null
let exitSubscription: (() => void) | null = null
const dataHandlers = new Map<number, (data: string) => void>()
const exitHandlers = new Map<number, (exitCode: number) => void>()
const pendingData = new Map<number, string[]>()
const pendingExit = new Map<number, number>()
const MAX_PENDING_CHUNKS = 500

function bufferPendingData(id: number, chunk: string): void {
  const buffer = pendingData.get(id) ?? []
  buffer.push(chunk)
  if (buffer.length > MAX_PENDING_CHUNKS) {
    buffer.splice(0, buffer.length - MAX_PENDING_CHUNKS)
  }
  pendingData.set(id, buffer)
}

function ensureSubscriptions(): void {
  if (!dataSubscription) {
    dataSubscription = Events.On('pty:data', ev => {
      const payload = ev.data as PtyEventPayload
      const handler = dataHandlers.get(payload.id)
      if (handler && payload.data !== undefined) {
        handler(payload.data)
      } else if (payload.data !== undefined) {
        bufferPendingData(payload.id, payload.data)
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

  // Local flow control: while paused, decoded output queues here and is
  // flushed on resume. Bounded — once the queue exceeds the cap the shim
  // streams anyway (degrading to "no pause" rather than growing forever).
  private paused = false
  private pausedBuffer: string[] = []
  private pausedChars = 0

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
        if (this.paused) {
          this.pausedBuffer.push(text)
          this.pausedChars += text.length
          if (this.pausedChars > MAX_PAUSED_CHARS) {
            console.warn(
              'Wails PTY shim: pause() buffer overflow, resuming stream'
            )
            this.resume()
          }
          return
        }
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
    // Release the global handler slots and any straggler buffers so
    // repeated open/close cycles do not leak closures per session
    // (guidelines §3.9).
    if (this.pid >= 0) {
      dataHandlers.delete(this.pid)
      exitHandlers.delete(this.pid)
      pendingData.delete(this.pid)
      pendingExit.delete(this.pid)
    }
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
    // Explicit no-op: the terminal buffer lives in xterm.js on the frontend,
    // which clears itself — there is nothing server-side to flush.
    console.warn('Wails PTY shim: clear() is a no-op')
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
    this.paused = true
  }

  resume(): void {
    if (!this.paused) {
      return
    }
    this.paused = false
    this.pausedChars = 0
    const buffered = this.pausedBuffer
    this.pausedBuffer = []
    for (const text of buffered) {
      this._onData.fire(text)
    }
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
