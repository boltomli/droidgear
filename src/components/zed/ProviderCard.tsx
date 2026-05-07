import { useTranslation } from 'react-i18next'
import { Pencil, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import type { ZedProviderConfig } from '@/lib/bindings'

interface ProviderCardProps {
  providerId: string
  config: ZedProviderConfig
  onEdit: () => void
  onDelete: () => void
}

export function ProviderCard({
  providerId,
  config,
  onEdit,
  onDelete,
}: ProviderCardProps) {
  const { t } = useTranslation()

  const modelCount = config.availableModels?.length ?? 0

  return (
    <div className="flex items-center justify-between p-3 border rounded-lg hover:bg-muted/50 transition-colors">
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="font-medium">{providerId}</span>
        </div>
        <div className="text-sm text-muted-foreground mt-1 space-y-0.5">
          {config.api_url && (
            <div className="truncate text-xs" title={config.api_url}>
              {config.api_url}
            </div>
          )}
          {config.apiKey && (
            <div className="text-xs text-muted-foreground">
              {t('zed.provider.apiKey')} ⚫⚫⚫
            </div>
          )}
        </div>
        <div className="flex items-center gap-2 mt-2">
          <Badge variant="outline" className="text-xs">
            {modelCount} {t('zed.provider.modelsCount', { count: modelCount })}
          </Badge>
        </div>
      </div>
      <div className="flex items-center gap-1 ml-2">
        <Button
          variant="ghost"
          size="icon"
          onClick={onEdit}
          title={t('common.edit')}
        >
          <Pencil className="h-4 w-4" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          onClick={onDelete}
          title={t('common.delete')}
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>
    </div>
  )
}
