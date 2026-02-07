import type {
  ChannelInferrer,
  ChannelInferenceContext,
  ModelProtocol,
} from '../types'

/**
 * NewAPI 推断器
 *
 * NewAPI 没有 platform 字段，无法从 Channel 推断协议。
 * 需要依赖模型名称推断。
 */
export class NewApiInferrer implements ChannelInferrer {
  inferFromChannel(_context: ChannelInferenceContext): ModelProtocol | null {
    // NewAPI 无法从 Channel 推断，必须使用模型名称
    return null
  }

  inferFromModel(
    modelId: string,
    _context: ChannelInferenceContext
  ): ModelProtocol | null {
    // NewAPI 使用简单的模型名称推断
    if (modelId.startsWith('claude-')) return 'anthropic'
    if (modelId.startsWith('gpt-')) return 'openai'

    // 其他返回 null，使用全局推断
    return null
  }

  getBaseUrl(protocol: ModelProtocol, baseUrl: string): string {
    // Anthropic 不需要 /v1 后缀
    if (protocol === 'anthropic') return baseUrl

    // 其他协议添加 /v1
    return normalizeBaseUrl(baseUrl, '/v1')
  }
}

function normalizeBaseUrl(baseUrl: string, suffix: string): string {
  const trimmed = baseUrl.replace(/\/+$/, '')
  return trimmed.endsWith(suffix) ? trimmed : `${trimmed}${suffix}`
}
