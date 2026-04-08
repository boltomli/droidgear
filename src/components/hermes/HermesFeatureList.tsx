import { useTranslation } from 'react-i18next'
import { Cpu } from 'lucide-react'
import { cn } from '@/lib/utils'
import { ActionButton } from '@/components/ui/action-button'

interface FeatureItem {
  id: 'model'
  labelKey: string
  icon: React.ElementType
}

const features: FeatureItem[] = [
  { id: 'model', labelKey: 'hermes.features.model', icon: Cpu },
]

export function HermesFeatureList() {
  const { t } = useTranslation()

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

      <div className="mt-auto p-3 border-t text-xs text-muted-foreground">
        {t('hermes.features.hint')}
      </div>
    </div>
  )
}
