import '@testing-library/jest-dom'
import { vi } from 'vitest'

// Node 22 can expose an incomplete global localStorage when Vitest workers
// start without a valid --localstorage-file. Keep tests independent of that
// runtime detail while preserving jsdom's implementation when it is usable.
const storage = globalThis.localStorage
if (
  typeof storage?.getItem !== 'function' ||
  typeof storage?.setItem !== 'function' ||
  typeof storage?.clear !== 'function'
) {
  const values = new Map<string, string>()
  const fallback: Storage = {
    get length() {
      return values.size
    },
    clear() {
      values.clear()
    },
    getItem(key) {
      return values.get(String(key)) ?? null
    },
    key(index) {
      return Array.from(values.keys())[index] ?? null
    },
    removeItem(key) {
      values.delete(String(key))
    },
    setItem(key, value) {
      values.set(String(key), String(value))
    },
  }

  Object.defineProperty(window, 'localStorage', {
    configurable: true,
    value: fallback,
  })
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: fallback,
  })
}

// Mock matchMedia for tests
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation(query => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(), // deprecated
    removeListener: vi.fn(), // deprecated
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})

// jsdom does not implement ResizeObserver, but Radix UI components (e.g.
// AlertDialog) construct one while sizing content. Provide a minimal stub so
// tests never depend on worker scheduling luck — a single test file used to
// stub this global, which only helped files sharing its worker.
if (!('ResizeObserver' in globalThis)) {
  class ResizeObserverStub {
    observe = vi.fn()
    unobserve = vi.fn()
    disconnect = vi.fn()
  }
  Object.defineProperty(globalThis, 'ResizeObserver', {
    configurable: true,
    writable: true,
    value: ResizeObserverStub,
  })
}

// Radix UI components rely on Pointer Events APIs that jsdom doesn't fully implement.
// Provide minimal shims to prevent runtime errors in tests.
if (!('hasPointerCapture' in Element.prototype)) {
  ;(
    Element.prototype as unknown as {
      hasPointerCapture: () => boolean
    }
  ).hasPointerCapture = () => false
}
if (!('setPointerCapture' in Element.prototype)) {
  ;(
    Element.prototype as unknown as {
      setPointerCapture: () => void
    }
  ).setPointerCapture = () => undefined
}
if (!('releasePointerCapture' in Element.prototype)) {
  ;(
    Element.prototype as unknown as {
      releasePointerCapture: () => void
    }
  ).releasePointerCapture = () => undefined
}

if (!('scrollIntoView' in Element.prototype)) {
  ;(
    Element.prototype as unknown as {
      scrollIntoView: () => void
    }
  ).scrollIntoView = () => undefined
}

// The Wails runtime package performs real IPC (fetch against the jsdom
// origin, http://localhost:3000) as soon as a method is called — in tests
// every call must be inert, otherwise each one lands as an unhandled
// ECONNREFUSED. Sub-modules with their own behaviour (shell/window,
// shell/events, shell/os, …) are mocked below with richer fixtures.
vi.mock('@wailsio/runtime', () => {
  const noop = async () => undefined
  const noopUnlisten = () => undefined
  const windowMock = {
    Show: noop,
    Hide: noop,
    Close: noop,
    Focus: noop,
    Center: noop,
    SetTitle: noop,
    SetSize: noop,
    SetBackgroundColour: noop,
    Minimise: noop,
    Maximise: noop,
    UnMaximise: noop,
    ToggleMaximise: noop,
    IsMaximised: async () => false,
    IsFocused: async () => false,
    Fullscreen: noop,
    UnFullscreen: noop,
    IsFullscreen: async () => false,
  }
  return {
    Window: windowMock,
    Events: { On: () => noopUnlisten, Once: () => noopUnlisten, Emit: noop },
    Dialogs: { OpenFile: async () => '', SaveFile: async () => '' },
    Clipboard: { SetText: noop, Text: async () => '' },
    System: {
      Environment: async () => ({
        OS: 'windows',
        Arch: 'x86_64',
        OSInfo: { Version: '10.0.19045 (Build: 19045)' },
      }),
    },
    Application: {},
    Browser: {},
    Screens: { Primary: async () => ({}) },
    Flags: { All: async () => ({}) },
  }
})

// Mock the shell bridges for tests
vi.mock('@/lib/shell/events', () => ({
  listen: vi.fn().mockResolvedValue(() => {
    // Mock unlisten function
  }),
}))

