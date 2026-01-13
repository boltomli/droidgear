import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { check, type Update } from '@tauri-apps/plugin-updater'
import { relaunch } from '@tauri-apps/plugin-process'
import { Button } from '@/components/ui/button'
import { SettingsField, SettingsSection } from '../shared/SettingsComponents'
import { commands } from '@/lib/tauri-bindings'
import { logger } from '@/lib/logger'
import { useUIStore } from '@/store/ui-store'

export function GeneralPane() {
  const { t } = useTranslation()

  // Update state
  const [updateStatus, setUpdateStatus] = useState<
    'idle' | 'checking' | 'downloading' | 'installing'
  >('idle')
  const [availableUpdate, setAvailableUpdate] = useState<Update | null>(null)
  const [updateError, setUpdateError] = useState<string | null>(null)

  // Read pending update from global store (set by auto-updater in App.tsx)
  const pendingUpdate = useUIStore(state => state.pendingUpdate)

  // Sync pending update from store to local state on mount
  useEffect(() => {
    if (pendingUpdate && !availableUpdate) {
      // We have a pending update from auto-check, show it
      logger.info('Displaying pending update from auto-check', {
        version: pendingUpdate.version,
      })
    }
  }, [pendingUpdate, availableUpdate])

  // Get current app version
  const { data: appVersion } = useQuery({
    queryKey: ['app-version'],
    queryFn: async () => {
      return await commands.getAppVersion()
    },
    staleTime: Infinity,
  })

  const handleCheckForUpdates = async () => {
    setUpdateStatus('checking')
    setUpdateError(null)
    setAvailableUpdate(null)

    try {
      const update = await check()
      if (update) {
        setAvailableUpdate(update)
        // Also update global store
        useUIStore.getState().setPendingUpdate({
          version: update.version,
          body: update.body ?? undefined,
        })
        logger.info('Update available', { version: update.version })
      } else {
        logger.info('No updates available')
      }
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : String(error)
      setUpdateError(errorMessage)
      logger.error('Failed to check for updates', { error: errorMessage })
    } finally {
      setUpdateStatus('idle')
    }
  }

  const handleDownloadAndInstall = async () => {
    if (!availableUpdate) return

    setUpdateStatus('downloading')
    try {
      await availableUpdate.downloadAndInstall(event => {
        if (event.event === 'Started') {
          logger.info('Download started', {
            contentLength: event.data.contentLength,
          })
        } else if (event.event === 'Progress') {
          logger.debug('Download progress', {
            chunkLength: event.data.chunkLength,
          })
        } else if (event.event === 'Finished') {
          logger.info('Download finished')
          setUpdateStatus('installing')
        }
      })
      toast.success(t('update.completed'))
      await relaunch()
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : String(error)
      setUpdateError(errorMessage)
      setUpdateStatus('idle')
      logger.error('Failed to download/install update', { error: errorMessage })
    }
  }

  return (
    <div className="space-y-6">
      <SettingsSection title={t('preferences.general.softwareUpdate')}>
        <SettingsField
          label={t('preferences.general.currentVersion', {
            version: appVersion ?? '...',
          })}
          description=""
        >
          {/* Only show check button when no update is available */}
          {!availableUpdate && !pendingUpdate && (
            <div className="flex items-center gap-3">
              <Button
                variant="outline"
                size="sm"
                onClick={handleCheckForUpdates}
                disabled={updateStatus !== 'idle'}
              >
                {updateStatus === 'checking'
                  ? t('preferences.general.checking')
                  : updateStatus === 'downloading'
                    ? t('preferences.general.downloading')
                    : updateStatus === 'installing'
                      ? t('preferences.general.installing')
                      : t('preferences.general.checkForUpdates')}
              </Button>
            </div>
          )}
        </SettingsField>

        {/* Update status display - show availableUpdate (from manual check) or pendingUpdate (from auto-check) */}
        {updateStatus === 'idle' && (availableUpdate || pendingUpdate) && (
          <>
            <SettingsField
              label={t('preferences.general.updateAvailable', {
                version: availableUpdate?.version ?? pendingUpdate?.version,
              })}
              description=""
            >
              {availableUpdate ? (
                <Button
                  variant="default"
                  size="sm"
                  onClick={handleDownloadAndInstall}
                >
                  {t('preferences.general.downloadAndInstall')}
                </Button>
              ) : (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleCheckForUpdates}
                >
                  {t('preferences.general.checkForUpdates')}
                </Button>
              )}
            </SettingsField>
          </>
        )}

        {updateStatus === 'idle' &&
          !availableUpdate &&
          !pendingUpdate &&
          !updateError && (
            <p className="text-sm text-muted-foreground">
              {t('preferences.general.upToDate')}
            </p>
          )}

        {updateError && (
          <p className="text-sm text-destructive">
            {t('preferences.general.updateFailed')}: {updateError}
          </p>
        )}

        {/* Always show release notes if available */}
        {(availableUpdate?.body ?? pendingUpdate?.body) && (
          <div className="text-sm text-muted-foreground whitespace-pre-wrap max-h-48 overflow-y-auto rounded-md border p-3">
            {availableUpdate?.body ?? pendingUpdate?.body}
          </div>
        )}
      </SettingsSection>
    </div>
  )
}
