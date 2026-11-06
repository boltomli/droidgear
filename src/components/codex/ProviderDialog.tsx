import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FolderInput } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { SecretInput } from '@/components/ui/secret-input'
import { Label } from '@/components/ui/label'
import {
  ResizableDialog,
  ResizableDialogContent,
  ResizableDialogDescription,
  ResizableDialogFooter,
  ResizableDialogHeader,
  ResizableDialogBody,
  ResizableDialogTitle,
} from '@/components/ui/resizable-dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useCodexStore } from '@/store/codex-store'
import type {
  CodexProviderConfig,
  CodexProfile,
  CustomModel,
} from '@/lib/bindings'
import { ChannelModelPickerDialog } from '@/components/channels/ChannelModelPickerDialog'
import type { ChannelProviderContext } from '@/components/channels'
import { inferModelProtocol } from '@/lib/model-protocol'
import {
  ensureOpenAICompatibleV1,
  isMultiProtocolPlatform,
  needsOpenAICompatibleV1,
} from '@/lib/sub2api-platform'
import { trimToNull } from '@/lib/utils'
import { formatKeyValueLines, parseKeyValueLines } from '@/lib/key-value'
import {
  clampEffortToSupported,
  getSupportedEfforts,
} from '@/lib/model-registry'

const WIRE_API_OPTIONS: {
  value: string
  label: string
  disabled?: boolean
}[] = [
  // Codex removed the chat/completions wire format in Feb 2026 —
  // `wire_api = "chat"` now fails at config load — so the option stays
  // visible (disabled) only to surface it on profiles that still use it.
  { value: 'chat', label: 'Chat Completions (removed)', disabled: true },
  { value: 'responses', label: 'Responses API' },
]

/**
 * Quick-select presets for the model context window (tokens). Picking a
 * preset also selects the linked auto-compact preset; the user can still
 * re-pick the auto-compact tier afterwards.
 */
const CONTEXT_WINDOW_PRESETS = [
  { value: '272000', label: '272K (272,000)', autoCompact: '250000' },
  { value: '1000000', label: '1M (1,000,000)', autoCompact: '900000' },
]

/** Quick-select presets for the auto-compact token limit. */
const AUTO_COMPACT_PRESETS = [
  { value: '250000', label: '250K (250,000)' },
  { value: '900000', label: '900K (900,000)' },
]

/** Sentinel for "not set" (config keys are omitted). */
const UNSET_VALUE = '__unset__'
/** Sentinel for a free-form numeric value. */
const CUSTOM_VALUE = '__custom__'

function tokenLimitSelectValue(
  raw: string,
  presets: { value: string }[]
): string {
  if (!raw) return UNSET_VALUE
  return presets.some(preset => preset.value === raw) ? raw : CUSTOM_VALUE
}

function parseTokenLimit(raw: string): number | null {
  const trimmed = raw.trim()
  if (!trimmed) return null
  const value = Number(trimmed)
  return Number.isFinite(value) && value > 0 ? Math.floor(value) : null
}

function sanitizeTokenLimitInput(raw: string): string {
  return raw.replace(/\D+/g, '')
}

/** Codex-specific fallback: includes `minimal`, which is not in the shared registry type. */
const CODEX_FALLBACK_EFFORTS = [
  'none',
  'minimal',
  'low',
  'medium',
  'high',
  'xhigh',
  'max',
] as const

const EFFORT_LABEL_KEYS: Record<string, string> = {
  none: 'models.reasoningEffort.none',
  low: 'models.reasoningEffort.low',
  medium: 'models.reasoningEffort.medium',
  high: 'models.reasoningEffort.high',
  xhigh: 'models.reasoningEffort.xhigh',
  max: 'models.reasoningEffort.max',
  minimal: 'codex.provider.reasoningEffort.minimal',
}

function effortsForModel(modelId: string): string[] {
  const trimmed = modelId.trim()
  if (!trimmed) return [...CODEX_FALLBACK_EFFORTS]
  return getSupportedEfforts(trimmed, 'openai') ?? [...CODEX_FALLBACK_EFFORTS]
}

function toStoredEffort(effort: string): string {
  return effort === 'none' ? '' : effort
}

const formatHeaderLines = formatKeyValueLines
const parseHeaderLines = parseKeyValueLines

/** Optional non-negative integer from a text input. */
function parseOptionalUint(text: string): number | null {
  const trimmed = text.trim()
  if (!trimmed) return null
  const value = Number(trimmed)
  return Number.isFinite(value) && value >= 0 ? Math.floor(value) : null
}

