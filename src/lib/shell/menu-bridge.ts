/**
 * Native menu bridge — the menu is built in Go
 * (internal/app/menu.go). Two responsibilities:
 *
 * 1. Ask Go to (re)build the menu whenever the language is known/changes,
 *    passing the current i18n language (the Go side keeps its own en/zh
 *    label tables, mirroring the `menu.*` locale keys).
 * 2. Translate the `menu:*` events emitted by Go into the store actions.
 *
 * The updater entry ("Check for Updates") is intentionally not present: the
 * app ships no update feed (see docs/developer/tauri-removal-design.zh.md).
 */

import { listen } from './events'
import { invoke } from './commands'
import i18n from '@/i18n/config'
import { commands } from '@/lib/app-bindings'
import { useUIStore } from '@/store/ui-store'
import { logger } from '@/lib/logger'

function rebuildMenu(): void {
  void invoke('build_app_menu', { lang: i18n.language }).catch(error => {
    logger.warn('Failed to build native menu', { error })
  })
}

/**
 * Installs the menu bridge. Returns a cleanup function (mirrors
 * `setupMenuLanguageListener` in src/lib/menu.ts).
 */
export function initWailsMenu(): () => void {
  const cleanups: (() => void)[] = []

  rebuildMenu()
  i18n.on('languageChanged', rebuildMenu)
  cleanups.push(() => i18n.off('languageChanged', rebuildMenu))

  const handlers: Record<string, () => void> = {
    'menu:open-preferences': () =>
      useUIStore.getState().setPreferencesOpen(true),
    'menu:toggle-left-sidebar': () => useUIStore.getState().toggleLeftSidebar(),
    'menu:toggle-right-sidebar': () =>
      useUIStore.getState().toggleRightSidebar(),
    'menu:reset-window': () => {
      void commands.resetWindowState().catch(error => {
        logger.error('Reset window failed', { error })
      })
    },
  }

  for (const [event, handler] of Object.entries(handlers)) {
    void listen(event, handler).then(unlisten => cleanups.push(unlisten))
  }

  return () => {
    for (const cleanup of cleanups) cleanup()
  }
}
