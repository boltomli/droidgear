import type { ChannelType } from '@/lib/bindings'

/**
 * 模型协议类型
 * 描述模型使用的 API 协议标准
 */
export type ModelProtocol =
  | 'anthropic' // Anthropic Messages API
  | 'openai' // OpenAI Chat Completions API
  | 'google-ai' // Google AI (Gemini) API
  | 'openai-compatible' // OpenAI 兼容 API (通用)

/**
 * 模型协议信息
 */
export interface ModelProtocolInfo {
  protocol: ModelProtocol
  baseUrl: string
}

/**
 * Channel 推断上下文
 */
export interface ChannelInferenceContext {
  channelType: ChannelType
  platform: string | null
  baseUrl: string
}

/**
 * Channel 推断器接口
 * 每个 Channel 类型可以实现自己的推断逻辑
 */
export interface ChannelInferrer {
  /**
   * 基于 Channel 自身信息推断协议
   * 返回 null 表示无法从 Channel 推断，需要使用模型名称推断
   */
  inferFromChannel(context: ChannelInferenceContext): ModelProtocol | null

  /**
   * 基于模型 ID 推断协议
   * 返回 null 表示无法推断，使用全局推断
   */
  inferFromModel(
    modelId: string,
    context: ChannelInferenceContext
  ): ModelProtocol | null

  /**
   * 获取协议对应的 Base URL
   */
  getBaseUrl(
    protocol: ModelProtocol,
    baseUrl: string,
    platform?: string | null
  ): string
}
