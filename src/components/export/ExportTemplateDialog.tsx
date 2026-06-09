import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Plus, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from 'sonner'
import { useExportStore } from '@/store/export-store'
import type {
  ExportTemplate,
  ExportFormat,
  OutputStructure,
  ChannelType,
  ChannelFilter,
  TokenFilter,
} from '@/lib/bindings'

interface ExportTemplateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  template?: ExportTemplate
}

const channelTypes: ChannelType[] = [
  'new-api',
  'sub-2-api',
  'cli-proxy-api',
  'ollama',
  'general',
  'deep-seek',
]

// Available source fields for the "fields" selector
const AVAILABLE_SOURCE_FIELDS = [
  { value: 'channel.id', label: 'Channel ID' },
  { value: 'channel.name', label: 'Channel Name' },
  { value: 'channel.type', label: 'Channel Type' },
  { value: 'channel.baseUrl', label: 'Channel Base URL' },
  { value: 'channel.enabled', label: 'Channel Enabled' },
  { value: 'token.name', label: 'Token Name' },
  { value: 'token.key', label: 'Token Key (API Key)' },
  { value: 'token.status', label: 'Token Status' },
  { value: 'token.platform', label: 'Token Platform' },
  { value: 'token.groupName', label: 'Token Group' },
  { value: 'token.remainQuota', label: 'Remaining Quota' },
  { value: 'token.usedQuota', label: 'Used Quota' },
  { value: 'token.unlimitedQuota', label: 'Unlimited Quota' },
  { value: 'model.id', label: 'Model ID' },
  { value: 'model.name', label: 'Model Name' },
  { value: 'model.protocol', label: 'Inferred Protocol' },
]

function defaultTemplate(name: string, format?: ExportFormat): ExportTemplate {
  const fmt = format ?? 'yaml'
  const ext = fmt === 'json' ? 'json' : fmt === 'toml' ? 'toml' : 'yaml'
  return {
    name,
    description: '',
    channels: {
      types: [],
      enabledOnly: true,
      ids: [],
    },
    tokens: {
      status: 1,
      platforms: [],
    },
    fetchModels: true,
    modelProtocolOverrides: {},
    fields: {},
    format: fmt,
    outputStructure: 'flat',
    outputPath: `~/export-{timestamp}.${ext}`,
  }
}

