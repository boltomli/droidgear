import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Plus,
  X,
  Play,
  CheckCircle,
  Circle,
  Pencil,
  FolderOpen,
} from 'lucide-react'
import { open } from '@tauri-apps/plugin-dialog'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Input } from '@/components/ui/input'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import { cn } from '@/lib/utils'
import { TerminalView } from './TerminalView'
import { useTerminalStore } from '@/store/terminal-store'

export function TerminalPage() {
  const { t } = useTranslation()
  const terminals = useTerminalStore(state => state.terminals)
  const selectedTerminalId = useTerminalStore(state => state.selectedTerminalId)
  const createTerminal = useTerminalStore(state => state.createTerminal)
  const closeTerminal = useTerminalStore(state => state.closeTerminal)
  const renameTerminal = useTerminalStore(state => state.renameTerminal)
  const selectTerminal = useTerminalStore(state => state.selectTerminal)
  const updateTerminalStatus = useTerminalStore(
    state => state.updateTerminalStatus
  )
  const setTerminalNotification = useTerminalStore(
    state => state.setTerminalNotification
  )

  const [editingId, setEditingId] = useState<string | null>(null)
  const [editingName, setEditingName] = useState('')

  const handleCreateTerminal = () => {
    createTerminal()
  }

  const handleCreateTerminalWithDir = async () => {
    const selected = await open({
      directory: true,
      multiple: false,
      title: t('droid.terminal.selectDirectory'),
    })
    if (selected) {
      createTerminal(undefined, selected as string)
    }
  }

  const handleCloseTerminal = (id: string) => {
    closeTerminal(id)
  }

  const handleStartRename = (id: string, currentName: string) => {
    setEditingId(id)
    setEditingName(currentName)
  }

  const handleFinishRename = () => {
    if (editingId && editingName.trim()) {
      renameTerminal(editingId, editingName.trim())
    }
    setEditingId(null)
    setEditingName('')
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleFinishRename()
    } else if (e.key === 'Escape') {
      setEditingId(null)
      setEditingName('')
    }
  }

  const handleTerminalExit = (terminalId: string, exitCode: number) => {
    updateTerminalStatus(terminalId, 'completed')
    // Show notification if this terminal is not currently selected
    if (selectedTerminalId !== terminalId) {
      setTerminalNotification(terminalId, true)
    }
    console.log(`Terminal ${terminalId} exited with code ${exitCode}`)
  }

  const getStatusIcon = (
    status: 'running' | 'completed',
    hasNotification: boolean
  ) => {
    if (hasNotification) {
      return <Circle className="h-3 w-3 fill-primary text-primary" />
    }
    switch (status) {
      case 'running':
        return <Play className="h-3 w-3 text-green-500" />
      case 'completed':
        return <CheckCircle className="h-3 w-3 text-muted-foreground" />
      default:
        return null
    }
  }

  const selectedTerminal = terminals.find(t => t.id === selectedTerminalId)

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between p-4 border-b">
        <h1 className="text-xl font-semibold">{t('droid.terminal.title')}</h1>
        <div className="flex items-center gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                className="h-8 w-8"
                onClick={handleCreateTerminalWithDir}
              >
                <FolderOpen className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {t('droid.terminal.newTerminalInDir')}
            </TooltipContent>
          </Tooltip>
          <Button variant="outline" size="sm" onClick={handleCreateTerminal}>
            <Plus className="h-4 w-4 mr-2" />
            {t('droid.terminal.newTerminal')}
          </Button>
        </div>
      </div>

      <div className="flex flex-1 overflow-hidden">
        {/* Terminal List */}
        <div className="w-56 border-r flex flex-col">
          <ScrollArea className="flex-1">
            <div className="p-2 space-y-1">
              {terminals.length === 0 ? (
                <div className="text-center text-muted-foreground text-sm py-8">
                  {t('droid.terminal.noTerminals')}
                </div>
              ) : (
                terminals.map(terminal => (
                  <ContextMenu key={terminal.id}>
                    <ContextMenuTrigger>
                      <button
                        onClick={() => selectTerminal(terminal.id)}
                        className={cn(
                          'w-full text-start p-2 rounded-md hover:bg-accent transition-colors',
                          selectedTerminalId === terminal.id && 'bg-accent'
                        )}
                      >
                        <div className="flex items-center gap-2">
                          {getStatusIcon(
                            terminal.status,
                            terminal.hasNotification
                          )}
                          {editingId === terminal.id ? (
                            <Input
                              value={editingName}
                              onChange={e => setEditingName(e.target.value)}
                              onBlur={handleFinishRename}
                              onKeyDown={handleKeyDown}
                              className="h-6 text-sm"
                              autoFocus
                              onClick={e => e.stopPropagation()}
                            />
                          ) : (
                            <span className="font-medium text-sm truncate flex-1">
                              {terminal.name}
                            </span>
                          )}
                        </div>
                        {terminal.cwd && (
                          <div className="text-xs text-muted-foreground mt-1 truncate pl-5">
                            {terminal.cwd}
                          </div>
                        )}
                      </button>
                    </ContextMenuTrigger>
                    <ContextMenuContent>
                      <ContextMenuItem
                        onClick={() =>
                          handleStartRename(terminal.id, terminal.name)
                        }
                      >
                        <Pencil className="h-4 w-4 mr-2" />
                        {t('droid.terminal.rename')}
                      </ContextMenuItem>
                      <ContextMenuItem
                        onClick={() => handleCloseTerminal(terminal.id)}
                        className="text-destructive"
                      >
                        <X className="h-4 w-4 mr-2" />
                        {t('droid.terminal.close')}
                      </ContextMenuItem>
                    </ContextMenuContent>
                  </ContextMenu>
                ))
              )}
            </div>
          </ScrollArea>
        </div>

        {/* Terminal View */}
        <div className="flex-1 flex flex-col min-w-0">
          {selectedTerminal ? (
            <TerminalView
              key={selectedTerminal.id}
              terminalId={selectedTerminal.id}
              cwd={selectedTerminal.cwd || undefined}
              onExit={exitCode =>
                handleTerminalExit(selectedTerminal.id, exitCode)
              }
            />
          ) : (
            <div className="flex items-center justify-center h-full text-muted-foreground">
              <p>{t('droid.terminal.selectOrCreate')}</p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
