import type { ModelProtocol } from './types'

/**
 * 全局模型名称推断
 * 基于模型 ID 的前缀/模式匹配
 */
export function inferProtocolFromModelId(modelId: string): ModelProtocol {
  const lowerModelId = modelId.toLowerCase()

  // Claude 系列 (Anthropic)
  if (lowerModelId.startsWith('claude-')) return 'anthropic'

  // GPT 系列 (OpenAI)
  if (lowerModelId.startsWith('gpt-')) return 'openai'
  if (lowerModelId.startsWith('o1-')) return 'openai'
  if (lowerModelId.startsWith('o3-')) return 'openai'

  // Gemini 系列 (Google AI)
  if (lowerModelId.startsWith('gemini-')) return 'google-ai'

  // 默认使用 OpenAI 兼容协议
  return 'openai-compatible'
}
