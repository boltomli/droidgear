import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Bot,
  Boxes,
  Globe,
  Package,
  Terminal,
  type LucideIcon,
} from 'lucide-react'
import { ThemeProvider } from '@/components/ThemeProvider'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { commands } from '@/lib/tauri-bindings'
import { initializeLanguage } from '@/i18n/language-init'
import { DshConfigPage } from '@/components/dsh/DshConfigPage'
import { PiConfigPage } from '@/components/pi/PiConfigPage'
import { HermesConfigPage } from '@/components/hermes/HermesConfigPage'
import { OmpConfigPage } from '@/components/omp/OmpConfigPage'

type SectionID = 'dsh-desktop' | 'dsh-web' | 'pi' | 'hermes' | 'omp'

interface NavItem {
  id: SectionID
  label: string
  icon: LucideIcon
}

const NAV_ITEMS: NavItem[] = [
  { id: 'dsh-desktop', label: 'Dsh (desktop)', icon: Terminal },
  { id: 'dsh-web', label: 'Dsh (web)', icon: Globe },
  { id: 'pi', label: 'Pi', icon: Bot },
  { id: 'hermes', label: 'Hermes', icon: Boxes },
  { id: 'omp', label: 'OMP', icon: Package },
]

function NavRow({
  item,
  active,
  onClick,
}: {
  item: NavItem
  active: boolean
  onClick: () => void
}) {
  const Icon = item.icon
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'relative flex h-9 w-full items-center gap-2.5 rounded-md px-3',
        'text-left text-sm text-muted-foreground outline-none transition-colors',
        'hover:bg-accent hover:text-accent-foreground',
        'focus-visible:ring-2 focus-visible:ring-ring/50',
        active && 'bg-accent font-medium text-foreground'
      )}
    >
      {active && (
        <span
          aria-hidden
          className="absolute left-0 top-1/2 h-4 w-1 -translate-y-1/2 rounded-full bg-primary"
        />
      )}
      <Icon aria-hidden className="size-4 shrink-0" />
      <span className="truncate">{item.label}</span>
    </button>
  )
}

function AppShell() {
  const { t } = useTranslation()
  const [section, setSection] = useState<SectionID>('dsh-desktop')

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
  }, [])

  return (
    <div className="flex h-full w-full overflow-hidden bg-background text-foreground">
      <aside className="flex w-60 shrink-0 flex-col border-r border-border bg-muted/30">
        <div className="flex items-baseline gap-1.5 px-4 pb-3 pt-4">
          <span className="text-base font-semibold tracking-tight">
            {t('appName', 'DroidGear')}
          </span>
          <span className="text-[11px] text-muted-foreground">minimal</span>
        </div>

        <div role="separator" className="mx-3 border-t border-border" />

        <nav className="flex-1 overflow-y-auto px-2 py-3" aria-label="Tools">
          <p className="px-2 pb-1.5 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
            {t('wailsNav.tools', 'Tools')}
          </p>
          <ul className="flex flex-col gap-0.5">
            {NAV_ITEMS.map(item => (
              <li key={item.id}>
                <NavRow
                  item={item}
                  active={section === item.id}
                  onClick={() => setSection(item.id)}
                />
              </li>
            ))}
          </ul>
        </nav>

        <div role="separator" className="mx-3 border-t border-border" />
        <div className="px-4 py-3 text-[11px] leading-relaxed text-muted-foreground">
          Wails v3 spike — dsh / pi / hermes / omp
        </div>
      </aside>

      <main className="flex-1 overflow-y-auto">
        {section === 'dsh-desktop' && <DshConfigPage profile="desktop" />}
        {section === 'dsh-web' && <DshConfigPage profile="web" />}
        {section === 'pi' && <PiConfigPage />}
        {section === 'hermes' && <HermesConfigPage />}
        {section === 'omp' && <OmpConfigPage />}
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