export function ExportTemplateDialog({
  open,
  onOpenChange,
  template,
}: ExportTemplateDialogProps) {
  const { t } = useTranslation()
  const saveTemplate = useExportStore(state => state.saveTemplate)
  const isLoading = useExportStore(state => state.isLoading)

  const [form, setForm] = useState<ExportTemplate>(() =>
    template ? { ...template } : defaultTemplate('')
  )

  // New field entry
  const [newSourceField, setNewSourceField] = useState('channel.name')
  const [newOutputName, setNewOutputName] = useState('')

  // New protocol override
  const [newOverridePrefix, setNewOverridePrefix] = useState('')
  const [newOverrideProtocol, setNewOverrideProtocol] = useState('anthropic')

  const isEditing = !!template

  const updateField = <K extends keyof ExportTemplate>(
    key: K,
    value: ExportTemplate[K]
  ) => {
    setForm(prev => {
      const next = { ...prev, [key]: value }
      // Auto-update file extension when format changes
      if (key === 'format') {
        const ext =
          value === 'json' ? 'json' : value === 'toml' ? 'toml' : 'yaml'
        const extMatch = prev.outputPath.match(/\.(json|yaml|toml)$/i)
        const matchedExt = extMatch?.[1]
        if (matchedExt && matchedExt.toLowerCase() !== ext) {
          next.outputPath = prev.outputPath.replace(/\.[a-z0-9]+$/i, `.${ext}`)
        }
      }
      return next
    })
  }

  const updateChannels = (update: Partial<ChannelFilter>) => {
    setForm(prev => ({
      ...prev,
      channels: {
        ...(prev.channels ?? { types: [], enabledOnly: true, ids: [] }),
        ...update,
      },
    }))
  }

  const updateTokens = (update: Partial<TokenFilter>) => {
    setForm(prev => ({
      ...prev,
      tokens: { ...(prev.tokens ?? { status: 1, platforms: [] }), ...update },
    }))
  }

  const addField = () => {
    if (!newSourceField || !newOutputName) return
    setForm(prev => ({
      ...prev,
      fields: { ...(prev.fields ?? {}), [newSourceField]: newOutputName },
    }))
    setNewSourceField('channel.name')
    setNewOutputName('')
  }

  const removeField = (source: string) => {
    setForm(prev => ({
      ...prev,
      fields: Object.fromEntries(
        Object.entries(prev.fields ?? {}).filter(([k]) => k !== source)
      ),
    }))
  }

  const addOverride = () => {
    if (!newOverridePrefix || !newOverrideProtocol) return
    const pattern = newOverridePrefix.endsWith('*')
      ? newOverridePrefix
      : `${newOverridePrefix}*`
    setForm(prev => ({
      ...prev,
      modelProtocolOverrides: {
        ...(prev.modelProtocolOverrides ?? {}),
        [pattern]: newOverrideProtocol,
      },
    }))
    setNewOverridePrefix('')
    setNewOverrideProtocol('anthropic')
  }

  const removeOverride = (pattern: string) => {
    setForm(prev => ({
      ...prev,
      modelProtocolOverrides: Object.fromEntries(
        Object.entries(prev.modelProtocolOverrides ?? {}).filter(
          ([k]) => k !== pattern
        )
      ),
    }))
  }

  const toggleChannelType = (type: ChannelType) => {
    const types = form.channels?.types ?? []
    updateChannels({
      types: types.includes(type)
        ? types.filter(t => t !== type)
        : [...types, type],
    })
  }

  const handleSave = async () => {
    if (!form.name.trim()) {
      toast.error(t('export.nameRequired'))
      return
    }
    if (!form.outputPath.trim()) {
      toast.error(t('export.pathRequired'))
      return
    }

    // If editing but name changed, we need to delete old first
    const finalTemplate = { ...form }

    await saveTemplate(finalTemplate)
    if (!useExportStore.getState().error) {
      toast.success(
        isEditing ? t('export.templateUpdated') : t('export.templateCreated')
      )
      onOpenChange(false)
    }
  }

  const channels = form.channels ?? { types: [], enabledOnly: true, ids: [] }
  const tokens = form.tokens ?? { status: 1, platforms: [] }
  const fields = form.fields ?? {}
  const overrides = form.modelProtocolOverrides ?? {}

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        key={isEditing ? template.name : 'new'}
        className="max-w-2xl max-h-[85vh] overflow-y-auto"
      >
        <DialogHeader>
          <DialogTitle>
            {isEditing ? t('export.editTemplate') : t('export.addTemplate')}
          </DialogTitle>
        </DialogHeader>

        <div className="grid gap-6 py-4">
          {/* ——— Basic Info ——— */}
          <div className="grid gap-3">
            <h3 className="text-sm font-medium text-muted-foreground">
              {t('export.basicInfo')}
            </h3>

            <div className="grid gap-2">
              <Label>{t('export.templateName')}</Label>
              <Input
                value={form.name}
                onChange={e => updateField('name', e.target.value)}
                placeholder="my-export-template"
              />
            </div>

            <div className="grid gap-2">
              <Label>{t('export.description')}</Label>
              <Input
                value={form.description ?? ''}
                onChange={e => updateField('description', e.target.value)}
                placeholder={t('export.descriptionPlaceholder')}
              />
            </div>
          </div>

          {/* ——— Filters ——— */}
          <div className="grid gap-3">
            <h3 className="text-sm font-medium text-muted-foreground">
              {t('export.filters')}
            </h3>

            {/* Channel filter */}
            <div className="grid gap-2">
              <Label>{t('export.channelTypes')}</Label>
              <div className="flex flex-wrap gap-2">
                {channelTypes.map(type => (
                  <label
                    key={type}
                    className="flex items-center gap-1.5 text-sm cursor-pointer"
                  >
                    <Checkbox
                      checked={(channels.types ?? []).includes(type)}
                      onCheckedChange={() => toggleChannelType(type)}
                    />
                    {type}
                  </label>
                ))}
              </div>
              <p className="text-xs text-muted-foreground">
                {t('export.channelTypesHint')}
              </p>
            </div>

            <div className="flex items-center gap-2">
              <Switch
                id="enabled-only"
                checked={channels.enabledOnly ?? true}
                onCheckedChange={checked =>
                  updateChannels({ enabledOnly: checked })
                }
              />
              <Label htmlFor="enabled-only">{t('export.enabledOnly')}</Label>
            </div>

            {/* Token filter */}
            <div className="grid gap-2">
              <Label>{t('export.tokenStatus')}</Label>
              <Select
                value={
                  tokens.status !== null && tokens.status !== undefined
                    ? String(tokens.status)
                    : 'all'
                }
                onValueChange={v =>
                  updateTokens({
                    status: v === 'all' ? null : parseInt(v),
                  })
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">{t('export.allStatuses')}</SelectItem>
                  <SelectItem value="1">{t('keys.status.enabled')}</SelectItem>
                  <SelectItem value="2">{t('keys.status.disabled')}</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="grid gap-2">
              <Label>{t('export.tokenPlatforms')}</Label>
              <Input
                value={(tokens.platforms ?? []).join(', ')}
                onChange={e =>
                  updateTokens({
                    platforms: e.target.value
                      .split(',')
                      .map(s => s.trim())
                      .filter(Boolean),
                  })
                }
                placeholder="anthropic, openai, gemini"
              />
              <p className="text-xs text-muted-foreground">
                {t('export.tokenPlatformsHint')}
              </p>
            </div>

            {/* Fetch models */}
            <div className="flex items-center gap-2">
              <Switch
                id="fetch-models"
                checked={form.fetchModels}
                onCheckedChange={checked => updateField('fetchModels', checked)}
              />
              <Label htmlFor="fetch-models">{t('export.fetchModels')}</Label>
            </div>
          </div>

          {/* ——— Protocol Overrides ——— */}
          <div className="grid gap-3">
            <h3 className="text-sm font-medium text-muted-foreground">
              {t('export.protocolOverrides')}
            </h3>

            {Object.entries(overrides).map(([pattern, protocol]) => (
              <div key={pattern} className="flex items-center gap-2 text-sm">
                <code className="px-2 py-0.5 bg-muted rounded text-xs">
                  {pattern}
                </code>
                <span>→</span>
                <code className="px-2 py-0.5 bg-muted rounded text-xs">
                  {protocol}
                </code>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6 ml-auto"
                  onClick={() => removeOverride(pattern)}
                >
                  <X className="h-3 w-3" />
                </Button>
              </div>
            ))}

            <div className="flex items-center gap-2">
              <Input
                value={newOverridePrefix}
                onChange={e => setNewOverridePrefix(e.target.value)}
                placeholder="model prefix (e.g. claude-)"
                className="flex-1"
              />
              <Select
                value={newOverrideProtocol}
                onValueChange={setNewOverrideProtocol}
              >
                <SelectTrigger className="w-[180px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="anthropic">anthropic</SelectItem>
                  <SelectItem value="openai">openai</SelectItem>
                  <SelectItem value="google-ai">google-ai</SelectItem>
                  <SelectItem value="openai-compatible">
                    openai-compatible
                  </SelectItem>
                </SelectContent>
              </Select>
              <Button
                variant="outline"
                size="icon"
                onClick={addOverride}
                disabled={!newOverridePrefix}
              >
                <Plus className="h-4 w-4" />
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              {t('export.protocolOverridesHint')}
            </p>
          </div>

          {/* ——— Field Selectors ("填表" core) ——— */}
          <div className="grid gap-3">
            <h3 className="text-sm font-medium text-muted-foreground">
              {t('export.fieldSelectors')}
            </h3>
            <p className="text-xs text-muted-foreground">
              {t('export.fieldSelectorsHint')}
            </p>

            {Object.entries(fields).length > 0 && (
              <div className="border rounded-md divide-y">
                {Object.entries(fields).map(([source, outputName]) => (
                  <div
                    key={source}
                    className="flex items-center gap-2 px-3 py-2 text-sm"
                  >
                    <code className="text-xs flex-1">{source}</code>
                    <span className="text-muted-foreground">→</span>
                    <code className="text-xs flex-1">{outputName}</code>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-6 w-6"
                      onClick={() => removeField(source)}
                    >
                      <X className="h-3 w-3" />
                    </Button>
                  </div>
                ))}
              </div>
            )}

            <div className="flex items-center gap-2">
              <Select value={newSourceField} onValueChange={setNewSourceField}>
                <SelectTrigger className="flex-1">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {AVAILABLE_SOURCE_FIELDS.map(f => (
                    <SelectItem key={f.value} value={f.value}>
                      {f.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Input
                value={newOutputName}
                onChange={e => setNewOutputName(e.target.value)}
                placeholder="output name"
                className="w-40"
              />
              <Button
                variant="outline"
                size="icon"
                onClick={addField}
                disabled={!newOutputName}
              >
                <Plus className="h-4 w-4" />
              </Button>
            </div>
          </div>

          {/* ——— Output ——— */}
          <div className="grid gap-3">
            <h3 className="text-sm font-medium text-muted-foreground">
              {t('export.outputSettings')}
            </h3>

            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-2">
                <Label>{t('export.format')}</Label>
                <Select
                  value={form.format}
                  onValueChange={v => updateField('format', v as ExportFormat)}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="json">JSON</SelectItem>
                    <SelectItem value="yaml">YAML</SelectItem>
                    <SelectItem value="toml">TOML</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="grid gap-2">
                <Label>{t('export.outputStructure')}</Label>
                <Select
                  value={form.outputStructure}
                  onValueChange={v =>
                    updateField('outputStructure', v as OutputStructure)
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="flat">{t('export.flat')}</SelectItem>
                    <SelectItem value="nested">{t('export.nested')}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="grid gap-2">
              <Label>{t('export.outputPath')}</Label>
              <Input
                value={form.outputPath}
                onChange={e => updateField('outputPath', e.target.value)}
                placeholder="~/export-{timestamp}.yaml"
              />
              <p className="text-xs text-muted-foreground">
                {t('export.outputPathHint')}
              </p>
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleSave} disabled={isLoading}>
            {isEditing ? t('common.save') : t('common.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
