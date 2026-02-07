import { describe, it, expect } from 'vitest'
import { protocolToOpenCodeNpm } from './opencode-npm'
import type { ModelProtocol } from './types'

describe('protocolToOpenCodeNpm', () => {
  it('should map anthropic to @ai-sdk/anthropic', () => {
    const protocol: ModelProtocol = 'anthropic'
    expect(protocolToOpenCodeNpm(protocol)).toBe('@ai-sdk/anthropic')
  })

  it('should map openai to @ai-sdk/openai', () => {
    const protocol: ModelProtocol = 'openai'
    expect(protocolToOpenCodeNpm(protocol)).toBe('@ai-sdk/openai')
  })

  it('should map google-ai to @ai-sdk/google', () => {
    const protocol: ModelProtocol = 'google-ai'
    expect(protocolToOpenCodeNpm(protocol)).toBe('@ai-sdk/google')
  })

  it('should map openai-compatible to @ai-sdk/openai-compatible', () => {
    const protocol: ModelProtocol = 'openai-compatible'
    expect(protocolToOpenCodeNpm(protocol)).toBe('@ai-sdk/openai-compatible')
  })

  it('should handle all protocol types', () => {
    const protocols: ModelProtocol[] = [
      'anthropic',
      'openai',
      'google-ai',
      'openai-compatible',
    ]

    const expectedPackages = [
      '@ai-sdk/anthropic',
      '@ai-sdk/openai',
      '@ai-sdk/google',
      '@ai-sdk/openai-compatible',
    ]

    protocols.forEach((protocol, index) => {
      expect(protocolToOpenCodeNpm(protocol)).toBe(expectedPackages[index])
    })
  })
})
