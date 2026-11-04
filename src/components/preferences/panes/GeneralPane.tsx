import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
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
import { useIsWindows, useIsMacOS } from '@/hooks/use-platform'

/**
 * Releases page — the app ships no auto-update feed (the updater was
 * Tauri-only; see docs/developer/tauri-removal-design.zh.md), so "Check for
 * Updates" opens the published releases in the browser instead.
 */
const RELEASES_URL = 'https://github.com/Sunshow/droidgear/releases'

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

  const handleCheckForUpdates = async () => {
    await openUrl(RELEASES_URL)
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
          <Button variant="outline" size="sm" onClick={handleCheckForUpdates}>
            {t('preferences.general.checkForUpdates')}
          </Button>
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
