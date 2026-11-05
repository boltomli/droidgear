# Testing

Testing patterns for TypeScript (Vitest) and Go (`go test`).

## Running Tests

```bash
npm run check:all      # All frontend checks (typecheck, lint, tests, build)
npm run test           # TypeScript tests (watch mode)
npm run test:run       # TypeScript tests (single run)
cd wails && go vet ./... && go test ./...   # Go service + CLI tests
```

## TypeScript Testing

Uses **Vitest** + **@testing-library/react**. Configuration in `vitest.config.ts`.

### Test File Location

Place tests next to the code they test:

```
src/components/ui/Button.tsx
src/components/ui/Button.test.tsx
```

### Mocking the command bridge (Critical)

Backend calls must be mocked since tests run outside the webview. Mocks are
configured in `src/test/setup.ts`:

```typescript
// src/test/setup.ts
import { vi } from 'vitest'

// Mock the Wails runtime event bus
vi.mock('@wailsio/runtime', () => ({
  /* Events.On / Events.Once … */
}))

// Mock the shell modules (events, window, os, fonts, opener, commands)
vi.mock('@/lib/shell/commands', () => ({ invoke: vi.fn() }))

// Mock the typed command facade
vi.mock('@/lib/app-bindings', () => ({
  commands: {
    greet: vi.fn().mockResolvedValue('Hello, test!'),
    loadPreferences: vi
      .fn()
      .mockResolvedValue({ status: 'ok', data: { theme: 'system' } }),
    savePreferences: vi.fn().mockResolvedValue({ status: 'ok', data: null }),
    sendNativeNotification: vi
      .fn()
      .mockResolvedValue({ status: 'ok', data: null }),
  },
}))
```

### Testing with Mocked Commands

```typescript
import { vi } from 'vitest'
import { commands } from '@/lib/app-bindings'

const mockCommands = vi.mocked(commands)

test('loads preferences', async () => {
  mockCommands.loadPreferences.mockResolvedValue({
    status: 'ok',
    data: { theme: 'dark' },
  })

  // Test code that calls loadPreferences
})
```

### Test Wrapper for Providers

Components using TanStack Query need a provider wrapper:

```typescript
// src/test/utils.ts
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ReactNode } from 'react'

export function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
}

export function TestProviders({ children }: { children: ReactNode }) {
  const queryClient = createTestQueryClient()
  return (
    <QueryClientProvider client={queryClient}>
      {children}
    </QueryClientProvider>
  )
}
```

Usage:

```typescript
import { render } from '@testing-library/react'
import { TestProviders } from '@/test/utils'

test('component with query', () => {
  render(
    <TestProviders>
      <MyComponent />
    </TestProviders>
  )
})
```

### Testing Zustand Stores

```typescript
import { renderHook, act } from '@testing-library/react'
import { useUIStore } from '@/store/ui-store'

test('toggles sidebar visibility', () => {
  const { result } = renderHook(() => useUIStore())

  expect(result.current.leftSidebarVisible).toBe(true)

  act(() => {
    result.current.setLeftSidebarVisible(false)
  })

  expect(result.current.leftSidebarVisible).toBe(false)
})
```

## Go Testing

Service and CLI tests live next to the code in `wails/internal/**` and
`wails/cmd/dg`:

```bash
cd wails && go vet ./... && go test ./...     # everything
go test ./internal/service/pi/ -v              # one package
go test -run TestApplyProfile ./internal/service/pi/   # one test
```

Use `testutil.IsolatedHome(t)` at the top of any test that resolves
`~/.droidgear`, `~/.pi`, … so it writes to a `t.TempDir()` instead of the
developer's real profile; see `wails/internal/service/*_test.go` for the
pattern (read → mutate → apply → assert round-trip).

```go
func TestApply(t *testing.T) {
    testutil.IsolatedHome(t)
    // … save a profile, apply it, read the file back, assert on the JSON
}
```

## Adding New Command Mocks

When adding new Go commands, update `src/test/setup.ts` mocks and
`src/lib/shell/commands.ts` (see `scripts/check-command-surface.mjs`):

```typescript
vi.mock('@/lib/app-bindings', () => ({
  commands: {
    // ... existing mocks
    myNewCommand: vi.fn().mockResolvedValue({ status: 'ok', data: null }),
  },
}))
```

## Best Practices

| Do                                           | Don't                         |
| -------------------------------------------- | ----------------------------- |
| Mock the command facade in setup.ts          | Call real Go methods in tests |
| Use `vi.mocked()` for type-safe mocks        | Use untyped mock assertions   |
| Test user-visible behavior                   | Test implementation details   |
| Isolate the filesystem in Go (`t.TempDir()`) | Write to the real file system |
