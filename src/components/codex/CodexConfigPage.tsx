import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Save, RefreshCw, Upload, Download, Copy } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useCodexStore } from '@/store/codex-store'
import type { JsonValue } from '@/lib/bindings'

export function CodexConfigPage() {
  const { t } = useTranslation()

  const profiles = useCodexStore(state => state.profiles)
  const activeProfileId = useCodexStore(state => state.activeProfileId)
  const currentProfile = useCodexStore(state => state.currentProfile)
  const hasChanges = useCodexStore(state => state.hasChanges)
  const isLoading = useCodexStore(state => state.isLoading)
  const error = useCodexStore(state => state.error)
  const configStatus = useCodexStore(state => state.configStatus)

  const loadProfiles = useCodexStore(state => state.loadProfiles)
  const loadActiveProfileId = useCodexStore(state => state.loadActiveProfileId)
  const loadConfigStatus = useCodexStore(state => state.loadConfigStatus)
  const selectProfile = useCodexStore(state => state.selectProfile)
  const saveProfile = useCodexStore(state => state.saveProfile)
  const applyProfile = useCodexStore(state => state.applyProfile)
  const resetChanges = useCodexStore(state => state.resetChanges)
  const updateProfileName = useCodexStore(state => state.updateProfileName)
  const updateProfileDescription = useCodexStore(
    state => state.updateProfileDescription
  )
  const updateAuthValue = useCodexStore(state => state.updateAuthValue)
  const updateConfigToml = useCodexStore(state => state.updateConfigToml)
  const loadFromLiveConfig = useCodexStore(state => state.loadFromLiveConfig)
  const setError = useCodexStore(state => state.setError)
  const duplicateProfile = useCodexStore(state => state.duplicateProfile)
  const deleteProfile = useCodexStore(state => state.deleteProfile)
  const createProfile = useCodexStore(state => state.createProfile)

  const [applyConfirmOpen, setApplyConfirmOpen] = useState(false)
  const [newProfileName, setNewProfileName] = useState('')
  const [duplicateName, setDuplicateName] = useState('')

  const apiKey = useMemo(() => {
    const auth = (currentProfile?.auth || {}) as Record<
      string,
      JsonValue | undefined
    >
    const value = auth.OPENAI_API_KEY
    return typeof value === 'string' ? value : ''
  }, [currentProfile])

  useEffect(() => {
    let cancelled = false
    void (async () => {
      await loadProfiles()
      await loadActiveProfileId()
      await loadConfigStatus()
      if (cancelled) return
      const state = useCodexStore.getState()
      if (!state.currentProfile && state.profiles[0])
        state.selectProfile(state.profiles[0].id)
    })()
    return () => {
      cancelled = true
    }
  }, [loadActiveProfileId, loadConfigStatus, loadProfiles])

  useEffect(() => {
    if (error) toast.error(error)
  }, [error])

  const handleCopyPath = async (text?: string) => {
    if (!text) return
    try {
      await navigator.clipboard.writeText(text)
      toast.success(t('common.copied'))
    } catch (e) {
      toast.error(String(e))
    }
  }

  const handleCreateProfile = async () => {
    if (!newProfileName.trim()) return
    await createProfile(newProfileName.trim())
    setNewProfileName('')
    await loadProfiles()
    toast.success(t('codex.profile.created'))
  }

  const handleDuplicateProfile = async () => {
    if (!currentProfile || !duplicateName.trim()) return
    await duplicateProfile(currentProfile.id, duplicateName.trim())
    setDuplicateName('')
    toast.success(t('codex.profile.duplicated'))
  }

  const handleDeleteProfile = async () => {
    if (!currentProfile) return
    await deleteProfile(currentProfile.id)
    toast.success(t('codex.profile.deleted'))
  }

  const headerBadges = useMemo(() => {
    if (!currentProfile) return null
    const isActive = activeProfileId === currentProfile.id
    return (
      <div className="flex items-center gap-2">
        {isActive && (
          <Badge variant="outline">{t('codex.profile.active')}</Badge>
        )}
        {hasChanges && <Badge variant="secondary">{t('common.unsaved')}</Badge>}
      </div>
    )
  }, [activeProfileId, currentProfile, hasChanges, t])

  if (!currentProfile) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground">
        <p>{t('codex.profile.noProfile')}</p>
      </div>
    )
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-semibold">{t('codex.title')}</h1>
          {headerBadges}
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={async () => {
              await loadFromLiveConfig()
              toast.success(t('codex.actions.loadedFromLive'))
            }}
          >
            <Download className="h-4 w-4 mr-2" />
            {t('codex.actions.loadFromLive')}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={async () => {
              await saveProfile()
              toast.success(t('common.saved'))
            }}
            disabled={!hasChanges}
          >
            <Save className="h-4 w-4 mr-2" />
            {t('common.save')}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              resetChanges()
              toast.success(t('common.reset'))
            }}
            disabled={!hasChanges}
          >
            <RefreshCw className="h-4 w-4 mr-2" />
            {t('common.reset')}
          </Button>
          <Button
            size="sm"
            onClick={() => setApplyConfirmOpen(true)}
            disabled={isLoading}
          >
            <Upload className="h-4 w-4 mr-2" />
            {t('codex.actions.apply')}
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="space-y-4">
          <div className="space-y-2">
            <Label>{t('codex.profile.select')}</Label>
            <Select value={currentProfile.id} onValueChange={selectProfile}>
              <SelectTrigger>
                <SelectValue placeholder={t('codex.profile.select')} />
              </SelectTrigger>
              <SelectContent>
                {profiles.map(p => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label>{t('codex.profile.name')}</Label>
            <Input
              value={currentProfile.name}
              onChange={e => updateProfileName(e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label>{t('codex.profile.description')}</Label>
            <Input
              value={currentProfile.description ?? ''}
              onChange={e => updateProfileDescription(e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label>{t('codex.profile.create')}</Label>
            <div className="flex gap-2">
              <Input
                value={newProfileName}
                onChange={e => setNewProfileName(e.target.value)}
                placeholder={t('codex.profile.createPlaceholder')}
              />
              <Button variant="secondary" onClick={handleCreateProfile}>
                {t('codex.profile.createAction')}
              </Button>
            </div>
          </div>

          <div className="space-y-2">
            <Label>{t('codex.profile.duplicate')}</Label>
            <div className="flex gap-2">
              <Input
                value={duplicateName}
                onChange={e => setDuplicateName(e.target.value)}
                placeholder={t('codex.profile.duplicatePlaceholder')}
              />
              <Button variant="secondary" onClick={handleDuplicateProfile}>
                {t('codex.profile.duplicateAction')}
              </Button>
            </div>
          </div>

          <div className="space-y-2">
            <Label>{t('codex.profile.delete')}</Label>
            <Button variant="destructive" onClick={handleDeleteProfile}>
              {t('codex.profile.deleteAction')}
            </Button>
          </div>

          <div className="space-y-2">
            <Label>{t('codex.live.status')}</Label>
            <div className="text-xs text-muted-foreground space-y-1">
              <div className="flex items-center justify-between gap-2">
                <span>{t('codex.live.authPath')}</span>
                <div className="flex items-center gap-2">
                  <Badge variant="outline">
                    {configStatus?.authExists
                      ? t('common.exists')
                      : t('common.missing')}
                  </Badge>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => handleCopyPath(configStatus?.authPath)}
                    title={t('common.copy')}
                  >
                    <Copy className="h-4 w-4" />
                  </Button>
                </div>
              </div>
              <div className="flex items-center justify-between gap-2">
                <span>{t('codex.live.configPath')}</span>
                <div className="flex items-center gap-2">
                  <Badge variant="outline">
                    {configStatus?.configExists
                      ? t('common.exists')
                      : t('common.missing')}
                  </Badge>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => handleCopyPath(configStatus?.configPath)}
                    title={t('common.copy')}
                  >
                    <Copy className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div className="lg:col-span-2 space-y-4">
          <div className="space-y-2">
            <Label>{t('codex.auth.apiKey')}</Label>
            <Input
              value={apiKey}
              onChange={e => updateAuthValue('OPENAI_API_KEY', e.target.value)}
              placeholder={t('codex.auth.apiKeyPlaceholder')}
              type="password"
              autoComplete="off"
            />
            <div className="text-xs text-muted-foreground">
              {t('codex.auth.apiKeyHint')}
            </div>
          </div>

          <div className="space-y-2">
            <Label>{t('codex.configToml')}</Label>
            <Textarea
              value={currentProfile.configToml}
              onChange={e => updateConfigToml(e.target.value)}
              className="min-h-[320px] font-mono text-xs"
              spellCheck={false}
            />
            <div className="text-xs text-muted-foreground">
              {t('codex.configTomlHint')}
            </div>
          </div>
        </div>
      </div>

      <AlertDialog open={applyConfirmOpen} onOpenChange={setApplyConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('codex.actions.apply')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('codex.actions.applyConfirm')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => setError(null)}>
              {t('common.cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={async () => {
                await applyProfile(currentProfile.id)
                setApplyConfirmOpen(false)
                toast.success(t('codex.actions.applied'))
              }}
            >
              {t('codex.actions.apply')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
