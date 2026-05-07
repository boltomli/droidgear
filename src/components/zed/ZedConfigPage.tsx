import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Plus,
  AlertCircle,
  RefreshCw,
  Play,
  Copy,
  Trash2,
  Download,
} from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useZedStore } from '@/store/zed-store'
import { ProviderCard } from './ProviderCard'
import { ProviderDialog } from './ProviderDialog'
import { ConfigStatus } from './ConfigStatus'

export function ZedConfigPage() {
  const { t } = useTranslation()
  const profiles = useZedStore(state => state.profiles)
  const activeProfileId = useZedStore(state => state.activeProfileId)
  const currentProfile = useZedStore(state => state.currentProfile)
  const isLoading = useZedStore(state => state.isLoading)
  const error = useZedStore(state => state.error)
  const configStatus = useZedStore(state => state.configStatus)

  const loadProfiles = useZedStore(state => state.loadProfiles)
  const loadActiveProfileId = useZedStore(state => state.loadActiveProfileId)
  const loadConfigStatus = useZedStore(state => state.loadConfigStatus)
  const selectProfile = useZedStore(state => state.selectProfile)
  const createProfile = useZedStore(state => state.createProfile)
  const deleteProfile = useZedStore(state => state.deleteProfile)
  const duplicateProfile = useZedStore(state => state.duplicateProfile)
  const applyProfile = useZedStore(state => state.applyProfile)
  const loadFromLiveConfig = useZedStore(state => state.loadFromLiveConfig)
  const deleteProvider = useZedStore(state => state.deleteProvider)
  const setError = useZedStore(state => state.setError)

  const [providerDialogOpen, setProviderDialogOpen] = useState(false)
  const [editingProviderId, setEditingProviderId] = useState<string | null>(
    null
  )
  const [deleteProviderId, setDeleteProviderId] = useState<string | null>(null)
  const [showApplyConfirm, setShowApplyConfirm] = useState(false)
  const [showDeleteProfileConfirm, setShowDeleteProfileConfirm] =
    useState(false)
  const [showCreateProfileDialog, setShowCreateProfileDialog] = useState(false)
  const [showDuplicateProfileDialog, setShowDuplicateProfileDialog] =
    useState(false)
  const [newProfileName, setNewProfileName] = useState('')
  const [applyLoading, setApplyLoading] = useState(false)

  // Local editing state for profile fields
  const profileKey = currentProfile?.id ?? ''
  const [editingName, setEditingName] = useState(currentProfile?.name ?? '')
  const [editingDescription, setEditingDescription] = useState(
    currentProfile?.description ?? ''
  )

  // Reset local state when profile changes
  const [lastProfileKey, setLastProfileKey] = useState(profileKey)
  if (profileKey !== lastProfileKey) {
    setLastProfileKey(profileKey)
    setEditingName(currentProfile?.name ?? '')
    setEditingDescription(currentProfile?.description ?? '')
  }

  useEffect(() => {
    const init = async () => {
      await loadProfiles()
      await loadActiveProfileId()
    }
    init()
    loadConfigStatus()
  }, [loadProfiles, loadActiveProfileId, loadConfigStatus])

  const handleProfileChange = (profileId: string) => {
    selectProfile(profileId)
  }

  const handleCreateProfile = async () => {
    if (!newProfileName.trim()) return
    await createProfile(newProfileName.trim())
    setNewProfileName('')
    setShowCreateProfileDialog(false)
  }

  const handleDuplicateProfile = async () => {
    if (!currentProfile || !newProfileName.trim()) return
    await duplicateProfile(currentProfile.id, newProfileName.trim())
    setNewProfileName('')
    setShowDuplicateProfileDialog(false)
  }

  const handleDeleteProfile = async () => {
    if (!currentProfile) return
    await deleteProfile(currentProfile.id)
    setShowDeleteProfileConfirm(false)
  }

  const handleApply = async () => {
    if (!currentProfile) return
    setApplyLoading(true)
    setError(null)
    await applyProfile(currentProfile.id)
    setShowApplyConfirm(false)
    setApplyLoading(false)
    const currentError = useZedStore.getState().error
    if (!currentError) {
      toast.success(t('zed.actions.applySuccess'))
    } else {
      toast.error(t('zed.actions.applyError'))
    }
  }

  const handleLoadFromConfig = async () => {
    setError(null)
    await loadFromLiveConfig()
    const currentError = useZedStore.getState().error
    if (!currentError) {
      toast.success(t('zed.providers.loadFromConfigSuccess'))
    } else {
      toast.error(currentError)
    }
  }

  const handleAddProvider = () => {
    setEditingProviderId(null)
    setProviderDialogOpen(true)
  }

  const handleEditProvider = (providerId: string) => {
    setEditingProviderId(providerId)
    setProviderDialogOpen(true)
  }

  const handleConfirmDeleteProvider = () => {
    if (deleteProviderId) {
      deleteProvider(deleteProviderId)
      setDeleteProviderId(null)
    }
  }

  const providerEntries = currentProfile?.providers
    ? Object.entries(currentProfile.providers)
    : []

  const isActiveProfile =
    currentProfile !== null && activeProfileId === currentProfile.id

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between gap-2 p-4 border-b">
        <div className="min-w-0 flex-1">
          <h1 className="text-xl font-semibold">{t('zed.title')}</h1>
          <div className="flex items-center gap-2 mt-1">
            {isActiveProfile && (
              <Badge variant="outline">{t('zed.profile.active')}</Badge>
            )}
          </div>
        </div>
        <div className="flex items-center gap-2 flex-shrink-0">
          <Button
            variant="outline"
            size="icon"
            onClick={() => {
              loadProfiles()
              loadConfigStatus()
            }}
            disabled={isLoading}
            title={t('common.refresh')}
          >
            <RefreshCw className="h-4 w-4" />
          </Button>
          <Button
            onClick={() => setShowApplyConfirm(true)}
            disabled={!currentProfile || isLoading || applyLoading}
          >
            {applyLoading ? (
              <RefreshCw className="h-4 w-4 mr-2 animate-spin" />
            ) : (
              <Play className="h-4 w-4 mr-2" />
            )}
            {isActiveProfile
              ? t('zed.actions.applied')
              : t('zed.actions.apply')}
          </Button>
        </div>
      </div>

      {/* Error Alert */}
      {error && (
        <div className="mx-4 mt-4 p-3 bg-destructive/10 border border-destructive/20 rounded-md flex items-center gap-2">
          <AlertCircle className="h-4 w-4 text-destructive" />
          <span className="text-sm text-destructive">{error}</span>
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto"
            onClick={() => setError(null)}
          >
            {t('common.dismiss')}
          </Button>
        </div>
      )}

      {/* Main Content */}
      <div className="flex-1 overflow-auto p-4 space-y-4">
        {/* Profile Section */}
        <div className="space-y-3 p-4 border rounded-lg">
          <div className="flex items-center gap-2">
            <Label className="w-20">{t('zed.profile.select')}</Label>
            <Select
              value={currentProfile?.id ?? ''}
              onValueChange={handleProfileChange}
            >
              <SelectTrigger className="flex-1">
                <SelectValue placeholder={t('zed.profile.select')} />
              </SelectTrigger>
              <SelectContent>
                {profiles.map(profile => (
                  <SelectItem key={profile.id} value={profile.id}>
                    {profile.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              variant="outline"
              size="icon"
              onClick={() => setShowCreateProfileDialog(true)}
              title={t('zed.profile.create')}
            >
              <Plus className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              onClick={() => {
                setNewProfileName(
                  currentProfile?.name ? `${currentProfile.name} (Copy)` : ''
                )
                setShowDuplicateProfileDialog(true)
              }}
              disabled={!currentProfile}
              title={t('zed.profile.duplicate')}
            >
              <Copy className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              onClick={() => setShowDeleteProfileConfirm(true)}
              disabled={!currentProfile || profiles.length <= 1}
              title={t('zed.profile.delete')}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>

          {currentProfile && (
            <>
              <div className="flex items-center gap-2">
                <Label className="w-24">{t('zed.profile.name')}</Label>
                <Input
                  value={editingName}
                  onChange={e => setEditingName(e.target.value)}
                  onBlur={() => {
                    if (currentProfile && editingName !== currentProfile.name) {
                      useZedStore.getState().updateProfileName(editingName)
                    }
                  }}
                  placeholder={t('zed.profile.namePlaceholder')}
                />
              </div>
              <div className="flex items-center gap-2">
                <Label className="w-24">{t('zed.profile.description')}</Label>
                <Input
                  value={editingDescription}
                  onChange={e => setEditingDescription(e.target.value)}
                  onBlur={() => {
                    if (
                      currentProfile &&
                      editingDescription !== (currentProfile.description ?? '')
                    ) {
                      useZedStore
                        .getState()
                        .updateProfileDescription(editingDescription)
                    }
                  }}
                  placeholder={t('zed.profile.descriptionPlaceholder')}
                />
              </div>
            </>
          )}
        </div>

        {/* Providers Section */}
        <div className="space-y-3 p-4 border rounded-lg">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-medium">{t('zed.providers.title')}</h2>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={handleLoadFromConfig}
                disabled={!currentProfile || !configStatus?.configExists}
                title={t('zed.providers.loadFromConfig')}
              >
                <Download className="h-4 w-4 mr-2" />
                {t('zed.providers.loadFromConfig')}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handleAddProvider}
                disabled={!currentProfile}
              >
                <Plus className="h-4 w-4 mr-2" />
                {t('zed.provider.add')}
              </Button>
            </div>
          </div>

          {providerEntries.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              {t('zed.provider.noProviders')}
            </div>
          ) : (
            <div className="space-y-2">
              {providerEntries.map(([providerId, config]) => (
                <ProviderCard
                  key={providerId}
                  providerId={providerId}
                  config={config ?? { api_url: '', availableModels: null }}
                  onEdit={() => handleEditProvider(providerId)}
                  onDelete={() => setDeleteProviderId(providerId)}
                />
              ))}
            </div>
          )}
        </div>

        {/* Config Status */}
        <ConfigStatus status={configStatus} isActiveProfile={isActiveProfile} />
      </div>

      {/* Provider Dialog */}
      <ProviderDialog
        open={providerDialogOpen}
        onOpenChange={setProviderDialogOpen}
        editingProviderId={editingProviderId}
        currentProfile={currentProfile}
      />

      {/* Apply Confirmation */}
      <AlertDialog open={showApplyConfirm} onOpenChange={setShowApplyConfirm}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('zed.actions.apply')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('zed.actions.applyConfirm')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={handleApply}>
              {t('zed.actions.apply')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Delete Provider Confirmation */}
      <AlertDialog
        open={deleteProviderId !== null}
        onOpenChange={() => setDeleteProviderId(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('zed.provider.delete')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('zed.provider.deleteConfirm')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={handleConfirmDeleteProvider}>
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Delete Profile Confirmation */}
      <AlertDialog
        open={showDeleteProfileConfirm}
        onOpenChange={setShowDeleteProfileConfirm}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('zed.profile.delete')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('zed.profile.deleteConfirm')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={handleDeleteProfile}>
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Create Profile Dialog */}
      <Dialog
        open={showCreateProfileDialog}
        onOpenChange={setShowCreateProfileDialog}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('zed.profile.create')}</DialogTitle>
            <DialogDescription>
              {t('zed.profile.createDescription')}
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <Input
              value={newProfileName}
              onChange={e => setNewProfileName(e.target.value)}
              placeholder={t('zed.profile.namePlaceholder')}
              onKeyDown={e => e.key === 'Enter' && handleCreateProfile()}
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShowCreateProfileDialog(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              onClick={handleCreateProfile}
              disabled={!newProfileName.trim()}
            >
              {t('common.add')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Duplicate Profile Dialog */}
      <Dialog
        open={showDuplicateProfileDialog}
        onOpenChange={setShowDuplicateProfileDialog}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('zed.profile.duplicate')}</DialogTitle>
            <DialogDescription>
              {t('zed.profile.duplicateDescription')}
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <Input
              value={newProfileName}
              onChange={e => setNewProfileName(e.target.value)}
              placeholder={t('zed.profile.namePlaceholder')}
              onKeyDown={e => e.key === 'Enter' && handleDuplicateProfile()}
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShowDuplicateProfileDialog(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              onClick={handleDuplicateProfile}
              disabled={!newProfileName.trim()}
            >
              {t('zed.profile.duplicate')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
