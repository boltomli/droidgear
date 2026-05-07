import { useTranslation } from 'react-i18next'
import { CheckCircle, XCircle, FileText, UserCheck } from 'lucide-react'
import type { ZedConfigStatus as ZedConfigStatusType } from '@/lib/bindings'

interface ConfigStatusProps {
  status: ZedConfigStatusType | null
  isActiveProfile?: boolean
}

export function ConfigStatus({ status, isActiveProfile }: ConfigStatusProps) {
  const { t } = useTranslation()

  if (!status) return null

  return (
    <div className="p-4 border rounded-lg space-y-2">
      <h3 className="text-sm font-medium text-muted-foreground">
        {t('zed.configStatus.title')}
      </h3>
      <div className="space-y-1 text-sm">
        <div className="flex items-center gap-2">
          <FileText className="h-4 w-4 text-muted-foreground shrink-0" />
          <code className="flex-1 truncate text-xs bg-muted px-1 py-0.5 rounded select-all cursor-text">
            {status.configPath}
          </code>
          {status.configExists ? (
            <>
              <CheckCircle className="h-4 w-4 text-green-500 shrink-0" />
              <span className="text-xs text-green-600 shrink-0">
                {t('common.exists')}
              </span>
            </>
          ) : (
            <>
              <XCircle className="h-4 w-4 text-muted-foreground shrink-0" />
              <span className="text-xs text-muted-foreground shrink-0">
                {t('common.missing')}
              </span>
            </>
          )}
        </div>

        <div className="flex items-center gap-2">
          <FileText className="h-4 w-4 text-muted-foreground shrink-0" />
          <span className="flex-1 text-xs">
            {t('zed.configStatus.hasOpenaiCompatible')}
          </span>
          {status.hasOpenaiCompatible ? (
            <CheckCircle className="h-4 w-4 text-green-500 shrink-0" />
          ) : (
            <XCircle className="h-4 w-4 text-muted-foreground shrink-0" />
          )}
        </div>

        {isActiveProfile !== undefined && (
          <div className="flex items-center gap-2">
            <UserCheck className="h-4 w-4 text-muted-foreground shrink-0" />
            <span className="flex-1 text-xs">
              {t('zed.configStatus.activeProfile')}
            </span>
            {isActiveProfile ? (
              <CheckCircle className="h-4 w-4 text-green-500 shrink-0" />
            ) : (
              <XCircle className="h-4 w-4 text-muted-foreground shrink-0" />
            )}
          </div>
        )}
      </div>
    </div>
  )
}
