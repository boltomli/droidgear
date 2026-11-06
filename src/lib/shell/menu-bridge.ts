/**
 * Native menu bridge — the menu is built in Go
 * (internal/app/menu.go). Two responsibilities:
 *
 * 1. Ask Go to (re)build the menu whenever the language is known/changes,
 *    passing the current i18n language (the Go side keeps its own en/zh
 *    label tables, mirroring the `menu.*` locale keys).
 * 2. Translate the `menu:*` events emitted by Go into the store actions.
 *
 * "Check for Updates" runs the same service call as Preferences → General
 * (internal/service/updater) and reports the answer as a toast — the pane is
 * where the download/install progress lives.
 */

import { toast } from 'sonner'
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
 * Native "Check for Updates..." — a one-shot check that never opens a second
 * window: up to date / dev build are toasts, an available release offers the
 * Preferences pane where download and install happen.
 */
async function checkForUpdatesFromMenu(): Promise<void> {
  try {
    const result = await commands.checkForUpdates()
    if (result.status === 'error') throw new Error(result.error)
    const update = result.data
    if (update.devBuild) {
      toast.info(i18n.t('updates.devBuild'))
      return
    }
    if (!update.updateAvailable) {
      toast.success(i18n.t('preferences.general.upToDate'))
      return
    }
    toast.info(
      i18n.t('preferences.general.updateAvailable', {
        version: update.latestVersion,
      }),
      {
        action: {
          label: i18n.t('updates.banner.action'),
          onClick: () => useUIStore.getState().setPreferencesOpen(true),
        },
      }
    )
  } catch (error) {
    logger.warn('Update check from the menu failed', { error })
    toast.error(i18n.t('preferences.general.updateFailed'), {
      description: String(error),
    })
  }
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
    'menu:check-for-updates': () => {
      void checkForUpdatesFromMenu()
    },
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
