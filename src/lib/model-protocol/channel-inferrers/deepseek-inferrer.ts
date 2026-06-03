import type { ChannelInferenceContext, ModelProtocol } from '../types'
import { GeneralInferrer } from './general-inferrer'

/**
 * DeepSeek 推断器
 *
 * DeepSeek API 仅支持 OpenAI 和 Anthropic 两种协议：
 * - OpenAI 格式：{base}/chat/completions, {base}/models
 * - Anthropic 格式：{base}/anthropic
 */
export class DeepSeekInferrer extends GeneralInferrer {
  override inferFromModel(
    modelId: string,
    _context: ChannelInferenceContext
  ): ModelProtocol | null {
    if (modelId.toLowerCase().startsWith('deepseek-')) return 'openai'
    return null
  }

  override getBaseUrl(protocol: ModelProtocol, baseUrl: string): string {
    const trimmed = baseUrl.replace(/\/+$/, '')
    if (protocol === 'anthropic') return `${trimmed}/anthropic`
    return trimmed
  }
}
