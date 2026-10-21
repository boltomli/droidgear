// jest-dom 7.0.1 augments vitest's Assertion with a single type parameter
// (Assertion<T>), but vitest 5 changed the interface to two (<R, T>), so the
// upstream module merge silently fails and every expect(...).toBeInTheDocument()
// loses its matcher types. Re-declare the augmentation with vitest 5's
// signature until jest-dom ships a compatible release.
import 'vitest'
import type { TestingLibraryMatchers } from '@testing-library/jest-dom/types/matchers'

declare module 'vitest' {
  interface Assertion<
    R extends void | Promise<void> = void,
    T = unknown,
  > extends TestingLibraryMatchers<any, R> {}
}
