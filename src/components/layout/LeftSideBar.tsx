import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Server,
  Bot,
  Terminal,
  Code,
  Feather,
  ChevronDown,
  Check,
  Cog,
  Cpu,
  CircuitBoard,
  BotMessageSquare,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { ActionButton } from '@/components/ui/action-button'
import { ActionDropdownMenuItem } from '@/components/ui/action-dropdown-menu-item'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
// Import directly from individual files (not barrel indexes) so that page
// components re-exported by those barrels don't end up in the main chunk.
// MainWindowContent uses React.lazy() for page components and needs clean
// dynamic import boundaries.
import { ChannelList } from '@/components/channels/ChannelList'
import { ChannelDialog } from '@/components/channels/ChannelDialog'
import { DroidFeatureList } from '@/components/droid/DroidFeatureList'
import { OpenCodeFeatureList } from '@/components/opencode/OpenCodeFeatureList'
import { CodexFeatureList } from '@/components/codex/CodexFeatureList'
import { ClaudeFeatureList } from '@/components/claude/ClaudeFeatureList'
import { OpenClawFeatureList } from '@/components/openclaw/OpenClawFeatureList'
import { HermesFeatureList } from '@/components/hermes/HermesFeatureList'
import { PiFeatureList } from '@/components/pi/PiFeatureList'
import { OmpFeatureList } from '@/components/omp/OmpFeatureList'
import { CopilotFeatureList } from '@/components/copilot/CopilotFeatureList'
import { DshFeatureList } from '@/components/dsh/DshFeatureList'
import {
  useUIStore,
  type NavigationView,
  type ToolView,
} from '@/store/ui-store'
import { useChannelStore } from '@/store/channel-store'
import { useModelStore } from '@/store/model-store'
import type { Channel } from '@/lib/bindings'
import { saveChannelAuth } from '@/lib/channel-utils'

interface ToolModule {
  id: ToolView
  labelKey: string
  icon: React.ElementType
  /** Legacy module kept for compatibility — rendered last, behind a divider. */
  deprecated?: boolean
}

const DROID_MODULE: ToolModule = {
  id: 'droid',
  labelKey: 'sidebar.droid',
  icon: Bot,
}

// Batches rather than a flat list: the first batch is what the tool switcher
// leads with, the second follows, and deprecated modules sit at the very
// bottom. Order inside a batch is alphabetical. This is the GUI half of the
// module order — `wails/internal/tui/screen.go` DefaultModules must match it,
// pinned by wails/internal/tui/tui_test.go::TestDefaultModulesMatchParityMatrix.
const TOOL_BATCHES: ToolModule[][] = [
  [
    { id: 'codex', labelKey: 'sidebar.codex', icon: Code },
    DROID_MODULE,
    { id: 'hermes', labelKey: 'sidebar.hermes', icon: Cpu },
    { id: 'omp', labelKey: 'sidebar.omp', icon: CircuitBoard },
    { id: 'pi', labelKey: 'sidebar.pi', icon: CircuitBoard },
  ],
  [
    { id: 'claude', labelKey: 'sidebar.claude', icon: Feather },
    { id: 'copilot', labelKey: 'sidebar.copilot', icon: BotMessageSquare },
    { id: 'dsh', labelKey: 'sidebar.dsh', icon: CircuitBoard },
  ],
  [
    {
      id: 'openclaw',
      labelKey: 'sidebar.openclaw',
      icon: Cog,
      deprecated: true,
    },
    {
      id: 'opencode',
      labelKey: 'sidebar.opencode',
      icon: Terminal,
      deprecated: true,
    },
  ],
]

const TOOL_MODULES = TOOL_BATCHES.flat()

interface LeftSideBarProps {
  children?: React.ReactNode
  className?: string
}

