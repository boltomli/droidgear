import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RefreshCw, Copy, Check, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useChannelStore } from '@/store/channel-store'
import type { ChannelToken, ChannelType } from '@/lib/bindings'

interface TokenListProps {
  channelId: string
  channelType: ChannelType
  baseUrl: string
  onSelectToken: (token: ChannelToken) => void
}

function formatQuota(
  quota: number,
  unlimited: boolean,
  t: (key: string) => string
): string {
  if (unlimited) return t('tokens.unlimited')
  if (quota >= 1000000) return `${(quota / 1000000).toFixed(2)}M`
  if (quota >= 1000) return `${(quota / 1000).toFixed(2)}K`
  return quota.toString()
}

function getStatusBadge(status: number, t: (key: string) => string) {
  switch (status) {
    case 1:
      return <Badge variant="default">{t('tokens.status.enabled')}</Badge>
    case 2:
      return <Badge variant="secondary">{t('tokens.status.disabled')}</Badge>
    case 3:
      return <Badge variant="destructive">{t('tokens.status.expired')}</Badge>
    case 4:
      return <Badge variant="outline">{t('tokens.status.exhausted')}</Badge>
    default:
      return <Badge variant="outline">{t('tokens.status.unknown')}</Badge>
  }
}

export function TokenList({
  channelId,
  channelType,
  baseUrl,
  onSelectToken,
}: TokenListProps) {
  const { t } = useTranslation()
  const tokensMap = useChannelStore(state => state.tokens)
  const isLoading = useChannelStore(state => state.isLoading)
  const fetchTokens = useChannelStore(state => state.fetchTokens)
  const [copiedId, setCopiedId] = useState<number | null>(null)

  // Safely get tokens array
  const tokens: ChannelToken[] = tokensMap?.[channelId] ?? []

  const handleRefresh = () => {
    fetchTokens(channelId, channelType, baseUrl)
  }

  const handleCopyKey = async (token: ChannelToken) => {
    await navigator.clipboard.writeText(token.key)
    setCopiedId(token.id)
    setTimeout(() => setCopiedId(null), 2000)
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-medium">{t('tokens.title')}</h3>
        <Button
          variant="outline"
          size="sm"
          onClick={handleRefresh}
          disabled={isLoading}
        >
          {isLoading ? (
            <Loader2 className="h-4 w-4 animate-spin mr-2" />
          ) : (
            <RefreshCw className="h-4 w-4 mr-2" />
          )}
          {t('common.refresh')}
        </Button>
      </div>

      {tokens.length === 0 ? (
        <div className="text-center py-8 text-muted-foreground">
          <p>{t('tokens.noTokens')}</p>
          <p className="text-sm mt-1">{t('tokens.noTokensHint')}</p>
        </div>
      ) : (
        <div className="border rounded-md">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('common.name')}</TableHead>
                <TableHead>{t('common.status')}</TableHead>
                <TableHead>{t('tokens.remaining')}</TableHead>
                <TableHead>{t('tokens.used')}</TableHead>
                <TableHead className="w-[100px]">
                  {t('common.actions')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.map(token => (
                <TableRow key={token.id}>
                  <TableCell className="font-medium">{token.name}</TableCell>
                  <TableCell>{getStatusBadge(token.status, t)}</TableCell>
                  <TableCell>
                    {formatQuota(token.remainQuota, token.unlimitedQuota, t)}
                  </TableCell>
                  <TableCell>
                    {formatQuota(token.usedQuota, false, t)}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        onClick={() => handleCopyKey(token)}
                        title={t('tokens.copyKey')}
                      >
                        {copiedId === token.id ? (
                          <Check className="h-4 w-4 text-green-500" />
                        ) : (
                          <Copy className="h-4 w-4" />
                        )}
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => onSelectToken(token)}
                        disabled={token.status !== 1}
                      >
                        {t('sidebar.models')}
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
