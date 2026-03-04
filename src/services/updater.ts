/**
 * Update service module - handles app updates with progress tracking
 */

import { type Update, check } from '@tauri-apps/plugin-updater'
import { relaunch } from '@tauri-apps/plugin-process'
import { toast } from 'sonner'
import i18n from '@/i18n/config'
import { logger } from '@/lib/logger'
import { useUIStore } from '@/store/ui-store'

// Module-level variable to store the Update object
// This avoids needing to call check() again when user clicks "Install Now"
let cachedUpdate: Update | null = null

/**
 * Get the cached Update object
 */
export function getCachedUpdate(): Update | null {
  return cachedUpdate
}

/**
 * Set the cached Update object
 */
export function setCachedUpdate(update: Update | null): void {
  cachedUpdate = update
}

/**
 * Clear the cached Update object
 */
export function clearCachedUpdate(): void {
  cachedUpdate = null
}

/**
 * Check if update checking is disabled via environment variable
 */
export function isUpdateCheckDisabled(): boolean {
  const disableUpdate = import.meta.env.DROIDGEAR_DISABLE_UPDATE_CHECK
  return disableUpdate === 'true' || disableUpdate === '1'
}

/**
 * Check for updates and return the Update object if available
 */
export async function checkForUpdate(): Promise<Update | null> {
  if (isUpdateCheckDisabled()) {
    logger.debug('Update check disabled via environment variable')
    return null
  }

  try {
    const update = await check()
    if (update) {
      cachedUpdate = update
      return update
    }
    return null
  } catch (error) {
    logger.error('Update check failed', { error })
    return null
  }
}

/**
 * Download and install the update with progress tracking
 */
export async function downloadAndInstallUpdate(update?: Update): Promise<void> {
  const updateToInstall = update ?? cachedUpdate
  if (!updateToInstall) {
    logger.error('No update available to install')
    return
  }

  const t = i18n.t.bind(i18n)
  let totalSize = 0
  let downloadedSize = 0

  try {
    // Show initial progress toast
    toast.loading(t('update.downloadProgress', { progress: 0 }), {
      id: 'update-progress',
      duration: Infinity,
    })

    await updateToInstall.downloadAndInstall(event => {
      if (event.event === 'Started') {
        totalSize = event.data.contentLength ?? 0
        logger.info('Download started', { contentLength: totalSize })
      } else if (event.event === 'Progress') {
        downloadedSize += event.data.chunkLength
        const progress =
          totalSize > 0 ? Math.round((downloadedSize / totalSize) * 100) : 0
        // Update progress toast
        toast.loading(t('update.downloadProgress', { progress }), {
          id: 'update-progress',
          duration: Infinity,
        })
      } else if (event.event === 'Finished') {
        logger.info('Download finished')
        toast.loading(t('update.installing'), {
          id: 'update-progress',
          duration: Infinity,
        })
      }
    })

    // Clear the cached update and pending update state
    clearCachedUpdate()
    useUIStore.getState().clearPendingUpdate()

    // Show success and relaunch
    toast.success(t('update.completed'), { id: 'update-progress' })
    await relaunch()
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : String(error)
    logger.error('Failed to download/install update', { error: errorMessage })
    toast.error(t('update.failed', { error: errorMessage }), {
      id: 'update-progress',
    })
  }
}

/**
 * Show update notification toast with Install Now and Later buttons
 */
export function showUpdateNotification(update: Update): void {
  const t = i18n.t.bind(i18n)

  // Skip if we already notified about this version
  const currentPending = useUIStore.getState().pendingUpdate
  if (currentPending?.version === update.version) {
    return
  }

  // Cache the update object for later use
  setCachedUpdate(update)

  // Store update info in global store
  useUIStore.getState().setPendingUpdate({
    version: update.version,
    body: update.body ?? undefined,
  })

  // Show toast with action buttons
  toast(t('update.availableNotification', { version: update.version }), {
    id: 'update-available',
    duration: Infinity,
    action: {
      label: t('update.installNow'),
      onClick: () => {
        downloadAndInstallUpdate(update)
      },
    },
    cancel: {
      label: t('update.later'),
      onClick: () => {
        // Just dismiss the toast, update info remains in store
        toast.dismiss('update-available')
      },
    },
  })
}
