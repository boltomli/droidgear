/**
 * OS facts for the shell.
 *
 * `platform()` / `family()` / `version()` / `arch()` / `type()` are
 * synchronous (use-platform and TerminalView call them synchronously),
 * `locale()` / `hostname()` stay async.
 *
 * Platform facts come from the Wails runtime, not the UA:
 * `internal/runtime/runtime_prod.go` injects
 * `window._wails.environment = { OS, Arch, Debug }` (Go's runtime.GOOS /
 * runtime.GOARCH) before any page script runs, so the sync getters read it
 * directly and only fall back to UA sniffing in a plain browser (dev
 * preview). `version()` has no sync source — the OS version is only
 * exposed through the async `System.Environment()` call — so this module
 * warms a cache at load time and the sync getter returns it (round2 §B8).
 */
import { System } from '@wailsio/runtime'

/** Values mirror the shape the components already consume. */
export type Platform =
  | 'linux'
  | 'macos'
  | 'ios'
  | 'freebsd'
  | 'dragonfly'
  | 'netbsd'
  | 'openbsd'
  | 'solaris'
  | 'android'
  | 'windows'
export type Family = 'unix' | 'windows'
export type OsType = 'linux' | 'windows' | 'macos' | 'ios' | 'android'
export type Arch =
  | 'x86'
  | 'x86_64'
  | 'arm'
  | 'aarch64'
  | 'mips'
  | 'mips64'
  | 'powerpc'
  | 'powerpc64'
  | 'riscv64'
  | 's390x'
  | 'sparc64'

type WailsOS = 'windows' | 'darwin' | 'linux' | 'ios' | 'android'

interface WailsEnvironment {
  OS?: string
  Arch?: string
}

type EnvironmentInfo = Awaited<ReturnType<typeof System.Environment>>

/** `window._wails.environment` — injected by the Go runtime, typed here. */
function injectedEnvironment(): WailsEnvironment | undefined {
  return (window as Window & { _wails?: { environment?: WailsEnvironment } })
    ._wails?.environment
}

/** runtime.GOOS when injected; UA heuristic otherwise (browser preview). */
function currentOS(): WailsOS {
  const injected = injectedEnvironment()?.OS
  if (injected) return injected as WailsOS
  const ua = navigator.userAgent
  if (/Mac|iPhone|iPad|iPod/i.test(ua)) return 'darwin'
  if (/Linux|X11/i.test(ua)) return 'linux'
  if (/Android/i.test(ua)) return 'android'
  if (/iPhone|iPad|iPod/i.test(ua)) return 'ios'
  return 'windows'
}

/**
 * Wails reports Windows as `"<releaseId> (Build: <build>)"`; Tauri's
 * plugin-os returns `"10.0.<build>"`, which `parseWindowsBuild()` in
 * TerminalView relies on (last dot-separated segment = build number, used
 * for the xterm ConPTY reflow heuristics).
 */
function normaliseVersion(info: EnvironmentInfo): string {
  const raw = info.OSInfo?.Version ?? ''
  if (info.OS !== 'windows') return raw
  const build = /Build:\s*(\d+)/i.exec(raw)?.[1]
  return build ? `10.0.${build}` : raw
}

// --- version cache --------------------------------------------------------
// version() must stay synchronous, so the async environment query runs once
// at module load (retried on access if the runtime was not ready yet).

let osVersion: string | null = null
let environmentRequest: Promise<void> | null = null

function loadEnvironment(): void {
  if (environmentRequest) return
  try {
    environmentRequest = System.Environment()
      .then(info => {
        osVersion = normaliseVersion(info)
      })
      .catch(() => {
        // Runtime not ready yet — allow a retry on the next version() call.
        environmentRequest = null
      })
  } catch {
    environmentRequest = null
  }
}

// Warm the cache eagerly: TerminalView parses the Windows build number when
// it mounts, long after this module loads.
loadEnvironment()

// --- getters ---------------------------------------------------------------

export function platform(): Platform {
  const os = currentOS()
  if (os === 'darwin') return 'macos'
  if (os === 'ios') return 'ios'
  if (os === 'android') return 'android'
  if (os === 'linux') return 'linux'
  return 'windows'
}

export function family(): Family {
  return currentOS() === 'windows' ? 'windows' : 'unix'
}

export function type(): OsType {
  const os = currentOS()
  if (os === 'darwin') return 'macos'
  if (os === 'ios' || os === 'android') return os
  if (os === 'windows') return 'windows'
  return 'linux'
}

export function version(): string {
  if (osVersion === null) loadEnvironment()
  return osVersion ?? ''
}

/** runtime.GOARCH (amd64/arm64/…) mapped to the plugin's Arch names. */
const ARCH_NAMES: Record<string, Arch> = {
  amd64: 'x86_64',
  arm64: 'aarch64',
  '386': 'x86',
  arm: 'arm',
  ppc64: 'powerpc64',
  ppc64le: 'powerpc64',
  riscv64: 'riscv64',
  s390x: 's390x',
}

export function arch(): Arch {
  const injected = injectedEnvironment()?.Arch
  if (injected && ARCH_NAMES[injected]) return ARCH_NAMES[injected]
  if (injected) return injected as Arch
  // Browser preview: no injection, mirror the previous UA heuristic.
  return navigator.userAgent.includes('ARM') ? 'aarch64' : 'x86_64'
}

export function eol(): '\r\n' | '\n' {
  return platform() === 'windows' ? '\r\n' : '\n'
}

export function exeExtension(): 'exe' | '' {
  return platform() === 'windows' ? 'exe' : ''
}

export async function locale(): Promise<string | null> {
  // The webview's navigator.language reflects the system locale closely
  // enough for language auto-detect; Wails exposes no sync locale.
  return navigator.language ?? null
}

export async function hostname(): Promise<string | null> {
  // No OS hostname is exposed to the webview; the page host is the closest
  // available value (unused by the app today — kept for API parity).
  return window.location.hostname || null
}
