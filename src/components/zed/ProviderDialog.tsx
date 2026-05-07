import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Plus, Trash2, FolderInput, Pencil } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { SecretInput } from '@/components/ui/secret-input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  ResizableDialog,
  ResizableDialogContent,
  ResizableDialogDescription,
  ResizableDialogFooter,
  ResizableDialogHeader,
  ResizableDialogBody,
  ResizableDialogTitle,
} from '@/components/ui/resizable-dialog'
import { useZedStore } from '@/store/zed-store'
import type {
  ZedProfile,
  ZedProviderConfig,
  ZedModel,
  CustomModel,
  ZedModelCapabilities,
} from '@/lib/bindings'
import { ChannelModelPickerDialog } from '@/components/channels/ChannelModelPickerDialog'
import type { ChannelProviderContext } from '@/components/channels'

interface ProviderDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  editingProviderId: string | null
  currentProfile: ZedProfile | null
}

// --- Model Item ---

interface ModelItemProps {
  model: ZedModel
  onEdit: () => void
  onDelete: () => void
}

function ModelItem({ model, onEdit, onDelete }: ModelItemProps) {
  const { t } = useTranslation()

  return (
    <div className="flex items-center justify-between p-2 border rounded-md hover:bg-muted/50 transition-colors">
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="font-medium text-sm">{model.name}</span>
          {model.displayName && (
            <span className="text-muted-foreground text-xs">
              ({model.displayName})
            </span>
          )}
        </div>
        <div className="flex items-center gap-2 mt-1 flex-wrap">
          {model.maxTokens != null && (
            <Badge variant="secondary" className="text-xs">
              Max: {model.maxTokens.toLocaleString()}
            </Badge>
          )}
          {model.maxOutputTokens != null && (
            <Badge variant="secondary" className="text-xs">
              MaxOut: {model.maxOutputTokens.toLocaleString()}
            </Badge>
          )}
          {model.maxCompletionTokens != null && (
            <Badge variant="secondary" className="text-xs">
              MaxCmp: {model.maxCompletionTokens.toLocaleString()}
            </Badge>
          )}
          {model.capabilities?.tools && (
            <Badge variant="outline" className="text-xs">
              {t('zed.model.tools')}
            </Badge>
          )}
          {model.capabilities?.images && (
            <Badge variant="outline" className="text-xs">
              {t('zed.model.images')}
            </Badge>
          )}
        </div>
      </div>
      <div className="flex items-center gap-1 ml-2">
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7"
          onClick={onEdit}
          title={t('common.edit')}
        >
          <Pencil className="h-3 w-3" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7"
          onClick={onDelete}
          title={t('common.delete')}
        >
          <Trash2 className="h-3 w-3" />
        </Button>
      </div>
    </div>
  )
}

// --- Model Edit Dialog ---

interface ModelEditDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  editingModelName: string | null
  model: ZedModel | null
  existingModelNames: string[]
  onSave: (modelName: string, model: ZedModel) => void
}

