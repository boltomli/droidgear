import { describe, it, expect } from 'vitest'
import { inferProviderForNewApi, getBaseUrlForNewApi } from './newapi-platform'

describe('inferProviderForNewApi', () => {
  it('returns anthropic for claude- models', () => {
    expect(inferProviderForNewApi('claude-3-opus')).toBe('anthropic')
    expect(inferProviderForNewApi('claude-sonnet-4')).toBe('anthropic')
  })

  it('returns openai for gpt- models', () => {
    expect(inferProviderForNewApi('gpt-4')).toBe('openai')
    expect(inferProviderForNewApi('gpt-5-turbo')).toBe('openai')
  })

  it('handles case-insensitive model matching', () => {
    expect(inferProviderForNewApi('GPT-5.4')).toBe('openai')
    expect(inferProviderForNewApi('GPT-4o')).toBe('openai')
    expect(inferProviderForNewApi('Claude-3-opus')).toBe('anthropic')
  })

  it('recognizes OpenAI o-series models', () => {
    expect(inferProviderForNewApi('o1')).toBe('openai')
    expect(inferProviderForNewApi('o1-mini')).toBe('openai')
    expect(inferProviderForNewApi('o3')).toBe('openai')
    expect(inferProviderForNewApi('o3-mini')).toBe('openai')
    expect(inferProviderForNewApi('o4-mini')).toBe('openai')
  })

  it('returns generic for other models', () => {
    expect(inferProviderForNewApi('gemini-pro')).toBe(
      'generic-chat-completion-api'
    )
    expect(inferProviderForNewApi('llama-3')).toBe(
      'generic-chat-completion-api'
    )
  })
})

describe('getBaseUrlForNewApi', () => {
  it('returns baseUrl as-is for anthropic', () => {
    expect(getBaseUrlForNewApi('anthropic', 'https://api.example.com')).toBe(
      'https://api.example.com'
    )
  })

  it('appends /v1 for openai', () => {
    expect(getBaseUrlForNewApi('openai', 'https://api.example.com')).toBe(
      'https://api.example.com/v1'
    )
  })

  it('appends /v1 for generic', () => {
    expect(
      getBaseUrlForNewApi(
        'generic-chat-completion-api',
        'https://api.example.com'
      )
    ).toBe('https://api.example.com/v1')
  })

  it('does not duplicate /v1', () => {
    expect(getBaseUrlForNewApi('openai', 'https://api.example.com/v1')).toBe(
      'https://api.example.com/v1'
    )
  })
})
