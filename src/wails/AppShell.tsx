import { useEffect, useState } from 'react'
import { Window as CurrentWindow } from '@wailsio/runtime'
import { ThemeProvider } from '@/components/ThemeProvider'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { commands } from '@/lib/tauri-bindings'
import { initializeLanguage } from '@/i18n/language-init'
import { SideNav } from './components/SideNav'
import { DEFAULT_SECTION, SECTIONS, type SectionID } from './sections'

/**
 * Minimal shell layout: sidebar navigation (see components/SideNav) plus the
 * active tool page. Cross-cutting startup behaviour lives here: language
 * initialisation, window fit and the version footer data.
 */
function AppShell() {
  const [section, setSection] = useState<SectionID>(DEFAULT_SECTION)
  const [appVersion, setAppVersion] = useState<string | null>(null)

  useEffect(() => {
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

    // Version string for the sidebar footer (bare command, not Result-wrapped).
    void commands
      .getAppVersion()
      .then(version => setAppVersion(version))
      .catch(() => setAppVersion(null))
  }, [])

  const ActivePage = SECTIONS.find(entry => entry.id === section)?.page

  return (
    <div className="flex h-full w-full overflow-hidden bg-background text-foreground">
      <SideNav active={section} onSelect={setSection} version={appVersion} />
      <main className="flex-1 overflow-y-auto">
        {ActivePage && <ActivePage />}
      </main>
      <Toaster position="bottom-right" expand={false} />
    </div>
  )
}

export default function MinimalApp() {
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
