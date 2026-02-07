import type { ModelProtocol } from './types'

/**
 * Map ModelProtocol to OpenCode npm package name
 *
 * OpenCode uses AI SDK packages for different providers:
 * - Anthropic: @ai-sdk/anthropic
 * - OpenAI: @ai-sdk/openai
 * - Google AI: @ai-sdk/google
 * - Generic OpenAI-compatible: @ai-sdk/openai-compatible
 */
export function protocolToOpenCodeNpm(protocol: ModelProtocol): string {
  switch (protocol) {
    case 'anthropic':
      return '@ai-sdk/anthropic'
    case 'openai':
      return '@ai-sdk/openai'
    case 'google-ai':
      return '@ai-sdk/google'
    case 'openai-compatible':
      return '@ai-sdk/openai-compatible'
  }
}
