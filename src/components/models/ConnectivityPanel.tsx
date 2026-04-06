import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  X,
  Wifi,
  AlertCircle,
  CheckCircle,
  Clock,
  AlertTriangle,
  MessageSquare,
} from 'lucide-react'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { useConnectivityStore } from '@/store/connectivity-store'
import { useModelStore } from '@/store/model-store'
import type { ModelTestResult, TestMode } from '@/lib/bindings'

interface ConnectivityPanelProps {
  onTestAll: () => Promise<void>
  onClose: () => void
  isLoading?: boolean
}

export function ConnectivityPanel({
  onTestAll,
  onClose,
  isLoading = false,
}: ConnectivityPanelProps) {
  const { t } = useTranslation()
  const testResults = useConnectivityStore(state => state.testResults)
  const testMode = useConnectivityStore(state => state.testMode)
  const customPrompt = useConnectivityStore(state => state.customPrompt)
  const { setTestMode, setCustomPrompt } = useConnectivityStore.getState()
  const models = useModelStore(state => state.models)
  const [isTesting, setIsTesting] = useState(false)

  const handleTestAll = async () => {
    setIsTesting(true)
    try {
      await onTestAll()
    } finally {
      setIsTesting(false)
    }
  }

  const total = testResults.length
  const available = testResults.filter(r => r.isAvailable).length
  const unavailable = total - available

  const getStatusIcon = (success: boolean) => {
    return success ? (
      <CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400" />
    ) : (
      <AlertCircle className="h-4 w-4 text-red-600 dark:text-red-400" />
    )
  }

  const getStatusColor = (success: boolean) =>
    success
      ? 'text-green-600 dark:text-green-400'
      : 'text-red-600 dark:text-red-400'

  const getStatusText = (success: boolean) =>
    success ? t('connectivity.connected') : t('connectivity.disconnected')

  const handleModeChange = (mode: TestMode) => {
    setTestMode(mode)
  }

  const renderModelStatus = (result: ModelTestResult) => {
    const { diagnostics } = result

    return (
      <div key={result.modelId} className="py-2">
        {/* Model name line */}
        <div className="flex items-center gap-2 min-w-0">
          {getStatusIcon(diagnostics.success)}
          <span className="font-medium text-sm truncate">
            {result.modelName}
          </span>
        </div>
        {/* Test result line */}
        <div className="flex items-center gap-2 mt-1 ml-6">
          {diagnostics.testMode === 'inference' ? (
            <MessageSquare className="h-3 w-3 text-muted-foreground flex-shrink-0" />
          ) : (
            <Wifi className="h-3 w-3 text-muted-foreground flex-shrink-0" />
          )}
          <span className={`text-xs ${getStatusColor(diagnostics.success)}`}>
            {getStatusText(diagnostics.success)}
          </span>
          {diagnostics.success && (
            <span className="text-xs text-muted-foreground">
              {diagnostics.latencyMs}ms
            </span>
          )}
        </div>
        {diagnostics.error && (
          <div className="mt-1 ml-6 text-xs text-red-600 dark:text-red-400 break-words">
            {diagnostics.error}
          </div>
        )}
        {diagnostics.responseText && (
          <div className="mt-1 ml-6 text-xs text-muted-foreground break-words bg-muted/50 rounded px-2 py-1">
            <span className="font-medium">
              {t('connectivity.responseText')}:{' '}
            </span>
            {diagnostics.responseText}
          </div>
        )}
      </div>
    )
  }

  const renderSummary = () => {
    if (total === 0) {
      return (
        <div className="text-center py-4 text-muted-foreground">
          <p>{t('connectivity.noTestResults')}</p>
          <p className="text-xs mt-1">{t('connectivity.runTestHint')}</p>
        </div>
      )
    }

    const pct = (available / total) * 100

    return (
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-4">
          <div className="text-center">
            <div className="text-2xl font-bold text-green-600 dark:text-green-400">
              {available}
            </div>
            <div className="text-xs text-muted-foreground">
              {t('connectivity.available')}
            </div>
          </div>
          <div className="text-center">
            <div className="text-2xl font-bold text-red-600 dark:text-red-400">
              {unavailable}
            </div>
            <div className="text-xs text-muted-foreground">
              {t('connectivity.unavailable')}
            </div>
          </div>
        </div>

        <div className="space-y-2">
          <div className="flex items-center justify-between text-sm">
            <span>{t('connectivity.availability')}</span>
            <span className="font-medium">{pct.toFixed(1)}%</span>
          </div>
          <div className="h-2 bg-muted rounded-full overflow-hidden">
            <div
              className="h-full bg-primary rounded-full transition-all"
              style={{ width: `${pct}%` }}
            />
          </div>
        </div>
      </div>
    )
  }

  return (
    <Card className="h-full flex flex-col border-0 rounded-none">
      <div className="flex items-center justify-between p-4 border-b">
        <div className="flex items-center gap-2">
          <Wifi className="h-5 w-5" />
          <h2 className="text-lg font-semibold">{t('connectivity.title')}</h2>
        </div>
        <Button variant="ghost" size="icon" onClick={onClose}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      <div className="flex-1 overflow-auto p-4 space-y-6">
        <div>
          <h3 className="text-sm font-medium mb-3">
            {t('connectivity.summary')}
          </h3>
          {renderSummary()}
        </div>

        <Separator />

        <div className="space-y-3">
          <h3 className="text-sm font-medium">
            {t('connectivity.testControls')}
          </h3>

          {/* Test Mode Selector */}
          <div className="space-y-2">
            <Label className="text-xs text-muted-foreground">
              {t('connectivity.mode')}
            </Label>
            <div className="flex gap-1 bg-muted rounded-md p-1">
              <button
                className={`flex-1 text-xs py-1.5 px-2 rounded transition-colors ${
                  testMode === 'ping'
                    ? 'bg-background shadow-sm font-medium'
                    : 'hover:bg-background/50'
                }`}
                onClick={() => handleModeChange('ping')}
              >
                {t('connectivity.modePing')}
              </button>
              <button
                className={`flex-1 text-xs py-1.5 px-2 rounded transition-colors ${
                  testMode === 'inference'
                    ? 'bg-background shadow-sm font-medium'
                    : 'hover:bg-background/50'
                }`}
                onClick={() => handleModeChange('inference')}
              >
                {t('connectivity.modeInference')}
              </button>
            </div>
          </div>

          {/* Custom Prompt (inference mode only) */}
          {testMode === 'inference' && (
            <div className="space-y-2">
              <Label htmlFor="custom-prompt" className="text-xs">
                {t('connectivity.customPrompt')}
              </Label>
              <Input
                id="custom-prompt"
                value={customPrompt}
                onChange={e => setCustomPrompt(e.target.value)}
                placeholder={t('connectivity.defaultPrompt')}
                className="text-xs"
              />
              <div className="flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
                <AlertTriangle className="h-3 w-3" />
                <span>{t('connectivity.inferenceWarning')}</span>
              </div>
            </div>
          )}

          <Button
            onClick={handleTestAll}
            disabled={isLoading || isTesting || models.length === 0}
            className="w-full"
          >
            {isTesting ? (
              <>
                <Clock className="h-4 w-4 mr-2 animate-spin" />
                {t('connectivity.testing')}
              </>
            ) : (
              <>
                <Wifi className="h-4 w-4 mr-2" />
                {t('connectivity.testAll')}
              </>
            )}
          </Button>
        </div>

        <Separator />

        <div>
          <h3 className="text-sm font-medium mb-3">
            {t('connectivity.detailedResults')} ({total})
          </h3>
          <div className="space-y-1">
            {total === 0 ? (
              <div className="text-center py-4 text-muted-foreground">
                <p>{t('connectivity.noTestResults')}</p>
              </div>
            ) : (
              testResults.map(renderModelStatus)
            )}
          </div>
        </div>
      </div>

      <div className="p-4 border-t bg-muted/50">
        <div className="text-xs text-muted-foreground">
          {t('connectivity.footerHint')}
        </div>
      </div>
    </Card>
  )
}