function ModelEditDialog({
  open,
  onOpenChange,
  editingModelName,
  model,
  existingModelNames,
  onSave,
}: ModelEditDialogProps) {
  const { t } = useTranslation()
  const isEditing = editingModelName !== null

  const [modelName, setModelName] = useState(model?.name ?? '')
  const [displayName, setDisplayName] = useState(model?.displayName ?? '')
  const [maxTokens, setMaxTokens] = useState(model?.maxTokens?.toString() ?? '')
  const [maxOutputTokens, setMaxOutputTokens] = useState(
    model?.maxOutputTokens?.toString() ?? ''
  )
  const [maxCompletionTokens, setMaxCompletionTokens] = useState(
    model?.maxCompletionTokens?.toString() ?? ''
  )
  const [tools, setTools] = useState(model?.capabilities?.tools ?? true)
  const [images, setImages] = useState(model?.capabilities?.images ?? false)
  const [parallelToolCalls, setParallelToolCalls] = useState(
    model?.capabilities?.parallelToolCalls ?? false
  )
  const [promptCacheKey, setPromptCacheKey] = useState(
    model?.capabilities?.promptCacheKey ?? false
  )
  const [chatCompletions, setChatCompletions] = useState(
    model?.capabilities?.chatCompletions ?? false
  )
  const [interleavedReasoning, setInterleavedReasoning] = useState(
    model?.capabilities?.interleavedReasoning ?? false
  )
  const [error, setError] = useState('')

  const handleSave = () => {
    const trimmed = modelName.trim()
    if (!trimmed) {
      setError(t('zed.model.nameRequired'))
      return
    }

    if (trimmed !== editingModelName && existingModelNames.includes(trimmed)) {
      setError(t('zed.model.nameDuplicate'))
      return
    }

    let parsedMaxTokens: number | null = null
    if (maxTokens.trim()) {
      parsedMaxTokens = parseInt(maxTokens, 10)
      if (isNaN(parsedMaxTokens) || parsedMaxTokens <= 0) {
        setError(t('zed.model.maxTokensInvalid'))
        return
      }
    }

    let parsedMaxOutputTokens: number | null = null
    if (maxOutputTokens.trim()) {
      parsedMaxOutputTokens = parseInt(maxOutputTokens, 10)
      if (isNaN(parsedMaxOutputTokens) || parsedMaxOutputTokens <= 0) {
        setError(t('zed.model.maxTokensInvalid'))
        return
      }
    }

    let parsedMaxCompletionTokens: number | null = null
    if (maxCompletionTokens.trim()) {
      parsedMaxCompletionTokens = parseInt(maxCompletionTokens, 10)
      if (isNaN(parsedMaxCompletionTokens) || parsedMaxCompletionTokens <= 0) {
        setError(t('zed.model.maxTokensInvalid'))
        return
      }
    }

    const capabilities: ZedModelCapabilities = {
      tools,
      images,
      parallelToolCalls,
      promptCacheKey,
      chatCompletions,
      interleavedReasoning,
    }

    const newModel: ZedModel = {
      name: trimmed,
      displayName: displayName.trim() || null,
      maxTokens: parsedMaxTokens,
      maxOutputTokens: parsedMaxOutputTokens,
      maxCompletionTokens: parsedMaxCompletionTokens,
      capabilities,
    }

    onSave(trimmed, newModel)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            {isEditing
              ? t('zed.provider.editModel')
              : t('zed.provider.addModel')}
          </DialogTitle>
          <DialogDescription>
            {isEditing ? '' : t('zed.provider.dialogDescription')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label>{t('zed.model.name')} *</Label>
            <Input
              value={modelName}
              onChange={e => {
                setModelName(e.target.value)
                setError('')
              }}
              placeholder="gpt-4o"
              disabled={isEditing}
            />
          </div>

          <div className="space-y-2">
            <Label>{t('zed.model.displayName')}</Label>
            <Input
              value={displayName}
              onChange={e => setDisplayName(e.target.value)}
              placeholder={t('zed.model.displayNamePlaceholder')}
            />
          </div>

          <div className="space-y-2">
            <Label>{t('zed.model.maxTokens')}</Label>
            <Input
              type="number"
              value={maxTokens}
              onChange={e => setMaxTokens(e.target.value)}
              placeholder="16384"
              min="0"
            />
          </div>

          <div className="space-y-2">
            <Label>{t('zed.model.maxOutputTokens')}</Label>
            <Input
              type="number"
              value={maxOutputTokens}
              onChange={e => setMaxOutputTokens(e.target.value)}
              placeholder="8192"
              min="0"
            />
          </div>

          <div className="space-y-2">
            <Label>{t('zed.model.maxCompletionTokens')}</Label>
            <Input
              type="number"
              value={maxCompletionTokens}
              onChange={e => setMaxCompletionTokens(e.target.value)}
              placeholder="8192"
              min="0"
            />
          </div>

          <div className="space-y-2">
            <Label>{t('zed.model.capabilities')}</Label>
            <div className="grid grid-cols-2 gap-2 pt-1">
              <div className="flex items-center gap-2">
                <Checkbox
                  id="model-tools"
                  checked={tools}
                  onCheckedChange={(checked: boolean) => setTools(checked)}
                />
                <Label htmlFor="model-tools" className="cursor-pointer">
                  {t('zed.model.tools')}
                </Label>
              </div>
              <div className="flex items-center gap-2">
                <Checkbox
                  id="model-images"
                  checked={images}
                  onCheckedChange={(checked: boolean) => setImages(checked)}
                />
                <Label htmlFor="model-images" className="cursor-pointer">
                  {t('zed.model.images')}
                </Label>
              </div>
              <div className="flex items-center gap-2">
                <Checkbox
                  id="model-parallel-tool-calls"
                  checked={parallelToolCalls}
                  onCheckedChange={(checked: boolean) =>
                    setParallelToolCalls(checked)
                  }
                />
                <Label
                  htmlFor="model-parallel-tool-calls"
                  className="cursor-pointer"
                >
                  {t('zed.model.parallelToolCalls')}
                </Label>
              </div>
              <div className="flex items-center gap-2">
                <Checkbox
                  id="model-prompt-cache-key"
                  checked={promptCacheKey}
                  onCheckedChange={(checked: boolean) =>
                    setPromptCacheKey(checked)
                  }
                />
                <Label
                  htmlFor="model-prompt-cache-key"
                  className="cursor-pointer"
                >
                  {t('zed.model.promptCacheKey')}
                </Label>
              </div>
              <div className="flex items-center gap-2">
                <Checkbox
                  id="model-chat-completions"
                  checked={chatCompletions}
                  onCheckedChange={(checked: boolean) =>
                    setChatCompletions(checked)
                  }
                />
                <Label
                  htmlFor="model-chat-completions"
                  className="cursor-pointer"
                >
                  {t('zed.model.chatCompletions')}
                </Label>
              </div>
              <div className="flex items-center gap-2">
                <Checkbox
                  id="model-interleaved-reasoning"
                  checked={interleavedReasoning}
                  onCheckedChange={(checked: boolean) =>
                    setInterleavedReasoning(checked)
                  }
                />
                <Label
                  htmlFor="model-interleaved-reasoning"
                  className="cursor-pointer"
                >
                  {t('zed.model.interleavedReasoning')}
                </Label>
              </div>
            </div>
          </div>

          {error && <div className="text-sm text-destructive">{error}</div>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleSave} disabled={!modelName.trim()}>
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// --- Provider Form (key-based remounting for state initialization) ---

interface ProviderFormProps {
  editingProviderId: string | null
  currentProfile: ZedProfile | null
  onClose: () => void
}

function ProviderForm({
  editingProviderId,
  currentProfile,
  onClose,
}: ProviderFormProps) {
  const { t } = useTranslation()
  const addProvider = useZedStore(state => state.addProvider)
  const updateProvider = useZedStore(state => state.updateProvider)

  const isEditing = editingProviderId !== null
  const existingConfig = editingProviderId
    ? currentProfile?.providers?.[editingProviderId]
    : undefined

  const [providerName, setProviderName] = useState(editingProviderId ?? '')
  const [apiUrl, setApiUrl] = useState(existingConfig?.api_url ?? '')
  const [apiKey, setApiKey] = useState(existingConfig?.apiKey ?? '')
  const [models, setModels] = useState<ZedModel[]>(
    existingConfig?.availableModels ?? []
  )
  const [nameError, setNameError] = useState('')
  const [apiUrlError, setApiUrlError] = useState('')
  const [modelDialogOpen, setModelDialogOpen] = useState(false)
  const [editingModelName, setEditingModelName] = useState<string | null>(null)
  const [editingModel, setEditingModel] = useState<ZedModel | null>(null)
  const [channelPickerOpen, setChannelPickerOpen] = useState(false)

  const sanitizeProviderName = (name: string): string => {
    return name
      .toLowerCase()
      .replace(/[^a-z0-9]/g, '-')
      .replace(/-+/g, '-')
      .replace(/^-|-$/g, '')
  }

  const handleImportFromChannel = (
    selectedModels: CustomModel[],
    context: ChannelProviderContext
  ) => {
    if (!isEditing) {
      const sanitizedName = sanitizeProviderName(context.channelName)
      setProviderName(sanitizedName)
      setApiUrl(context.baseUrl)
      setApiKey(context.apiKey)

      const zedModels: ZedModel[] = selectedModels.map(m => ({
        name: m.model,
        displayName: m.displayName || null,
        maxTokens: m.maxOutputTokens ?? null,
        capabilities: {
          tools: true,
          images: !(m.noImageSupport ?? true),
          parallelToolCalls: false,
          promptCacheKey: false,
          chatCompletions: false,
          interleavedReasoning: false,
        },
      }))
      setModels(zedModels)
    }
  }

  const isValidUrl = (url: string): boolean => {
    try {
      const parsed = new URL(url)
      return parsed.protocol === 'http:' || parsed.protocol === 'https:'
    } catch {
      return false
    }
  }

  const handleSave = () => {
    if (!providerName.trim()) {
      setNameError(t('zed.provider.nameRequired'))
      return
    }

    if (!isEditing && currentProfile?.providers) {
      if (providerName.trim() in currentProfile.providers) {
        setNameError(t('zed.provider.nameDuplicate'))
        return
      }
    }

    if (!apiUrl.trim()) {
      setApiUrlError(t('zed.provider.apiUrlRequired'))
      return
    }
    if (!isValidUrl(apiUrl.trim())) {
      setApiUrlError(t('zed.provider.apiUrlInvalid'))
      return
    }

    const config: ZedProviderConfig = {
      api_url: apiUrl.trim(),
      availableModels: models.length > 0 ? models : null,
      apiKey: apiKey.trim() || null,
    }

    if (isEditing) {
      updateProvider(providerName.trim(), config)
    } else {
      addProvider(providerName.trim(), config)
    }

    onClose()
  }

  const handleAddModel = () => {
    setEditingModelName(null)
    setEditingModel(null)
    setModelDialogOpen(true)
  }

  const handleEditModel = (modelName: string, model: ZedModel) => {
    setEditingModelName(modelName)
    setEditingModel(model)
    setModelDialogOpen(true)
  }

  const handleDeleteModel = (modelName: string) => {
    setModels(prev => prev.filter(m => m.name !== modelName))
  }

  const handleSaveModel = (_modelName: string, model: ZedModel) => {
    if (editingModelName && editingModelName !== model.name) {
      setModels(prev => [
        ...prev.filter(m => m.name !== editingModelName),
        model,
      ])
    } else if (editingModelName) {
      setModels(prev =>
        prev.map(m => (m.name === editingModelName ? model : m))
      )
    } else {
      setModels(prev => [...prev, model])
    }
  }

  return (
    <>
      <ResizableDialogBody>
        <div className="space-y-4">
          {/* Import from Channel - add mode only */}
          {!isEditing && (
            <Button
              type="button"
              variant="outline"
              className="w-full"
              onClick={() => setChannelPickerOpen(true)}
              title={t('zed.provider.importFromChannelHint')}
            >
              <FolderInput className="h-4 w-4 mr-2" />
              {t('zed.provider.importFromChannel')}
            </Button>
          )}

          {/* Provider Name */}
          <div className="space-y-2">
            <Label>{t('zed.provider.name')} *</Label>
            <Input
              value={providerName}
              onChange={e => {
                setProviderName(e.target.value)
                setNameError('')
              }}
              placeholder="my-provider"
              disabled={isEditing}
            />
            {nameError && (
              <p className="text-xs text-destructive">{nameError}</p>
            )}
          </div>

          {/* API URL */}
          <div className="space-y-2">
            <Label>{t('zed.provider.apiUrl')} *</Label>
            <Input
              value={apiUrl}
              onChange={e => {
                setApiUrl(e.target.value)
                setApiUrlError('')
              }}
              placeholder="https://api.openai.com/v1"
            />
            {apiUrlError && (
              <p className="text-xs text-destructive">{apiUrlError}</p>
            )}
          </div>

          {/* API Key */}
          <div className="space-y-2">
            <Label>{t('zed.provider.apiKey')}</Label>
            <SecretInput
              value={apiKey}
              onChange={e => setApiKey(e.target.value)}
              placeholder="sk-..."
            />
            <p className="text-xs text-muted-foreground">
              {t('zed.provider.apiKeyHint')}
            </p>
          </div>

          {/* Models Section */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label>{t('zed.provider.models')}</Label>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={handleAddModel}
              >
                <Plus className="h-4 w-4 mr-1" />
                {t('zed.provider.addModel')}
              </Button>
            </div>

            {models.length === 0 ? (
              <div className="text-sm text-muted-foreground text-center py-4 border rounded-md">
                {t('zed.provider.noModels')}
              </div>
            ) : (
              <div className="space-y-2 border rounded-md p-2 max-h-[200px] overflow-y-auto">
                {models.map(model => (
                  <ModelItem
                    key={model.name}
                    model={model}
                    onEdit={() => handleEditModel(model.name, model)}
                    onDelete={() => handleDeleteModel(model.name)}
                  />
                ))}
              </div>
            )}
          </div>
        </div>
      </ResizableDialogBody>

      <ResizableDialogFooter>
        <Button variant="outline" onClick={onClose}>
          {t('common.cancel')}
        </Button>
        <Button
          onClick={handleSave}
          disabled={!providerName.trim() || !apiUrl.trim()}
        >
          {t('common.save')}
        </Button>
      </ResizableDialogFooter>

      {/* Channel Model Picker Dialog */}
      <ChannelModelPickerDialog
        open={channelPickerOpen}
        onOpenChange={setChannelPickerOpen}
        mode="multiple"
        onSelect={_models => {
          /* handled by onSelectWithContext */
        }}
        onSelectWithContext={handleImportFromChannel}
        showBatchConfig={false}
      />

      {/* Model Edit Dialog */}
      <ModelEditDialog
        key={editingModelName ?? 'new'}
        open={modelDialogOpen}
        onOpenChange={setModelDialogOpen}
        editingModelName={editingModelName}
        model={editingModel}
        existingModelNames={models.map(m => m.name)}
        onSave={handleSaveModel}
      />
    </>
  )
}

// --- Main ProviderDialog ---

export function ProviderDialog({
  open,
  onOpenChange,
  editingProviderId,
  currentProfile,
}: ProviderDialogProps) {
  const { t } = useTranslation()
  const isEditing = editingProviderId !== null
  const formKey = editingProviderId ?? 'new'
  const resetKey = `${formKey}-${currentProfile?.id ?? 'none'}`

  return (
    <ResizableDialog open={open} onOpenChange={onOpenChange}>
      <ResizableDialogContent
        defaultWidth={600}
        defaultHeight={520}
        minWidth={500}
        minHeight={400}
        onCloseAutoFocus={e => e.preventDefault()}
      >
        <ResizableDialogHeader>
          <ResizableDialogTitle>
            {isEditing ? t('zed.provider.edit') : t('zed.provider.add')}
          </ResizableDialogTitle>
          <ResizableDialogDescription>
            {t('zed.provider.dialogDescription')}
          </ResizableDialogDescription>
        </ResizableDialogHeader>

        {open && (
          <ProviderForm
            key={resetKey}
            editingProviderId={editingProviderId}
            currentProfile={currentProfile}
            onClose={() => onOpenChange(false)}
          />
        )}
      </ResizableDialogContent>
    </ResizableDialog>
  )
}