vi.mock('@/lib/shell/window', () => ({
  getCurrentWindow: vi.fn().mockReturnValue({
    onFocusChanged: vi.fn().mockResolvedValue(() => undefined),
    onCloseRequested: vi.fn().mockResolvedValue(() => undefined),
    onResized: vi.fn().mockResolvedValue(() => undefined),
    show: vi.fn().mockResolvedValue(undefined),
    setFocus: vi.fn().mockResolvedValue(undefined),
    isMaximized: vi.fn().mockResolvedValue(false),
    maximize: vi.fn().mockResolvedValue(undefined),
    unmaximize: vi.fn().mockResolvedValue(undefined),
  }),
}))

vi.mock('@/lib/shell/commands', () => ({
  invoke: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/lib/shell/os', () => ({
  locale: vi.fn().mockResolvedValue('en-US'),
  platform: vi.fn().mockResolvedValue('macos'),
}))

vi.mock('@/lib/shell/fonts', () => ({
  getSystemFonts: vi.fn().mockResolvedValue([]),
}))

vi.mock('@/lib/shell/opener', () => ({
  openUrl: vi.fn().mockResolvedValue(undefined),
  openPath: vi.fn().mockResolvedValue(undefined),
  revealItemInDir: vi.fn().mockResolvedValue(undefined),
}))

// Mock the typed command facade
vi.mock('@/lib/app-bindings', () => ({
  commands: {
    greet: vi.fn().mockResolvedValue('Hello, test!'),
    getAppVersion: vi.fn().mockResolvedValue('0.5.3'),
    loadPreferences: vi
      .fn()
      .mockResolvedValue({ status: 'ok', data: { theme: 'system' } }),
    savePreferences: vi.fn().mockResolvedValue({ status: 'ok', data: null }),
    sendNativeNotification: vi
      .fn()
      .mockResolvedValue({ status: 'ok', data: null }),
    saveEmergencyData: vi.fn().mockResolvedValue({ status: 'ok', data: null }),
    loadEmergencyData: vi.fn().mockResolvedValue({ status: 'ok', data: null }),
    cleanupOldRecoveryFiles: vi
      .fn()
      .mockResolvedValue({ status: 'ok', data: 0 }),
    checkLegacyConfig: vi.fn().mockResolvedValue({ status: 'ok', data: false }),
    deleteLegacyConfig: vi.fn().mockResolvedValue({ status: 'ok', data: null }),
    listDroidSettingsFiles: vi.fn().mockResolvedValue({
      status: 'ok',
      data: [
        {
          name: 'Global',
          path: '/home/user/.factory/settings.json',
          isGlobal: true,
          isActive: true,
          exists: true,
          isExternal: false,
        },
      ],
    }),
    getActiveDroidSettingsFile: vi.fn().mockResolvedValue({
      status: 'ok',
      data: {
        name: 'Global',
        path: '/home/user/.factory/settings.json',
        isGlobal: true,
        isActive: true,
        exists: true,
        isExternal: false,
      },
    }),
    setActiveDroidSettingsFile: vi
      .fn()
      .mockResolvedValue({ status: 'ok', data: null }),
    createDroidSettingsFile: vi
      .fn()
      .mockResolvedValue({ status: 'ok', data: null }),
    deleteDroidSettingsFile: vi
      .fn()
      .mockResolvedValue({ status: 'ok', data: null }),
    linkDroidSettingsFile: vi.fn().mockResolvedValue({
      status: 'ok',
      data: {
        name: 'team-settings',
        path: '/home/user/configs/team-settings.json',
        isGlobal: false,
        isActive: true,
        exists: true,
        isExternal: true,
      },
    }),
    unlinkDroidSettingsFile: vi
      .fn()
      .mockResolvedValue({ status: 'ok', data: null }),
    getDroidLaunchCommand: vi
      .fn()
      .mockResolvedValue({ status: 'ok', data: null }),
    launchDroid: vi.fn().mockResolvedValue({ status: 'ok', data: null }),
    listDroidTrustedFolders: vi
      .fn()
      .mockResolvedValue({ status: 'ok', data: [] }),
    addDroidTrustedFolder: vi.fn().mockResolvedValue({
      status: 'ok',
      data: {
        path: '/home/user/projects',
        trustedAt: '2026-01-01T00:00:00.000Z',
      },
    }),
    removeDroidTrustedFolder: vi
      .fn()
      .mockResolvedValue({ status: 'ok', data: null }),
    removeDroidTrustedFolders: vi
      .fn()
      .mockResolvedValue({ status: 'ok', data: null }),
  },
  unwrapResult: vi.fn((result: { status: string; data?: unknown }) => {
    if (result.status === 'ok') return result.data
    throw result
  }),
}))
