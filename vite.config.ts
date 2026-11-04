import { defineConfig, type UserConfig, type Plugin } from 'vite'
import react, { reactCompilerPreset } from '@vitejs/plugin-react'
import babel from '@rolldown/plugin-babel'
import tailwindcss from '@tailwindcss/vite'
import fs from 'node:fs'
import path, { resolve } from 'path'
import { fileURLToPath } from 'url'
import packageJson from './package.json'

/**
 * Single frontend build for the Wails shell.
 *
 * The bundle is embedded by `wails/main.go` (`//go:embed all:frontend/dist`),
 * so `outDir` is `wails/frontend/dist`. Bindings are generated first
 * (`wails3 generate bindings -clean=true -ts -i`, gitignored) and reach the
 * source through the `@wails-bindings` alias.
 *
 * @see docs/agents/wails-port.md — build order and the Tailwind `@source` trap
 */

const dir = path.dirname(fileURLToPath(import.meta.url))

/**
 * `emptyOutDir` wipes wails/frontend/dist on every build, including the
 * tracked `.gitkeep` that keeps `//go:embed all:frontend/dist` compiling on a
 * clean clone (wails/.gitignore negation). Restore it after each build so
 * `git status` stays clean.
 */
function restoreGitkeep(): Plugin {
  return {
    name: 'restore-frontend-dist-gitkeep',
    closeBundle() {
      fs.writeFileSync(resolve(dir, 'wails/frontend/dist/.gitkeep'), '')
    },
  }
}

export default defineConfig(async () => {
  const babelPlugin = await babel({ presets: [reactCompilerPreset()] })
  return {
    root: dir,
    publicDir: resolve(dir, 'public'),
    define: {
      __APP_VERSION__: JSON.stringify(packageJson.version),
    },
    plugins: [react(), babelPlugin, tailwindcss(), restoreGitkeep()],
    resolve: {
      alias: {
        '@': resolve(dir, 'src'),
        '@wails-bindings': resolve(
          dir,
          'wails/frontend/bindings/droidgear-wails/internal/app/app.ts'
        ),
      },
    },
    server: {
      host: '127.0.0.1',
      port: Number(process.env.WAILS_VITE_PORT) || 9245,
      strictPort: true,
      fs: {
        allow: [dir],
      },
    },
    build: {
      chunkSizeWarningLimit: 600,
      cssMinify: 'lightningcss',
      outDir: resolve(dir, 'wails/frontend/dist'),
      emptyOutDir: true,
      rollupOptions: {
        input: resolve(dir, 'index.html'),
      },
    },
    clearScreen: false,
  } satisfies UserConfig
})
