import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { RefreshCw } from 'lucide-react'
import { toast } from 'sonner'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useOpenClawStore } from '@/store/openclaw-store'

interface BlockStreamingConfig {
  blockStreamingDefault: 'on' | 'off'
  blockStreamingBreak: 'text_end' | 'message_end'
  blockStreamingChunk: {
    minChars: number
    maxChars: number
  }
  blockStreamingCoalesce: {
    idleMs: number
  }
}

interface TelegramChannelConfig {
  blockStreaming: boolean
  chunkMode: 'newline' | 'chars'
}

const DEFAULT_BLOCK_STREAMING: BlockStreamingConfig = {
  blockStreamingDefault: 'on',
  blockStreamingBreak: 'text_end',
  blockStreamingChunk: {
    minChars: 200,
    maxChars: 600,
  },
  blockStreamingCoalesce: {
    idleMs: 200,
  },
}

const DEFAULT_TELEGRAM: TelegramChannelConfig = {
  blockStreaming: true,
  chunkMode: 'newline',
}

export function OpenClawHelpersPage() {
  const { t } = useTranslation()
  const currentProfile = useOpenClawStore(state => state.currentProfile)
  const updateBlockStreamingConfig = useOpenClawStore(
    state => state.updateBlockStreamingConfig
  )

  const [blockStreaming, setBlockStreaming] = useState<BlockStreamingConfig>(
    DEFAULT_BLOCK_STREAMING
  )
  const [telegram, setTelegram] =
    useState<TelegramChannelConfig>(DEFAULT_TELEGRAM)
  const [isLoading, setIsLoading] = useState(false)

  useEffect(() => {
    if (currentProfile?.blockStreamingConfig) {
      const config = currentProfile.blockStreamingConfig
      setBlockStreaming({
        blockStreamingDefault:
          (config.blockStreamingDefault as 'on' | 'off') ?? 'on',
        blockStreamingBreak:
          (config.blockStreamingBreak as 'text_end' | 'message_end') ??
          'text_end',
        blockStreamingChunk: {
          minChars: config.blockStreamingChunk?.minChars ?? 200,
          maxChars: config.blockStreamingChunk?.maxChars ?? 600,
        },
        blockStreamingCoalesce: {
          idleMs: config.blockStreamingCoalesce?.idleMs ?? 200,
        },
      })
      if (config.telegramChannel) {
        setTelegram({
          blockStreaming: config.telegramChannel.blockStreaming ?? true,
          chunkMode:
            (config.telegramChannel.chunkMode as 'newline' | 'chars') ??
            'newline',
        })
      }
    }
  }, [currentProfile])

  const handleSave = async () => {
    if (!currentProfile) return
    setIsLoading(true)
    try {
      await updateBlockStreamingConfig({
        ...blockStreaming,
        telegramChannel: telegram,
      })
      toast.success(t('common.saved'))
    } finally {
      setIsLoading(false)
    }
  }

  const handleReset = () => {
    setBlockStreaming(DEFAULT_BLOCK_STREAMING)
    setTelegram(DEFAULT_TELEGRAM)
  }

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between p-4 border-b">
        <h1 className="text-xl font-semibold">{t('openclaw.helpers.title')}</h1>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleReset}
            disabled={isLoading}
          >
            <RefreshCw className="h-4 w-4 mr-2" />
            {t('common.reset')}
          </Button>
          <Button
            size="sm"
            onClick={handleSave}
            disabled={!currentProfile || isLoading}
          >
            {t('common.save')}
          </Button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-4">
        <div className="space-y-6">
          {/* Block Streaming Settings */}
          <div className="space-y-4 p-4 border rounded-lg">
            <div>
              <h2 className="text-lg font-medium">
                {t('openclaw.helpers.blockStreaming.title')}
              </h2>
              <p className="text-sm text-muted-foreground">
                {t('openclaw.helpers.blockStreaming.description')}
              </p>
            </div>

            {/* Enable/Disable */}
            <div className="flex items-center justify-between">
              <div className="flex flex-col gap-1">
                <Label className="text-sm font-medium">
                  {t('openclaw.helpers.blockStreaming.default')}
                </Label>
              </div>
              <Select
                value={blockStreaming.blockStreamingDefault}
                onValueChange={value =>
                  setBlockStreaming(prev => ({
                    ...prev,
                    blockStreamingDefault: value as 'on' | 'off',
                  }))
                }
              >
                <SelectTrigger className="w-32">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="on">{t('common.enabled')}</SelectItem>
                  <SelectItem value="off">{t('common.disabled')}</SelectItem>
                </SelectContent>
              </Select>
            </div>

            {/* Break Mode */}
            <div className="flex items-center justify-between">
              <div className="flex flex-col gap-1">
                <Label className="text-sm font-medium">
                  {t('openclaw.helpers.blockStreaming.break')}
                </Label>
              </div>
              <Select
                value={blockStreaming.blockStreamingBreak}
                onValueChange={value =>
                  setBlockStreaming(prev => ({
                    ...prev,
                    blockStreamingBreak: value as 'text_end' | 'message_end',
                  }))
                }
              >
                <SelectTrigger className="w-48">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="text_end">
                    {t('openclaw.helpers.blockStreaming.break.textEnd')}
                  </SelectItem>
                  <SelectItem value="message_end">
                    {t('openclaw.helpers.blockStreaming.break.messageEnd')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>

            {/* Min Chars */}
            <div className="flex items-center justify-between">
              <div className="flex flex-col gap-1">
                <Label className="text-sm font-medium">
                  {t('openclaw.helpers.blockStreaming.minChars')}
                </Label>
              </div>
              <Input
                type="number"
                className="w-32"
                value={blockStreaming.blockStreamingChunk.minChars}
                onChange={e =>
                  setBlockStreaming(prev => ({
                    ...prev,
                    blockStreamingChunk: {
                      ...prev.blockStreamingChunk,
                      minChars: parseInt(e.target.value) || 0,
                    },
                  }))
                }
              />
            </div>

            {/* Max Chars */}
            <div className="flex items-center justify-between">
              <div className="flex flex-col gap-1">
                <Label className="text-sm font-medium">
                  {t('openclaw.helpers.blockStreaming.maxChars')}
                </Label>
              </div>
              <Input
                type="number"
                className="w-32"
                value={blockStreaming.blockStreamingChunk.maxChars}
                onChange={e =>
                  setBlockStreaming(prev => ({
                    ...prev,
                    blockStreamingChunk: {
                      ...prev.blockStreamingChunk,
                      maxChars: parseInt(e.target.value) || 0,
                    },
                  }))
                }
              />
            </div>

            {/* Idle Ms */}
            <div className="flex items-center justify-between">
              <div className="flex flex-col gap-1">
                <Label className="text-sm font-medium">
                  {t('openclaw.helpers.blockStreaming.idleMs')}
                </Label>
              </div>
              <Input
                type="number"
                className="w-32"
                value={blockStreaming.blockStreamingCoalesce.idleMs}
                onChange={e =>
                  setBlockStreaming(prev => ({
                    ...prev,
                    blockStreamingCoalesce: {
                      idleMs: parseInt(e.target.value) || 0,
                    },
                  }))
                }
              />
            </div>
          </div>

          {/* Telegram Channel Settings */}
          <div className="space-y-4 p-4 border rounded-lg">
            <h2 className="text-lg font-medium">
              {t('openclaw.helpers.telegram.title')}
            </h2>

            {/* Block Streaming Toggle */}
            <div className="flex items-center justify-between">
              <div className="flex flex-col gap-1">
                <Label className="text-sm font-medium">
                  {t('openclaw.helpers.telegram.blockStreaming')}
                </Label>
              </div>
              <Switch
                checked={telegram.blockStreaming}
                onCheckedChange={checked =>
                  setTelegram(prev => ({ ...prev, blockStreaming: checked }))
                }
              />
            </div>

            {/* Chunk Mode */}
            <div className="flex items-center justify-between">
              <div className="flex flex-col gap-1">
                <Label className="text-sm font-medium">
                  {t('openclaw.helpers.telegram.chunkMode')}
                </Label>
              </div>
              <Select
                value={telegram.chunkMode}
                onValueChange={value =>
                  setTelegram(prev => ({
                    ...prev,
                    chunkMode: value as 'newline' | 'chars',
                  }))
                }
              >
                <SelectTrigger className="w-40">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="newline">
                    {t('openclaw.helpers.telegram.chunkMode.newline')}
                  </SelectItem>
                  <SelectItem value="chars">
                    {t('openclaw.helpers.telegram.chunkMode.chars')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
