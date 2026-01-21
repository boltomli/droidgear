import { useTranslation } from 'react-i18next'
import { useState } from 'react'
import { Wrench, Plug, MessageSquare, TerminalSquare } from 'lucide-react'
import { cn } from '@/lib/utils'
import { ActionButton } from '@/components/ui/action-button'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { useUIStore } from '@/store/ui-store'
import { useCodexStore } from '@/store/codex-store'
import type { CodexSubView } from '@/store/ui-store'

export function CodexFeatureList() {
  const { t } = useTranslation()
  const codexSubView = useUIStore(state => state.codexSubView)
  const setCodexSubView = useUIStore(state => state.setCodexSubView)
  const codexHasChanges = useCodexStore(state => state.hasChanges)

  const [pendingSubView, setPendingSubView] = useState<CodexSubView | null>(
    null
  )

  const features: {
    id: CodexSubView
    labelKey: string
    icon: React.ElementType
  }[] = [
    { id: 'config', labelKey: 'codex.title', icon: Wrench },
    { id: 'mcp', labelKey: 'droid.features.mcp', icon: Plug },
    {
      id: 'sessions',
      labelKey: 'droid.features.sessions',
      icon: MessageSquare,
    },
    {
      id: 'terminal',
      labelKey: 'droid.features.terminal',
      icon: TerminalSquare,
    },
  ]

  const handleSubViewChange = (view: CodexSubView) => {
    if (view === codexSubView) return

    if (codexSubView === 'config' && codexHasChanges) {
      setPendingSubView(view)
    } else {
      setCodexSubView(view)
    }
  }

  const handleSaveAndSwitch = async () => {
    await useCodexStore.getState().saveProfile()
    if (pendingSubView) {
      setCodexSubView(pendingSubView)
      setPendingSubView(null)
    }
  }

  const handleDiscardAndSwitch = () => {
    useCodexStore.getState().resetChanges()
    if (pendingSubView) {
      setCodexSubView(pendingSubView)
      setPendingSubView(null)
    }
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-col gap-1 p-2">
        {features.map(feature => (
          <ActionButton
            key={feature.id}
            variant={codexSubView === feature.id ? 'secondary' : 'ghost'}
            size="sm"
            className={cn('justify-start w-full')}
            onClick={() => handleSubViewChange(feature.id)}
          >
            <feature.icon className="h-4 w-4 mr-2" />
            {t(feature.labelKey)}
          </ActionButton>
        ))}
      </div>

      <div className="mt-auto p-3 border-t text-xs text-muted-foreground">
        {t('codex.features.hint')}
      </div>

      <AlertDialog
        open={pendingSubView !== null}
        onOpenChange={open => !open && setPendingSubView(null)}
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
            <ActionButton
              variant="destructive"
              onClick={handleDiscardAndSwitch}
            >
              {t('sidebar.unsavedChanges.discard')}
            </ActionButton>
            <ActionButton onClick={handleSaveAndSwitch}>
              {t('sidebar.unsavedChanges.save')}
            </ActionButton>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
