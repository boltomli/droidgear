import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  Download,
  ExternalLink,
  Loader2,
  RefreshCw,
  RotateCcw,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { SettingsField, SettingsSection } from '../shared/SettingsComponents'
import { commands } from '@/lib/app-bindings'
import { openUrl } from '@/lib/shell/opener'
import { usePreferences, useSavePreferences } from '@/services/preferences'
import {
  useApplyUpdate,
  useDownloadUpdate,
  usePendingUpdate,
  useUpdateCheck,
  useUpdateProgress,
} from '@/services/updates'
import { useIsWindows, useIsMacOS } from '@/hooks/use-platform'

/**
 * Software update — the app ships as a single-file `wails-v*` release, so it
 * can replace itself: check GitHub, download the platform asset, verify it
 * against the release's SHA256SUMS.txt, swap the executable and restart
 * (internal/service/updater; `dg update` is the CLI half of the same service).
 * The releases page stays reachable for manual downloads.
 */
const RELEASES_URL = 'https://github.com/boltomli/droidgear/releases'

/** Renders a byte count for the asset-size line (no dependency). */
function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  let value = bytes
  for (const unit of ['KiB', 'MiB', 'GiB']) {
    value /= 1024
    if (value < 1024) return `${value.toFixed(1)} ${unit}`
  }
  return `${value.toFixed(1)} TiB`
}

