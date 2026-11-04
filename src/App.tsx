import { useEffect } from 'react'
import { Window as CurrentWindow } from '@wailsio/runtime'
import { ThemeProvider } from '@/components/ThemeProvider'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { TooltipProvider } from '@/components/ui/tooltip'
import { MainWindow } from '@/components/layout/MainWindow'
import { LegacyConfigDialog } from '@/components/LegacyConfigDialog'
import { initializeCommandSystem } from '@/lib/commands'
import { initializeLanguage } from '@/i18n/language-init'
import { preloadShellEnv } from '@/services/shell-env'
import { cleanupOldFiles } from '@/lib/recovery'
import { commands } from '@/lib/app-bindings'
import { initWailsMenu } from '@/lib/shell/menu-bridge'
import { syncWindowBackground } from '@/lib/shell/window-theme'
import { logger } from '@/lib/logger'

/**
 * The application shell: renders MainWindow (TitleBar + LeftSideBar +
 * MainWindowContent + command palette + preferences) and the legacy-config
 * prompt. Cross-cutting startup behaviour lives here: command system
 * registration, language initialisation, shell-env preload, recovery cleanup,
 * window fit, and the native-menu bridge.
 */
function AppShell() {
  useEffect(() => {
    initializeCommandSystem()

    // The window is created hidden (Hidden: true in wails/main.go, mirroring
    // the original visible:false) — reveal it now that the shell has mounted,
    // and keep the native background colour following the theme (round2 §B6).
    const stopBackgroundSync = syncWindowBackground()
    void CurrentWindow.Show().then(() => CurrentWindow.Focus())

    // Shrink the window when the restored/default 1150×700 does not fit the
    // current work area (small screens / large display scale), then re-centre.
    // Both the runtime SetSize API and screen.avail* are in CSS pixels
    // (Wails converts DIP → physical internally).
    const margin = 48
    const targetWidth = Math.min(1150, window.screen.availWidth - margin)
    const targetHeight = Math.min(700, window.screen.availHeight - margin)
    if (window.innerWidth > targetWidth || window.innerHeight > targetHeight) {
      void CurrentWindow.SetSize(targetWidth, targetHeight).then(() =>
        CurrentWindow.Center()
      )
    }

    return stopBackgroundSync
  }, [])

  useEffect(() => {
    // Language auto-detection: saved preference first, then system locale.
    // Failures fall back to English inside initializeLanguage.
    const init = async () => {
      let savedLanguage: string | null = null
      try {
        const result = await commands.loadPreferences()
        if (result.status === 'ok') {
          savedLanguage = result.data.language
        }
      } catch {
        // preferences unavailable — use system locale detection
      }
      await initializeLanguage(savedLanguage)
    }
    void init()

    // Warm the shell env cache (used by the terminal) and clean up old crash
    // recovery files.
    void preloadShellEnv()
    void cleanupOldFiles().catch(error => {
      logger.warn('Recovery cleanup failed', { error })
    })

    // Native menu: install after language initialisation so labels match the
    // detected locale, and re-emit on every language change.
    return initWailsMenu()
  }, [])

  return (
    <>
      <MainWindow />
      {/* Legacy ~/.droidgear/config.json migration prompt. */}
      <LegacyConfigDialog />
    </>
  )
}

export default function App() {
  return (
    <ErrorBoundary>
      <ThemeProvider>
        <TooltipProvider>
          <AppShell />
        </TooltipProvider>
      </ThemeProvider>
    </ErrorBoundary>
  )
}
