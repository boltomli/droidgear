import { useTranslation } from 'react-i18next'
import { Server } from 'lucide-react'
import { writeText } from '@tauri-apps/plugin-clipboard-manager'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { ActionButton } from '@/components/ui/action-button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useIsWindows } from '@/hooks/use-platform'

interface FeatureItem {
  id: string
  labelKey: string
  icon: React.ElementType
}

const features: FeatureItem[] = [
  { id: 'providers', labelKey: 'zed.features.providers', icon: Server },
]

export function ZedFeatureList() {
  const { t } = useTranslation()
  const isWindows = useIsWindows()

  const handleCopyCommand = async (command: string) => {
    await writeText(command)
    toast.success(t('common.copied'))
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-col gap-1 p-2">
        {features.map(feature => (
          <ActionButton
            key={feature.id}
            variant="secondary"
            size="sm"
            className={cn('justify-start w-full')}
          >
            <feature.icon className="h-4 w-4 mr-2" />
            {t(feature.labelKey)}
          </ActionButton>
        ))}
      </div>

      {/* Install Section */}
      <div className="mt-auto p-3 border-t text-xs text-muted-foreground">
        <div className="font-medium mb-2">{t('zed.install.title')}</div>
        <Tabs defaultValue={isWindows ? 'windows' : 'unix'} className="w-full">
          <TabsList className="w-full">
            <TabsTrigger value="unix" className="flex-1">
              {t('zed.install.macosLinux')}
            </TabsTrigger>
            <TabsTrigger value="windows" className="flex-1">
              {t('zed.install.windows')}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="unix">
            <code
              className="block bg-muted p-2 rounded text-xs break-all cursor-pointer hover:bg-muted/80 transition-colors"
              onClick={() =>
                handleCopyCommand(t('zed.install.macosLinuxCommand'))
              }
            >
              {t('zed.install.macosLinuxCommand')}
            </code>
          </TabsContent>
          <TabsContent value="windows">
            <code
              className="block bg-muted p-2 rounded text-xs break-all cursor-pointer hover:bg-muted/80 transition-colors"
              onClick={() => handleCopyCommand(t('zed.install.windowsCommand'))}
            >
              {t('zed.install.windowsCommand')}
            </code>
          </TabsContent>
        </Tabs>
        <a
          href="https://zed.dev"
          target="_blank"
          rel="noopener noreferrer"
          className="text-primary hover:underline mt-2 inline-block"
        >
          {t('zed.install.learnMore')}
        </a>
      </div>
    </div>
  )
}
