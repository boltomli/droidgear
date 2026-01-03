import type { Provider } from '@/lib/bindings'
import { normalizeBaseUrl } from '@/lib/sub2api-platform'

export const inferProviderForNewApi = (modelId: string): Provider => {
  if (modelId.startsWith('claude-')) return 'anthropic'
  if (modelId.startsWith('gpt-')) return 'openai'
  return 'generic-chat-completion-api'
}

export const getBaseUrlForNewApi = (
  provider: Provider,
  baseUrl: string
): string => {
  if (provider === 'anthropic') return baseUrl
  return normalizeBaseUrl(baseUrl, '/v1')
}
