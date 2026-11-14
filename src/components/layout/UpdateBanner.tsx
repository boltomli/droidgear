import { useTranslation } from 'react-i18next'
import { ArrowUpCircle, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { usePreferences } from '@/services/preferences'
import { useDismissUpdate, useUpdateCheck } from '@/services/updates'
import { useUIStore } from '@/store/ui-store'

/**
 * Startup update notice for the single-file release pipeline.
 *
 * A newer `wails-v*` release shows up here once, on launch, unless the user
 * turned the startup check off (Preferences → General → "check at startup") or
 * dismissed this version. Installing happens in Preferences → General, where
 * the progress bar and the SHA-256-verified download live — the banner only
 * routes there.
 */
export function UpdateBanner() {
  const { t } = useTranslation()
  const { data: preferences } = usePreferences()
  // Preferences may still be loading: wait until they are known so a disabled
  // startup check never fires a request.
  const autoCheckEnabled =
    preferences !== undefined && preferences.disable_auto_update !== true
  const check = useUpdateCheck({ enabled: autoCheckEnabled })
  const dismiss = useDismissUpdate()

  const update = check.data
  if (!update?.updateAvailable) return null
  if (update.dismissedVersion === update.latestVersion) return null

  return (
    <div className="flex items-center gap-3 border-b border-border bg-muted/60 px-4 py-2 text-sm">
      <ArrowUpCircle className="size-4 shrink-0 text-primary" />
      <span className="flex-1 truncate">
        {t('updates.banner.title', { version: update.latestVersion })}
      </span>
      {update.preview && (
        <span className="shrink-0 rounded bg-primary/10 px-1.5 py-0.5 text-xs text-primary">
          {t('updates.preview')}
        </span>
      )}
      <Button
        size="sm"
        variant="outline"
        className="h-7"
        onClick={() => useUIStore.getState().setPreferencesOpen(true)}
      >
        {t('updates.banner.action')}
      </Button>
      <Button
        size="icon"
        variant="ghost"
        className="size-7"
        aria-label={t('updates.banner.dismiss')}
        onClick={() => dismiss.mutate(update.latestVersion)}
      >
        <X className="size-4" />
      </Button>
    </div>
  )
}
