import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { ExternalLink } from 'lucide-react'
import { toast } from 'sonner'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { commands } from '@/lib/bindings'
import {
  type MissionModelSettings,
  type MissionModelSettings_Deserialize,
  type CustomModel,
} from '@/lib/bindings'

const REASONING_EFFORT_OPTIONS = ['none', 'low', 'medium', 'high'] as const
const NOT_SET_VALUE = '__not_set__'

export function MissionsPage() {
  const { t } = useTranslation()

  const [settings, setSettings] = useState<MissionModelSettings>({
    workerModel: null,
    workerReasoningEffort: null,
    validationWorkerModel: null,
    validationWorkerReasoningEffort: null,
  })
  const [customModels, setCustomModels] = useState<CustomModel[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    const fetchData = async () => {
      const [settingsResult, modelsResult] = await Promise.all([
        commands.getMissionModelSettings(),
        commands.loadCustomModels(),
      ])

      if (cancelled) return

      if (settingsResult.status === 'ok') {
        setSettings(settingsResult.data)
      }
      if (modelsResult.status === 'ok') {
        setCustomModels(modelsResult.data)
      }
      setLoading(false)
    }
    fetchData()
    return () => {
      cancelled = true
    }
  }, [])

  const saveSettings = async (updated: MissionModelSettings) => {
    const oldSettings = settings
    setSettings(updated)
    const result = await commands.saveMissionModelSettings(updated)
    if (result.status === 'error') {
      setSettings(oldSettings)
      toast.error(t('toast.error.generic'))
    }
  }

  const handleWorkerModelChange = (value: string) => {
    saveSettings({
      ...settings,
      workerModel: value === NOT_SET_VALUE ? null : value,
    } as MissionModelSettings_Deserialize)
  }

  const handleWorkerReasoningEffortChange = (value: string) => {
    saveSettings({
      ...settings,
      workerReasoningEffort: value === NOT_SET_VALUE ? null : value,
    } as MissionModelSettings_Deserialize)
  }

  const handleValidationWorkerModelChange = (value: string) => {
    saveSettings({
      ...settings,
      validationWorkerModel: value === NOT_SET_VALUE ? null : value,
    } as MissionModelSettings_Deserialize)
  }

  const handleValidationWorkerReasoningEffortChange = (value: string) => {
    saveSettings({
      ...settings,
      validationWorkerReasoningEffort: value === NOT_SET_VALUE ? null : value,
    } as MissionModelSettings_Deserialize)
  }

  const getModelDisplayLabel = (model: CustomModel) => {
    const id = model.id ?? model.model
    if (model.displayName) {
      return `${model.displayName} (${id})`
    }
    return id
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground">
        <p>{t('common.loading')}</p>
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between p-4 border-b">
        <h1 className="text-xl font-semibold">{t('droid.missions.title')}</h1>
      </div>

      <div className="flex-1 overflow-y-auto p-4">
        <div className="space-y-6">
          <p className="text-sm text-muted-foreground">
            {t('droid.missions.description')}
          </p>

          {/* Worker Model */}
          <div className="space-y-4 pt-2 border-t">
            <h2 className="text-base font-medium">
              {t('droid.missions.workerSection')}
            </h2>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <div className="flex flex-col gap-1">
                  <Label htmlFor="worker-model" className="text-sm font-medium">
                    {t('droid.missions.workerModel')}
                  </Label>
                  <p className="text-sm text-muted-foreground">
                    {t('droid.missions.workerModelDescription')}
                  </p>
                </div>
                <Select
                  value={settings.workerModel ?? NOT_SET_VALUE}
                  onValueChange={handleWorkerModelChange}
                >
                  <SelectTrigger className="w-64">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NOT_SET_VALUE}>
                      {t('droid.missions.modelPlaceholder')}
                    </SelectItem>
                    {customModels.map(model => {
                      const modelId = model.id ?? model.model
                      return (
                        <SelectItem key={modelId} value={modelId}>
                          {getModelDisplayLabel(model)}
                        </SelectItem>
                      )
                    })}
                  </SelectContent>
                </Select>
              </div>
            </div>

            {/* Worker Reasoning Effort */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <div className="flex flex-col gap-1">
                  <Label
                    htmlFor="worker-reasoning-effort"
                    className="text-sm font-medium"
                  >
                    {t('droid.missions.workerReasoningEffort')}
                  </Label>
                  <p className="text-sm text-muted-foreground">
                    {t('droid.missions.workerReasoningEffortDescription')}
                  </p>
                </div>
                <Select
                  value={settings.workerReasoningEffort ?? NOT_SET_VALUE}
                  onValueChange={handleWorkerReasoningEffortChange}
                >
                  <SelectTrigger className="w-40">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NOT_SET_VALUE}>
                      {t('droid.missions.modelPlaceholder')}
                    </SelectItem>
                    {REASONING_EFFORT_OPTIONS.map(option => (
                      <SelectItem key={option} value={option}>
                        {t(`droid.missions.reasoningEffort.${option}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          </div>

          {/* Validation Worker Section */}
          <div className="space-y-4 pt-2 border-t">
            <h2 className="text-base font-medium">
              {t('droid.missions.validationWorkerSection')}
            </h2>

            {/* Validation Worker Model */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <div className="flex flex-col gap-1">
                  <Label
                    htmlFor="validation-worker-model"
                    className="text-sm font-medium"
                  >
                    {t('droid.missions.validationWorkerModel')}
                  </Label>
                  <p className="text-sm text-muted-foreground">
                    {t('droid.missions.validationWorkerModelDescription')}
                  </p>
                </div>
                <Select
                  value={settings.validationWorkerModel ?? NOT_SET_VALUE}
                  onValueChange={handleValidationWorkerModelChange}
                >
                  <SelectTrigger className="w-64">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NOT_SET_VALUE}>
                      {t('droid.missions.modelPlaceholder')}
                    </SelectItem>
                    {customModels.map(model => {
                      const modelId = model.id ?? model.model
                      return (
                        <SelectItem key={modelId} value={modelId}>
                          {getModelDisplayLabel(model)}
                        </SelectItem>
                      )
                    })}
                  </SelectContent>
                </Select>
              </div>
            </div>

            {/* Validation Worker Reasoning Effort */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <div className="flex flex-col gap-1">
                  <Label
                    htmlFor="validation-worker-reasoning-effort"
                    className="text-sm font-medium"
                  >
                    {t('droid.missions.validationWorkerReasoningEffort')}
                  </Label>
                  <p className="text-sm text-muted-foreground">
                    {t(
                      'droid.missions.validationWorkerReasoningEffortDescription'
                    )}
                  </p>
                </div>
                <Select
                  value={
                    settings.validationWorkerReasoningEffort ?? NOT_SET_VALUE
                  }
                  onValueChange={handleValidationWorkerReasoningEffortChange}
                >
                  <SelectTrigger className="w-40">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NOT_SET_VALUE}>
                      {t('droid.missions.modelPlaceholder')}
                    </SelectItem>
                    {REASONING_EFFORT_OPTIONS.map(option => (
                      <SelectItem key={option} value={option}>
                        {t(`droid.missions.reasoningEffort.${option}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          </div>

          {/* Learn More Link */}
          <div className="pt-2">
            <a
              href="https://docs.factory.ai/cli/features/missions"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 text-sm text-primary hover:underline"
            >
              <ExternalLink className="h-3.5 w-3.5" />
              {t('droid.missions.learnMore')}
            </a>
          </div>
        </div>
      </div>
    </div>
  )
}
