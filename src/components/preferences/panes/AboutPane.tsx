import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { ExternalLink } from 'lucide-react'
import { openUrl } from '@tauri-apps/plugin-opener'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { commands } from '@/lib/tauri-bindings'

const PROJECT_URL = 'https://github.com/Sunshow/droidgear'
const TEMPLATE_URL = 'https://github.com/dannysmith/tauri-template'

export function AboutPane() {
  const { t } = useTranslation()

  const { data: appVersion } = useQuery({
    queryKey: ['app-version'],
    queryFn: async () => {
      return await commands.getAppVersion()
    },
    staleTime: Infinity,
  })

  const handleOpenUrl = async (url: string) => {
    await openUrl(url)
  }

  return (
    <div className="flex flex-col items-center justify-center space-y-6 py-8">
      {/* App Icon & Name */}
      <div className="flex flex-col items-center space-y-3">
        <img
          src="/Icon512.png"
          alt="DroidGear"
          className="h-20 w-20 rounded-2xl shadow-lg"
        />
        <div className="text-center">
          <h1 className="text-2xl font-bold text-foreground">DroidGear</h1>
          <p className="text-sm text-muted-foreground">
            v{appVersion ?? '...'}
          </p>
        </div>
      </div>

      {/* Description */}
      <p className="max-w-md text-center text-sm text-muted-foreground">
        {t('preferences.about.description')}
      </p>

      <Separator className="w-64" />

      {/* Project URL */}
      <div className="flex flex-col items-center space-y-2">
        <span className="text-sm font-medium text-foreground">
          {t('preferences.about.projectUrl')}
        </span>
        <Button
          variant="outline"
          size="sm"
          onClick={() => handleOpenUrl(PROJECT_URL)}
          className="gap-2"
        >
          <ExternalLink className="h-4 w-4" />
          GitHub
        </Button>
      </div>

      <Separator className="w-64" />

      {/* License & Copyright */}
      <div className="flex flex-col items-center space-y-1 text-center text-sm text-muted-foreground">
        <p>{t('preferences.about.license')}: MIT License</p>
        <p>{t('preferences.about.copyright')}: © 2025 Sunshow</p>
      </div>

      <Separator className="w-64" />

      {/* Acknowledgements */}
      <div className="flex flex-col items-center space-y-2 text-center">
        <span className="text-sm font-medium text-foreground">
          {t('preferences.about.acknowledgements')}
        </span>
        <p className="text-sm text-muted-foreground">
          {t('preferences.about.templateCredit')}
        </p>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => handleOpenUrl(TEMPLATE_URL)}
          className="gap-2 text-xs"
        >
          <ExternalLink className="h-3 w-3" />
          tauri-template
        </Button>
      </div>

      <Separator className="w-64" />

      {/* Tech Stack */}
      <div className="flex flex-col items-center space-y-2">
        <span className="text-sm font-medium text-foreground">
          {t('preferences.about.techStack')}
        </span>
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <span>Tauri</span>
          <span>·</span>
          <span>React</span>
          <span>·</span>
          <span>Rust</span>
        </div>
      </div>
    </div>
  )
}
