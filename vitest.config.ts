import path from 'path'
import { fileURLToPath } from 'url'
import { defineConfig } from 'vitest/config'
import react, { reactCompilerPreset } from '@vitejs/plugin-react'
import babel from '@rolldown/plugin-babel'

// `__dirname` is unsupported by Vite's native config loader (planned default);
// derive it from the module URL instead.
const dir = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  plugins: [
    react(),
    babel({ presets: [reactCompilerPreset()] }),
  ],
  test: {
    globals: true,
    environment: 'jsdom',
    // Heavy dialogs (ProviderDialog with the model registry) run close to 1s
    // alone and past 5s when the whole suite saturates the machine; the
    // default 5s budget turned that load into intermittent "timed out"
    // failures rather than real assertion failures.
    testTimeout: 15000,
    env: {
      DROIDGEAR_DISABLE_UPDATE_CHECK: '',
    },
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.{test,spec}.{js,mjs,cjs,ts,mts,cts,jsx,tsx}'],
    exclude: ['node_modules', 'dist', '.git', '.cache', 'build'],
    coverage: {
      provider: 'v8',
      thresholds: {
        lines: 60,
        functions: 60,
        branches: 60,
      },
    },
  },
  resolve: {
    alias: {
      '@': path.resolve(dir, './src'),
      '@wails-bindings': path.resolve(
        dir,
        './wails/frontend/bindings/droidgear-wails/internal/app/app.ts'
      ),
    },
  },
})