/** Optional string from a text input. */
function trimToNullValue(text: string): string | null {
  const trimmed = text.trim()
  return trimmed ? trimmed : null
}

interface ProviderDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  editingProviderId: string | null
}

interface ProviderFormProps {
  editingProviderId: string | null
  currentProfile: CodexProfile | null
  onClose: () => void
}

function ProviderForm({
  editingProviderId,
  currentProfile,
  onClose,
}: ProviderFormProps) {
  const { t } = useTranslation()
  const addProvider = useCodexStore(state => state.addProvider)
  const updateProvider = useCodexStore(state => state.updateProvider)

  const isEditing = editingProviderId !== null
  const existingConfig = editingProviderId
    ? (
        (currentProfile?.providers ?? {}) as Record<string, CodexProviderConfig>
      )[editingProviderId]
    : null

  const [providerId, setProviderId] = useState(editingProviderId ?? '')
  const [name, setName] = useState(existingConfig?.name ?? '')
  const [baseUrl, setBaseUrl] = useState(existingConfig?.baseUrl ?? '')
  const [wireApi, setWireApi] = useState(existingConfig?.wireApi ?? 'responses')
  const [model, setModel] = useState(existingConfig?.model ?? '')
  const [modelContextWindow, setModelContextWindow] = useState(
    existingConfig?.modelContextWindow != null
      ? String(existingConfig.modelContextWindow)
      : ''
  )
  const [modelAutoCompactTokenLimit, setModelAutoCompactTokenLimit] = useState(
    existingConfig?.modelAutoCompactTokenLimit != null
      ? String(existingConfig.modelAutoCompactTokenLimit)
      : ''
  )
  const [modelReasoningEffort, setModelReasoningEffort] = useState(() => {
    const initialModel = existingConfig?.model ?? ''
    const options = effortsForModel(initialModel)
    const current = existingConfig?.modelReasoningEffort || 'none'
    return toStoredEffort(clampEffortToSupported(current, options))
  })
  const [apiKey, setApiKey] = useState(existingConfig?.apiKey ?? '')

  // `[model_providers.<id>]` advanced keys (upstream config-reference).
  const [advanced, setAdvanced] = useState(() => ({
    modelCatalogUrl: existingConfig?.modelCatalogUrl ?? '',
    envHttpHeaders: formatHeaderLines(existingConfig?.envHttpHeaders),
    requestMaxRetries:
      existingConfig?.requestMaxRetries != null
        ? String(existingConfig.requestMaxRetries)
        : '',
    streamMaxRetries:
      existingConfig?.streamMaxRetries != null
        ? String(existingConfig.streamMaxRetries)
        : '',
    streamIdleTimeoutMs:
      existingConfig?.streamIdleTimeoutMs != null
        ? String(existingConfig.streamIdleTimeoutMs)
        : '',
    supportsWebSockets: existingConfig?.supportsWebSockets ?? false,
    supportsStandaloneWebSearch:
      existingConfig?.supportsStandaloneWebSearch ?? false,
    externalWebAccess:
      existingConfig?.capabilities?.externalWebAccess == null
        ? 'default'
        : existingConfig.capabilities.externalWebAccess
          ? 'on'
          : 'off',
    remoteCompaction:
      existingConfig?.capabilities?.remoteCompaction ?? 'default',
    authCommand: existingConfig?.auth?.command ?? '',
    authArgs: (existingConfig?.auth?.args ?? []).join(' '),
    authTimeoutMs:
      existingConfig?.auth?.timeoutMs != null
        ? String(existingConfig.auth.timeoutMs)
        : '',
    authRefreshIntervalMs:
      existingConfig?.auth?.refreshIntervalMs != null
        ? String(existingConfig.auth.refreshIntervalMs)
        : '',
    authCwd: existingConfig?.auth?.cwd ?? '',
  }))
  const setAdv = (patch: Partial<typeof advanced>) =>
    setAdvanced(previous => ({ ...previous, ...patch }))

  const isReservedProviderId = providerId.trim().toLowerCase() === 'openai'

  // Channel picker state
  const [channelPickerOpen, setChannelPickerOpen] = useState(false)

  const effortOptions = useMemo(() => effortsForModel(model), [model])

  // Keep Select value valid when model changes without writing during render.
  const resolvedEffort = useMemo(() => {
    const current = modelReasoningEffort || 'none'
    return toStoredEffort(clampEffortToSupported(current, effortOptions))
  }, [modelReasoningEffort, effortOptions])

  const handleModelChange = (nextModel: string) => {
    setModel(nextModel)
    const options = effortsForModel(nextModel)
    const current = modelReasoningEffort || 'none'
    setModelReasoningEffort(
      toStoredEffort(clampEffortToSupported(current, options))
    )
  }

  const contextWindowSelectValue = tokenLimitSelectValue(
    modelContextWindow,
    CONTEXT_WINDOW_PRESETS
  )
  const autoCompactSelectValue = tokenLimitSelectValue(
    modelAutoCompactTokenLimit,
    AUTO_COMPACT_PRESETS
  )

  const handleContextWindowSelect = (value: string) => {
    if (value === UNSET_VALUE || value === CUSTOM_VALUE) {
      if (value === UNSET_VALUE) setModelContextWindow('')
      return
    }
    const preset = CONTEXT_WINDOW_PRESETS.find(option => option.value === value)
    setModelContextWindow(value)
    // Linked tier: preset context windows select the matching auto-compact
    // preset; the user may re-pick the auto-compact tier afterwards.
    if (preset) setModelAutoCompactTokenLimit(preset.autoCompact)
  }

  const handleAutoCompactSelect = (value: string) => {
    if (value === UNSET_VALUE) setModelAutoCompactTokenLimit('')
    else if (value !== CUSTOM_VALUE) setModelAutoCompactTokenLimit(value)
  }

  const sanitizeProviderId = (name: string): string => {
    return name
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
  }

  const handleImportFromChannel = (
    models: CustomModel[],
    context: ChannelProviderContext
  ) => {
    if (!isEditing) {
      const sanitizedId = sanitizeProviderId(context.channelName)

      // Infer protocol from channel context
      const protocol = inferModelProtocol(
        context.channelType,
        context.platform,
        context.baseUrl
      )

      // Codex removed the Chat Completions wire format (wire_api = "chat")
      // in Feb 2026 and hard-fails on it, so imports always select the
      // Responses API. The endpoint shape (/v1 suffix) still follows the
      // chat-completions intent of the channel: for explicitly picked
      // protocols (multi-protocol platforms) only the OpenAI option means
      // the Responses API.
      const usesChatCompletions =
        context.provider != null
          ? context.provider !== 'openai'
          : protocol === 'anthropic'

      setProviderId(sanitizedId)
      setName(context.channelName)
      // 通用兼容模式与 sub2api 渠道的 chat/completions 端点都必须带 /v1
      setBaseUrl(
        needsOpenAICompatibleV1(
          context.channelType,
          context.provider,
          usesChatCompletions
        )
          ? ensureOpenAICompatibleV1(context.baseUrl)
          : context.baseUrl
      )
      setApiKey(context.apiKey)
      setWireApi('responses')

      // Pre-fill model from selected model
      const selectedModel = models[0]
      if (selectedModel) {
        handleModelChange(selectedModel.model)
      }
    }
  }

  const handleSave = () => {
    if (!providerId.trim()) return

    const capabilities =
      advanced.externalWebAccess === 'default' &&
      advanced.remoteCompaction === 'default'
        ? null
        : {
            externalWebAccess:
              advanced.externalWebAccess === 'default'
                ? null
                : advanced.externalWebAccess === 'on',
            remoteCompaction:
              advanced.remoteCompaction === 'default'
                ? null
                : advanced.remoteCompaction,
          }
    const authCommand = advanced.authCommand.trim()
    const auth = authCommand
      ? {
          command: authCommand,
          args: advanced.authArgs.trim()
            ? advanced.authArgs.trim().split(/\s+/)
            : null,
          timeoutMs: parseOptionalUint(advanced.authTimeoutMs),
          refreshIntervalMs: parseOptionalUint(advanced.authRefreshIntervalMs),
          cwd: trimToNullValue(advanced.authCwd),
        }
      : null

    const config: CodexProviderConfig = {
      name: trimToNull(name),
      baseUrl: trimToNull(baseUrl),
      wireApi: wireApi || null,
      requiresOpenaiAuth: false,
      // Preserve keys this dialog does not edit instead of nulling them.
      envKey: existingConfig?.envKey ?? null,
      envKeyInstructions: existingConfig?.envKeyInstructions ?? null,
      httpHeaders: existingConfig?.httpHeaders ?? null,
      envHttpHeaders: parseHeaderLines(advanced.envHttpHeaders),
      queryParams: existingConfig?.queryParams ?? null,
      modelCatalogUrl: trimToNullValue(advanced.modelCatalogUrl),
      capabilities,
      requestMaxRetries: parseOptionalUint(advanced.requestMaxRetries),
      streamMaxRetries: parseOptionalUint(advanced.streamMaxRetries),
      streamIdleTimeoutMs: parseOptionalUint(advanced.streamIdleTimeoutMs),
      supportsWebSockets: advanced.supportsWebSockets ? true : null,
      supportsStandaloneWebSearch: advanced.supportsStandaloneWebSearch
        ? true
        : null,
      auth,
      model: trimToNull(model),
      modelReasoningEffort: resolvedEffort || null,
      modelContextWindow: parseTokenLimit(modelContextWindow),
      modelAutoCompactTokenLimit: parseTokenLimit(modelAutoCompactTokenLimit),
      apiKey: trimToNull(apiKey),
    }

    if (isEditing) {
      updateProvider(providerId, config)
    } else {
      addProvider(providerId.trim(), config)
    }

    onClose()
  }

  const effortLabel = (effort: string): string => {
    const key = EFFORT_LABEL_KEYS[effort]
    if (!key) return effort
    const translated = t(key)
    // If i18n key is missing for minimal, fall back to English label.
    return translated === key && effort === 'minimal' ? 'Minimal' : translated
  }

  return (
    <>
      <ResizableDialogHeader>
        <ResizableDialogTitle>
          {isEditing ? t('codex.provider.edit') : t('codex.provider.add')}
        </ResizableDialogTitle>
        <ResizableDialogDescription>
          {t('codex.provider.dialogDescription')}
        </ResizableDialogDescription>
      </ResizableDialogHeader>

      <ResizableDialogBody>
        <div className="space-y-4">
          {/* Import from Channel */}
          {!isEditing && (
            <Button
              type="button"
              variant="outline"
              className="w-full"
              onClick={() => setChannelPickerOpen(true)}
            >
              <FolderInput className="h-4 w-4 mr-2" />
              {t('codex.provider.importFromChannel')}
            </Button>
          )}

          {/* Provider ID */}
          <div className="space-y-2">
            <Label>{t('codex.provider.id')} *</Label>
            <Input
              value={providerId}
              onChange={e => setProviderId(e.target.value)}
              placeholder="custom"
              disabled={isEditing}
            />
            {isReservedProviderId && (
              <p className="text-sm text-destructive">
                {t('codex.provider.reservedProviderName')}
              </p>
            )}
          </div>

          {/* Display Name */}
          <div className="space-y-2">
            <Label>{t('codex.provider.name')}</Label>
            <Input
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="Custom Provider"
            />
          </div>

          {/* Base URL */}
          <div className="space-y-2">
            <Label>{t('codex.provider.baseUrl')}</Label>
            <Input
              value={baseUrl}
              onChange={e => setBaseUrl(e.target.value)}
              placeholder="https://api.example.com/v1"
            />
          </div>

          {/* API Key */}
          <div className="space-y-2">
            <Label>{t('codex.provider.apiKey')}</Label>
            <SecretInput
              value={apiKey}
              onChange={e => setApiKey(e.target.value)}
              placeholder="sk-..."
            />
            <p className="text-xs text-muted-foreground">
              {t('codex.provider.apiKeyHint')}
            </p>
          </div>

          {/* Wire API */}
          <div className="space-y-2">
            <Label>{t('codex.provider.wireApi')}</Label>
            <Select value={wireApi} onValueChange={setWireApi}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {WIRE_API_OPTIONS.map(option => (
                  <SelectItem
                    key={option.value}
                    value={option.value}
                    disabled={option.disabled === true}
                  >
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {wireApi === 'chat' && (
              <p className="text-sm text-destructive">
                {t('codex.provider.wireApiChatDeprecated')}
              </p>
            )}
          </div>

          {/* Model */}
          <div className="space-y-2">
            <Label>{t('codex.provider.model')}</Label>
            <Input
              value={model}
              onChange={e => handleModelChange(e.target.value)}
              placeholder={t('codex.provider.modelPlaceholder')}
            />
          </div>

          {/* Context Window */}
          <div className="space-y-2">
            <Label>{t('codex.provider.modelContextWindow')}</Label>
            <Select
              value={contextWindowSelectValue}
              onValueChange={handleContextWindowSelect}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={UNSET_VALUE}>
                  {t('codex.provider.notSet')}
                </SelectItem>
                {CONTEXT_WINDOW_PRESETS.map(option => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
                <SelectItem value={CUSTOM_VALUE}>
                  {t('codex.provider.custom')}
                </SelectItem>
              </SelectContent>
            </Select>
            {contextWindowSelectValue === CUSTOM_VALUE && (
              <Input
                value={modelContextWindow}
                onChange={e =>
                  setModelContextWindow(sanitizeTokenLimitInput(e.target.value))
                }
                placeholder={t('codex.provider.tokenLimitPlaceholder')}
                inputMode="numeric"
              />
            )}
          </div>

          {/* Auto-Compact Token Limit */}
          <div className="space-y-2">
            <Label>{t('codex.provider.modelAutoCompactTokenLimit')}</Label>
            <Select
              value={autoCompactSelectValue}
              onValueChange={handleAutoCompactSelect}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={UNSET_VALUE}>
                  {t('codex.provider.notSet')}
                </SelectItem>
                {AUTO_COMPACT_PRESETS.map(option => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
                <SelectItem value={CUSTOM_VALUE}>
                  {t('codex.provider.custom')}
                </SelectItem>
              </SelectContent>
            </Select>
            {autoCompactSelectValue === CUSTOM_VALUE && (
              <Input
                value={modelAutoCompactTokenLimit}
                onChange={e =>
                  setModelAutoCompactTokenLimit(
                    sanitizeTokenLimitInput(e.target.value)
                  )
                }
                placeholder={t('codex.provider.tokenLimitPlaceholder')}
                inputMode="numeric"
              />
            )}
            <p className="text-xs text-muted-foreground">
              {t('codex.provider.contextWindowHint')}
            </p>
          </div>

          {/* Reasoning Effort */}
          <div className="space-y-2">
            <Label>{t('codex.provider.reasoningEffort')}</Label>
            <Select
              value={resolvedEffort || '__none__'}
              onValueChange={v =>
                setModelReasoningEffort(v === '__none__' ? '' : v)
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {effortOptions.map(effort => (
                  <SelectItem
                    key={effort}
                    value={effort === 'none' ? '__none__' : effort}
                  >
                    {effortLabel(effort)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Advanced provider keys (upstream config-reference) */}
          <details className="rounded-md border px-3 py-2">
            <summary className="cursor-pointer text-sm font-medium">
              {t('codex.provider.advanced')}
            </summary>
            <div className="mt-3 space-y-4">
              <div className="space-y-2">
                <Label>{t('codex.provider.modelCatalogUrl')}</Label>
                <Input
                  value={advanced.modelCatalogUrl}
                  onChange={e => setAdv({ modelCatalogUrl: e.target.value })}
                  placeholder="https://example.com/models.json"
                />
              </div>

              <div className="space-y-2">
                <Label>{t('codex.provider.envHttpHeaders')}</Label>
                <textarea
                  className="min-h-20 w-full rounded-md border bg-transparent px-3 py-2 font-mono text-sm"
                  value={advanced.envHttpHeaders}
                  onChange={e => setAdv({ envHttpHeaders: e.target.value })}
                  placeholder="X-Org=ORG_ID"
                />
                <p className="text-xs text-muted-foreground">
                  {t('codex.provider.envHttpHeadersHint')}
                </p>
              </div>

              <div className="grid grid-cols-3 gap-3">
                <div className="space-y-2">
                  <Label>{t('codex.provider.requestMaxRetries')}</Label>
                  <Input
                    inputMode="numeric"
                    value={advanced.requestMaxRetries}
                    onChange={e =>
                      setAdv({
                        requestMaxRetries: e.target.value.replace(/\D+/g, ''),
                      })
                    }
                  />
                </div>
                <div className="space-y-2">
                  <Label>{t('codex.provider.streamMaxRetries')}</Label>
                  <Input
                    inputMode="numeric"
                    value={advanced.streamMaxRetries}
                    onChange={e =>
                      setAdv({
                        streamMaxRetries: e.target.value.replace(/\D+/g, ''),
                      })
                    }
                  />
                </div>
                <div className="space-y-2">
                  <Label>{t('codex.provider.streamIdleTimeoutMs')}</Label>
                  <Input
                    inputMode="numeric"
                    value={advanced.streamIdleTimeoutMs}
                    onChange={e =>
                      setAdv({
                        streamIdleTimeoutMs: e.target.value.replace(/\D+/g, ''),
                      })
                    }
                  />
                </div>
              </div>

              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={advanced.supportsWebSockets}
                  onChange={e =>
                    setAdv({ supportsWebSockets: e.target.checked })
                  }
                />
                {t('codex.provider.supportsWebSockets')}
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={advanced.supportsStandaloneWebSearch}
                  onChange={e =>
                    setAdv({ supportsStandaloneWebSearch: e.target.checked })
                  }
                />
                {t('codex.provider.supportsStandaloneWebSearch')}
              </label>

              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-2">
                  <Label>{t('codex.provider.externalWebAccess')}</Label>
                  <Select
                    value={advanced.externalWebAccess}
                    onValueChange={value =>
                      setAdv({ externalWebAccess: value })
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="default">
                        {t('codex.provider.defaultValue')}
                      </SelectItem>
                      <SelectItem value="on">
                        {t('codex.provider.on')}
                      </SelectItem>
                      <SelectItem value="off">
                        {t('codex.provider.off')}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>{t('codex.provider.remoteCompaction')}</Label>
                  <Select
                    value={advanced.remoteCompaction}
                    onValueChange={value => setAdv({ remoteCompaction: value })}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="default">
                        {t('codex.provider.defaultValue')}
                      </SelectItem>
                      <SelectItem value="unsupported">unsupported</SelectItem>
                      <SelectItem value="v2">v2</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <div className="space-y-2">
                <Label>{t('codex.provider.authCommand')}</Label>
                <Input
                  value={advanced.authCommand}
                  onChange={e => setAdv({ authCommand: e.target.value })}
                  placeholder="get-token"
                />
              </div>
              <div className="space-y-2">
                <Label>{t('codex.provider.authArgs')}</Label>
                <Input
                  value={advanced.authArgs}
                  onChange={e => setAdv({ authArgs: e.target.value })}
                  placeholder="--audience codex"
                />
              </div>
              <div className="grid grid-cols-3 gap-3">
                <div className="space-y-2">
                  <Label>{t('codex.provider.authTimeoutMs')}</Label>
                  <Input
                    inputMode="numeric"
                    value={advanced.authTimeoutMs}
                    onChange={e =>
                      setAdv({
                        authTimeoutMs: e.target.value.replace(/\D+/g, ''),
                      })
                    }
                  />
                </div>
                <div className="space-y-2">
                  <Label>{t('codex.provider.authRefreshIntervalMs')}</Label>
                  <Input
                    inputMode="numeric"
                    value={advanced.authRefreshIntervalMs}
                    onChange={e =>
                      setAdv({
                        authRefreshIntervalMs: e.target.value.replace(
                          /\D+/g,
                          ''
                        ),
                      })
                    }
                  />
                </div>
                <div className="space-y-2">
                  <Label>{t('codex.provider.authCwd')}</Label>
                  <Input
                    value={advanced.authCwd}
                    onChange={e => setAdv({ authCwd: e.target.value })}
                  />
                </div>
              </div>
            </div>
          </details>
        </div>
      </ResizableDialogBody>

      <ResizableDialogFooter>
        <Button variant="outline" onClick={onClose}>
          {t('common.cancel')}
        </Button>
        <Button
          onClick={handleSave}
          disabled={!providerId.trim() || isReservedProviderId}
        >
          {t('common.save')}
        </Button>
      </ResizableDialogFooter>

      {/* Channel Model Picker Dialog */}
      <ChannelModelPickerDialog
        open={channelPickerOpen}
        onOpenChange={setChannelPickerOpen}
        mode="single"
        onSelect={_models => {
          // Provider-level import handled by onSelectWithContext
        }}
        onSelectWithContext={handleImportFromChannel}
        showBatchConfig={false}
        platformFilter={p =>
          p === null || p === 'openai' || isMultiProtocolPlatform(p)
        }
      />
    </>
  )
}

export function ProviderDialog({
  open,
  onOpenChange,
  editingProviderId,
}: ProviderDialogProps) {
  const currentProfile = useCodexStore(state => state.currentProfile)

  return (
    <ResizableDialog open={open} onOpenChange={onOpenChange}>
      <ResizableDialogContent
        defaultWidth={500}
        defaultHeight={680}
        minWidth={400}
        minHeight={380}
        onCloseAutoFocus={e => e.preventDefault()}
      >
        {open && (
          <ProviderForm
            editingProviderId={editingProviderId}
            currentProfile={currentProfile}
            onClose={() => onOpenChange(false)}
          />
        )}
      </ResizableDialogContent>
    </ResizableDialog>
  )
}
