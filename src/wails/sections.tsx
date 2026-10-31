import type { ComponentType } from 'react'
import {
  Bot,
  Boxes,
  Globe,
  Package,
  Terminal,
  type LucideIcon,
} from 'lucide-react'
import { DshConfigPage } from '@/components/dsh/DshConfigPage'
import { PiConfigPage } from '@/components/pi/PiConfigPage'
import { HermesConfigPage } from '@/components/hermes/HermesConfigPage'
import { OmpConfigPage } from '@/components/omp/OmpConfigPage'

/**
 * Section registry for the minimal shell.
 *
 * Each section pairs a sidebar entry (label + icon) with the page component
 * it renders. Adding a tool to the shell is one entry here plus the Go
 * command surface — no changes needed in SideNav/AppShell.
 */
export type SectionID = 'dsh-desktop' | 'dsh-web' | 'pi' | 'hermes' | 'omp'

export interface Section {
  id: SectionID
  label: string
  icon: LucideIcon
  page: ComponentType
}

/** DshConfigPage needs an explicit runtime profile prop. */
function DshDesktopPage() {
  return <DshConfigPage profile="desktop" />
}

function DshWebPage() {
  return <DshConfigPage profile="web" />
}

export const SECTIONS: Section[] = [
  {
    id: 'dsh-desktop',
    label: 'Dsh (desktop)',
    icon: Terminal,
    page: DshDesktopPage,
  },
  { id: 'dsh-web', label: 'Dsh (web)', icon: Globe, page: DshWebPage },
  { id: 'pi', label: 'Pi', icon: Bot, page: PiConfigPage },
  { id: 'hermes', label: 'Hermes', icon: Boxes, page: HermesConfigPage },
  { id: 'omp', label: 'OMP', icon: Package, page: OmpConfigPage },
]

export const DEFAULT_SECTION: SectionID = 'dsh-desktop'
