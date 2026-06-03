import type { ModelProtocol } from '../types'
import { GeneralInferrer } from './general-inferrer'

/**
 * DeepSeek 推断器
 *
 * DeepSeek API 不使用 /v1 前缀，所有端点直接在 base URL 下：
 * - Chat: POST {base}/chat/completions
 * - Models: GET {base}/models
 */
export class DeepSeekInferrer extends GeneralInferrer {
  override getBaseUrl(_protocol: ModelProtocol, baseUrl: string): string {
    return baseUrl.replace(/\/+$/, '')
  }
}