export function LeftSideBar({ children, className }: LeftSideBarProps) {
  const { t } = useTranslation()
  const currentView = useUIStore(state => state.currentView)
  const setCurrentView = useUIStore(state => state.setCurrentView)
  const addChannel = useChannelStore(state => state.addChannel)
  const saveChannels = useChannelStore(state => state.saveChannels)
  const channels = useChannelStore(state => state.channels)
  const selectChannel = useChannelStore(state => state.selectChannel)
  const channelHasChanges = useChannelStore(state => state.hasChanges)
  const modelHasChanges = useModelStore(state => state.hasChanges)

  const [channelDialogOpen, setChannelDialogOpen] = useState(false)
  const [editingChannel, setEditingChannel] = useState<Channel | undefined>()
  const [pendingView, setPendingView] = useState<NavigationView | null>(null)
  const [dropdownOpen, setDropdownOpen] = useState(false)

  // Get lastToolView from store (automatically updated when switching to droid/opencode)
  const lastToolView = useUIStore(state => state.lastToolView)

  // Shared content for tool button
  const activeModule =
    TOOL_MODULES.find(module => module.id === lastToolView) ?? DROID_MODULE
  const toolButtonContent = (
    <>
      <activeModule.icon className="h-4 w-4 mr-2" />
      {t(activeModule.labelKey)}
      <ChevronDown className="h-3 w-3 ml-1" />
    </>
  )

  const handleViewChange = (view: NavigationView) => {
    if (view === currentView) return
    // Check if current view has unsaved changes
    const hasUnsavedChanges =
      (currentView === 'droid' && modelHasChanges) ||
      (currentView === 'channels' && channelHasChanges)

    if (hasUnsavedChanges) {
      setPendingView(view)
    } else {
      setCurrentView(view)
    }
  }

  const handleSaveAndSwitch = async () => {
    if (currentView === 'droid') {
      await useModelStore.getState().saveModels()
    } else if (currentView === 'channels') {
      await useChannelStore.getState().saveChannels()
    }
    if (pendingView) {
      setCurrentView(pendingView)
      setPendingView(null)
    }
  }

  const handleDiscardAndSwitch = () => {
    if (currentView === 'droid') {
      useModelStore.getState().resetChanges()
    } else if (currentView === 'channels') {
      useChannelStore.getState().resetChanges()
    }
    if (pendingView) {
      setCurrentView(pendingView)
      setPendingView(null)
    }
  }

  const handleAddChannel = () => {
    setEditingChannel(undefined)
    setChannelDialogOpen(true)
  }

  const handleSaveChannel = async (
    channel: Channel,
    username: string,
    password: string
  ) => {
    const authResult = await saveChannelAuth(
      channel.id,
      channel.type,
      username,
      password
    )
    if (!authResult.ok) {
      console.error('Failed to save channel auth:', authResult.error)
      return
    }

    // Check if this is an edit or new channel
    const existingIndex = channels.findIndex(c => c.id === channel.id)
    if (existingIndex >= 0) {
      useChannelStore.getState().updateChannel(channel.id, channel)
    } else {
      addChannel(channel)
    }

    await saveChannels()
    selectChannel(channel.id)
    setCurrentView('channels')
  }

  return (
    <div
      className={cn('flex h-full flex-col border-r bg-background', className)}
    >
      {/* Navigation Tabs */}
      <div className="flex items-center gap-1 p-2 border-b">
        <ActionButton
          variant={currentView === 'channels' ? 'secondary' : 'ghost'}
          size="sm"
          className="flex-1"
          onClick={() => handleViewChange('channels')}
        >
          <Server className="h-4 w-4 mr-2" />
          {t('sidebar.channels')}
        </ActionButton>

        {/* Simple button when in Channels - direct switch to lastToolView */}
        {currentView === 'channels' ? (
          // Simple button when in Channels - direct switch to lastToolView
          <ActionButton
            variant="ghost"
            size="sm"
            className="flex-1"
            onClick={() => handleViewChange(lastToolView)}
          >
            {toolButtonContent}
          </ActionButton>
        ) : (
          // Dropdown menu when in Droid/OpenCode - allow switching between tools
          <DropdownMenu open={dropdownOpen} onOpenChange={setDropdownOpen}>
            <DropdownMenuTrigger asChild>
              <ActionButton variant="secondary" size="sm" className="flex-1">
                {toolButtonContent}
              </ActionButton>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              {TOOL_BATCHES.map((batch, batchIndex) => (
                <Fragment key={batch.map(module => module.id).join('-')}>
                  {batchIndex > 0 && <DropdownMenuSeparator />}
                  {batch.map(module => (
                    <ActionDropdownMenuItem
                      key={module.id}
                      onClick={() => {
                        handleViewChange(module.id)
                        setDropdownOpen(false)
                      }}
                    >
                      <module.icon className="h-4 w-4 mr-2" />
                      {t(module.labelKey)}
                      {module.deprecated && (
                        <Badge
                          variant="outline"
                          className="ml-2 px-1.5 py-0 text-[10px] font-normal text-muted-foreground"
                        >
                          {t('sidebar.deprecated')}
                        </Badge>
                      )}
                      {lastToolView === module.id && (
                        <Check className="h-4 w-4 ml-auto" />
                      )}
                    </ActionDropdownMenuItem>
                  ))}
                </Fragment>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>

      {/* Content based on view */}
      <div className="flex-1 overflow-hidden">
        {currentView === 'channels' ? (
          <ChannelList onAddChannel={handleAddChannel} />
        ) : currentView === 'droid' ? (
          <DroidFeatureList />
        ) : currentView === 'opencode' ? (
          <OpenCodeFeatureList />
        ) : currentView === 'claude' ? (
          <ClaudeFeatureList />
        ) : currentView === 'openclaw' ? (
          <OpenClawFeatureList />
        ) : currentView === 'hermes' ? (
          <HermesFeatureList />
        ) : currentView === 'pi' ? (
          <PiFeatureList />
        ) : currentView === 'omp' ? (
          <OmpFeatureList />
        ) : currentView === 'copilot' ? (
          <CopilotFeatureList />
        ) : currentView === 'dsh' ? (
          <DshFeatureList />
        ) : (
          <CodexFeatureList />
        )}
      </div>

      {children}

      {/* Channel Dialog */}
      <ChannelDialog
        open={channelDialogOpen}
        onOpenChange={setChannelDialogOpen}
        channel={editingChannel}
        onSave={handleSaveChannel}
      />

      {/* Unsaved Changes Confirmation Dialog */}
      <AlertDialog
        open={pendingView !== null}
        onOpenChange={open => !open && setPendingView(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('sidebar.unsavedChanges.title')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('sidebar.unsavedChanges.description')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <Button variant="destructive" onClick={handleDiscardAndSwitch}>
              {t('sidebar.unsavedChanges.discard')}
            </Button>
            <Button onClick={handleSaveAndSwitch}>
              {t('sidebar.unsavedChanges.save')}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
