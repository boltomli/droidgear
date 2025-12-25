import { useState } from 'react'
import { Loader2 } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { commands, type CustomModel, type Provider, type ModelInfo } from '@/lib/bindings'

interface ModelDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  model?: CustomModel
  onSave: (model: CustomModel) => void
}

const defaultBaseUrls: Record<Provider, string> = {
  anthropic: 'https://api.anthropic.com',
  openai: 'https://api.openai.com',
  'generic-chat-completion-api': '',
}

interface ModelFormProps {
  model?: CustomModel
  onSave: (model: CustomModel) => void
  onCancel: () => void
}

function ModelForm({ model, onSave, onCancel }: ModelFormProps) {
  const [provider, setProvider] = useState<Provider>(model?.provider ?? 'anthropic')
  const [baseUrl, setBaseUrl] = useState(model?.baseUrl ?? defaultBaseUrls.anthropic)
  const [apiKey, setApiKey] = useState(model?.apiKey ?? '')
  const [modelId, setModelId] = useState(model?.model ?? '')
  const [displayName, setDisplayName] = useState(model?.displayName ?? '')
  const [maxTokens, setMaxTokens] = useState(model?.maxOutputTokens?.toString() ?? '')
  const [supportsImages, setSupportsImages] = useState(model?.supportsImages ?? false)

  const [availableModels, setAvailableModels] = useState<ModelInfo[]>([])
  const [isFetching, setIsFetching] = useState(false)
  const [fetchError, setFetchError] = useState<string | null>(null)

  const handleProviderChange = (value: Provider) => {
    setProvider(value)
    setBaseUrl(defaultBaseUrls[value])
    setAvailableModels([])
    setFetchError(null)
  }

  const handleFetchModels = async () => {
    if (!baseUrl || !apiKey) {
      setFetchError('Please enter API URL and API Key first')
      return
    }

    setIsFetching(true)
    setFetchError(null)

    const result = await commands.fetchModels(provider, baseUrl, apiKey)

    setIsFetching(false)

    if (result.status === 'ok') {
      setAvailableModels(result.data)
      if (result.data.length === 0) {
        setFetchError('No models found')
      }
    } else {
      setFetchError(result.error)
    }
  }

  const handleSave = () => {
    if (!modelId || !baseUrl || !apiKey) return

    const newModel: CustomModel = {
      model: modelId,
      baseUrl: baseUrl,
      apiKey: apiKey,
      provider,
      displayName: displayName || undefined,
      maxOutputTokens: maxTokens ? parseInt(maxTokens) : undefined,
      supportsImages: supportsImages || undefined,
    }

    onSave(newModel)
  }

  const isValid = modelId && baseUrl && apiKey

  return (
    <>
      <div className="grid gap-4 py-4">
        <div className="grid gap-2">
          <Label htmlFor="provider">Provider</Label>
          <Select value={provider} onValueChange={handleProviderChange}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="anthropic">Anthropic</SelectItem>
              <SelectItem value="openai">OpenAI</SelectItem>
              <SelectItem value="generic-chat-completion-api">Generic (OpenAI Compatible)</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div className="grid gap-2">
          <Label htmlFor="baseUrl">API URL</Label>
          <Input
            id="baseUrl"
            value={baseUrl}
            onChange={e => setBaseUrl(e.target.value)}
            placeholder="https://api.example.com"
          />
        </div>

        <div className="grid gap-2">
          <Label htmlFor="apiKey">API Key</Label>
          <div className="flex gap-2">
            <Input
              id="apiKey"
              type="password"
              value={apiKey}
              onChange={e => setApiKey(e.target.value)}
              placeholder="sk-..."
              className="flex-1"
            />
            <Button
              variant="outline"
              onClick={handleFetchModels}
              disabled={isFetching || !baseUrl || !apiKey}
            >
              {isFetching ? <Loader2 className="h-4 w-4 animate-spin" /> : 'Fetch Models'}
            </Button>
          </div>
          {fetchError && <p className="text-sm text-destructive">{fetchError}</p>}
        </div>

        <div className="grid gap-2">
          <Label htmlFor="model">Model</Label>
          {availableModels.length > 0 ? (
            <Select value={modelId} onValueChange={setModelId}>
              <SelectTrigger>
                <SelectValue placeholder="Select a model" />
              </SelectTrigger>
              <SelectContent>
                {availableModels.map(m => (
                  <SelectItem key={m.id} value={m.id}>
                    {m.name || m.id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <Input
              id="model"
              value={modelId}
              onChange={e => setModelId(e.target.value)}
              placeholder="claude-sonnet-4-5-20250929"
            />
          )}
        </div>

        <div className="grid gap-2">
          <Label htmlFor="displayName">Display Name (optional)</Label>
          <Input
            id="displayName"
            value={displayName}
            onChange={e => setDisplayName(e.target.value)}
            placeholder="My Custom Model"
          />
        </div>

        <div className="grid gap-2">
          <Label htmlFor="maxTokens">Max Tokens (optional)</Label>
          <Input
            id="maxTokens"
            type="number"
            value={maxTokens}
            onChange={e => setMaxTokens(e.target.value)}
            placeholder="8192"
          />
        </div>

        <div className="flex items-center gap-2">
          <Checkbox
            id="supportsImages"
            checked={supportsImages}
            onCheckedChange={checked => setSupportsImages(checked === true)}
          />
          <Label htmlFor="supportsImages">Supports Images</Label>
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" onClick={onCancel}>
          Cancel
        </Button>
        <Button onClick={handleSave} disabled={!isValid}>
          {model ? 'Save' : 'Add'}
        </Button>
      </DialogFooter>
    </>
  )
}

export function ModelDialog({ open, onOpenChange, model, onSave }: ModelDialogProps) {
  const formKey = model ? `edit-${model.model}` : 'new'

  const handleSave = (newModel: CustomModel) => {
    onSave(newModel)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>{model ? 'Edit Model' : 'Add Model'}</DialogTitle>
        </DialogHeader>
        {open && (
          <ModelForm
            key={formKey}
            model={model}
            onSave={handleSave}
            onCancel={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}