export function GeneralPane() {
  const { t } = useTranslation()
  const { data: preferences } = usePreferences()
  const savePreferences = useSavePreferences()
  const isWindows = useIsWindows()
  const isMacOS = useIsMacOS()

  const { data: appVersion } = useQuery({
    queryKey: ['app-version'],
    queryFn: async () => {
      return await commands.getAppVersion()
    },
    staleTime: Infinity,
  })

  // `enabled: true` here (not the preference) so the button always works even
  // when the startup check is off; the banner owns that decision.
  const check = useUpdateCheck()
  const pending = usePendingUpdate()
  const download = useDownloadUpdate()
  const apply = useApplyUpdate()
  const { progress, reset: resetProgress } = useUpdateProgress()
  const [restarting, setRestarting] = useState(false)

  const autoCheckEnabled = preferences?.disable_auto_update !== true
  const update = check.data
  const staged = pending.data
  const stagedReady =
    !!staged &&
    !!update &&
    staged.version === update.latestVersion &&
    staged.assetName === update.assetName
  const downloading = download.isPending

  const handleApply = () => {
    setRestarting(true)
    apply.mutate(undefined, { onSettled: () => setRestarting(false) })
  }

  const handleDownload = () => {
    resetProgress()
    download.mutate()
  }

  return (
    <div className="space-y-6">
      <SettingsSection title={t('preferences.general.softwareUpdate')}>
        <SettingsField
          label={t('preferences.general.currentVersion', {
            version: appVersion ?? '...',
          })}
          description={update?.devBuild ? t('updates.devBuild') : ''}
        >
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => void check.refetch()}
              disabled={check.isFetching || downloading}
            >
              {check.isFetching ? (
                <Loader2 className="mr-2 size-4 animate-spin" />
              ) : (
                <RefreshCw className="mr-2 size-4" />
              )}
              {t('preferences.general.checkForUpdates')}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => void openUrl(RELEASES_URL)}
            >
              <ExternalLink className="mr-2 size-4" />
              {t('updates.releasesPage')}
            </Button>
          </div>
        </SettingsField>

        {check.isError && (
          <p className="text-sm text-destructive">
            {t('preferences.general.updateFailed')}: {String(check.error)}
          </p>
        )}

        {!check.isError &&
          update &&
          !update.devBuild &&
          !update.updateAvailable && (
            <p className="text-sm text-muted-foreground">
              {t('preferences.general.upToDate')}
            </p>
          )}

        {update?.updateAvailable && (
          <div className="space-y-3 rounded-lg border border-border p-3">
            <div className="flex items-center justify-between gap-3">
              <div className="space-y-1">
                <p className="text-sm font-medium">
                  {t('preferences.general.updateAvailable', {
                    version: update.latestVersion,
                  })}
                </p>
                <p className="text-xs text-muted-foreground">
                  {update.assetName}
                  {update.assetSize
                    ? ` · ${formatBytes(update.assetSize)}`
                    : ''}
                  {update.publishedAt
                    ? ` · ${new Date(update.publishedAt).toLocaleDateString()}`
                    : ''}
                </p>
              </div>
              {stagedReady ? (
                <Button size="sm" onClick={handleApply} disabled={restarting}>
                  {restarting ? (
                    <Loader2 className="mr-2 size-4 animate-spin" />
                  ) : (
                    <RotateCcw className="mr-2 size-4" />
                  )}
                  {restarting
                    ? t('updates.restarting')
                    : t('updates.restartToInstall')}
                </Button>
              ) : (
                <Button
                  size="sm"
                  onClick={handleDownload}
                  disabled={downloading}
                >
                  {downloading ? (
                    <Loader2 className="mr-2 size-4 animate-spin" />
                  ) : (
                    <Download className="mr-2 size-4" />
                  )}
                  {downloading
                    ? t('updates.downloading', {
                        percent: Math.round(progress?.percent ?? 0),
                      })
                    : t('preferences.general.downloadAndInstall')}
                </Button>
              )}
            </div>

            {downloading && (
              <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
                <div
                  className="h-full rounded-full bg-primary transition-[width] duration-150"
                  style={{
                    width: `${Math.max(2, Math.round(progress?.percent ?? 0))}%`,
                  }}
                />
              </div>
            )}

            {stagedReady && (
              <p className="text-xs text-muted-foreground">
                {t('updates.staged', { version: staged.version })}
              </p>
            )}

            {update.releaseUrl && (
              <Button
                variant="link"
                size="sm"
                className="h-auto p-0 text-xs"
                onClick={() => void openUrl(update.releaseUrl as string)}
              >
                {t('updates.releaseNotes')}
              </Button>
            )}
          </div>
        )}

        <SettingsField
          label={t('preferences.general.autoUpdate')}
          description={t('preferences.general.autoUpdateDescription')}
        >
          <Switch
            checked={autoCheckEnabled}
            onCheckedChange={checked => {
              if (preferences) {
                savePreferences.mutate({
                  ...preferences,
                  disable_auto_update: !checked,
                })
              }
            }}
          />
        </SettingsField>
      </SettingsSection>

      <SettingsSection title={t('preferences.general.terminal')}>
        <SettingsField
          label={t('preferences.general.preferredTerminal')}
          description={t('preferences.general.preferredTerminalDescription')}
        >
          <Select
            value={
              !preferences?.preferred_terminal ||
              preferences.preferred_terminal === 'windows-terminal'
                ? 'system-default'
                : preferences.preferred_terminal
            }
            onValueChange={value => {
              if (preferences) {
                savePreferences.mutate({
                  ...preferences,
                  preferred_terminal:
                    value === 'system-default' || value === 'windows-terminal'
                      ? null
                      : value || null,
                })
              }
            }}
          >
            <SelectTrigger className="w-48">
              <SelectValue
                placeholder={t('preferences.general.terminalSystemDefault')}
              />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="system-default">
                {isMacOS || isWindows
                  ? t('preferences.general.terminalSystemDefault')
                  : t('preferences.general.terminalAutoDetect')}
              </SelectItem>
              {isMacOS && (
                <>
                  <SelectItem value="terminal">
                    {t('preferences.general.terminalApp')}
                  </SelectItem>
                  <SelectItem value="iterm2">iTerm2</SelectItem>
                  <SelectItem value="ghostty">Ghostty</SelectItem>
                </>
              )}
              {!isMacOS && !isWindows && (
                <>
                  <SelectItem value="gnome-terminal">GNOME Terminal</SelectItem>
                  <SelectItem value="konsole">Konsole</SelectItem>
                  <SelectItem value="xfce4-terminal">XFCE Terminal</SelectItem>
                  <SelectItem value="x-terminal-emulator">
                    x-terminal-emulator
                  </SelectItem>
                </>
              )}
              {isWindows && (
                <>
                  <SelectItem value="cmd">
                    {t('preferences.general.terminalCmd')}
                  </SelectItem>
                  <SelectItem value="powershell">PowerShell</SelectItem>
                </>
              )}
            </SelectContent>
          </Select>
        </SettingsField>
      </SettingsSection>
    </div>
  )
}
