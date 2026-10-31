import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { SECTIONS, type Section, type SectionID } from '../sections'

interface SideNavProps {
  active: SectionID
  onSelect: (id: SectionID) => void
  /** App version shown in the footer (null while loading). */
  version?: string | null
}

function NavRow({
  section,
  active,
  onClick,
}: {
  section: Section
  active: boolean
  onClick: () => void
}) {
  const Icon = section.icon
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
      <span className="truncate">{section.label}</span>
    </button>
  )
}

/**
 * Left sidebar of the minimal shell: app title, tool list, version footer.
 */
export function SideNav({ active, onSelect, version }: SideNavProps) {
  const { t } = useTranslation()

  return (
    <aside className="flex w-60 shrink-0 flex-col border-r border-border bg-muted/40">
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
          {SECTIONS.map(section => (
            <li key={section.id}>
              <NavRow
                section={section}
                active={section.id === active}
                onClick={() => onSelect(section.id)}
              />
            </li>
          ))}
        </ul>
      </nav>

      <div role="separator" className="mx-3 border-t border-border" />
      <div className="flex items-center justify-between px-4 py-3 text-[11px] leading-relaxed text-muted-foreground">
        <span>Wails v3 spike</span>
        {version && <span className="tabular-nums">v{version}</span>}
      </div>
    </aside>
  )
}
