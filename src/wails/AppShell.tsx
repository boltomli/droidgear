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
import { commands } from '@/lib/tauri-bindings'
import { initWailsMenu } from '@/wails/menu-bridge'
import { useUIStore } from '@/store/ui-store'
import { isToolSupported, WAILS_FALLBACK_TOOL } from '@/lib/capabilities'
import { logger } from '@/lib/logger'

/**
 * Wails shell: renders the shared MainWindow (TitleBar + LeftSideBar +
 * MainWindowContent + command palette + preferences) unchanged. Cross-cutting
 * startup behaviour lives here: command system registration, language
 * initialisation, shell-env preload, recovery cleanup, window fit, and
 * redirecting away from persisted views whose Go backend does not exist yet
 * (see src/lib/capabilities.ts).
 */
function AppShell() {
  useEffect(() => {
    initializeCommandSystem()

    // Redirect persisted views (zustand/localStorage) that the Wails backend
    // does not support to the fallback tool, keeping the sidebar consistent.
    const state = useUIStore.getState()
    if (!isToolSupported(state.currentView)) {
      useUIStore.setState({
        currentView: WAILS_FALLBACK_TOOL,
        lastToolView: WAILS_FALLBACK_TOOL,
      })
    } else if (!isToolSupported(state.lastToolView)) {
      useUIStore.setState({ lastToolView: WAILS_FALLBACK_TOOL })
    }

    // Shrink the window when the default 1280×840 does not fit the current
    // work area (small screens / large display scale), then re-centre.
    // Both the runtime SetSize API and screen.avail* are in CSS pixels
    // (Wails converts DIP → physical internally).
    const margin = 48
    const targetWidth = Math.min(1280, window.screen.availWidth - margin)
    const targetHeight = Math.min(840, window.screen.availHeight - margin)
    if (window.innerWidth > targetWidth || window.innerHeight > targetHeight) {
      void CurrentWindow.SetSize(targetWidth, targetHeight).then(() =>
        CurrentWindow.Center()
      )
    }
  }, [])

  useEffect(() => {
    // Language auto-detection mirrors the main app: saved preference first,
    // then system locale. Failures fall back to English inside
    // initializeLanguage.
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
    // recovery files, matching the Tauri app's startup sequence.
    void preloadShellEnv()
    void cleanupOldFiles().catch(error => {
      logger.warn('Recovery cleanup failed', { error })
    })

    // Native menu: install after language initialisation so labels match
    // the detected locale, and re-emit on every language change.
    return initWailsMenu()
  }, [])

  return (
    <>
      <MainWindow />
      {/* Legacy ~/.droidgear/config.json migration prompt — the Go backend
          (CheckLegacyConfig / DeleteLegacyConfig) and its shim mapping have
          always existed; only the mount was missing (guidelines §3.1). */}
      <LegacyConfigDialog />
    </>
  )
}

export default function WailsApp() {
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
