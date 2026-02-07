import { describe, it, expect } from 'vitest'
import { Sub2ApiInferrer } from './channel-inferrers/sub2api-inferrer'
import { NewApiInferrer } from './channel-inferrers/newapi-inferrer'
import { CliProxyInferrer } from './channel-inferrers/cliproxy-inferrer'
import type { ChannelInferenceContext } from './types'

describe('Sub2ApiInferrer', () => {
  const inferrer = new Sub2ApiInferrer()

  describe('inferFromChannel', () => {
    it('infers anthropic from platform', () => {
      expect(
        inferrer.inferFromChannel({
          channelType: 'sub-2-api',
          platform: 'anthropic',
          baseUrl: 'https://api.example.com',
        })
      ).toBe('anthropic')
    })

    it('infers openai from platform', () => {
      expect(
        inferrer.inferFromChannel({
          channelType: 'sub-2-api',
          platform: 'openai',
          baseUrl: 'https://api.example.com',
        })
      ).toBe('openai')
    })

    it('infers google-ai from gemini platform', () => {
      expect(
        inferrer.inferFromChannel({
          channelType: 'sub-2-api',
          platform: 'gemini',
          baseUrl: 'https://api.example.com',
        })
      ).toBe('google-ai')
    })

    it('returns null for antigravity (needs model inference)', () => {
      expect(
        inferrer.inferFromChannel({
          channelType: 'sub-2-api',
          platform: 'antigravity',
          baseUrl: 'https://api.example.com',
        })
      ).toBeNull()
    })

    it('returns null for unknown platform', () => {
      expect(
        inferrer.inferFromChannel({
          channelType: 'sub-2-api',
          platform: 'unknown',
          baseUrl: 'https://api.example.com',
        })
      ).toBeNull()
    })

    it('returns null for null platform', () => {
      expect(
        inferrer.inferFromChannel({
          channelType: 'sub-2-api',
          platform: null,
          baseUrl: 'https://api.example.com',
        })
      ).toBeNull()
    })
  })

  describe('inferFromModel', () => {
    const antigravityCtx: ChannelInferenceContext = {
      channelType: 'sub-2-api',
      platform: 'antigravity',
      baseUrl: 'https://api.example.com',
    }

    it('infers anthropic for claude on antigravity', () => {
      expect(inferrer.inferFromModel('claude-3-opus', antigravityCtx)).toBe(
        'anthropic'
      )
    })

    it('infers google-ai for gemini on antigravity', () => {
      expect(inferrer.inferFromModel('gemini-1.5-pro', antigravityCtx)).toBe(
        'google-ai'
      )
    })

    it('defaults to anthropic for unknown model on antigravity', () => {
      expect(inferrer.inferFromModel('unknown-model', antigravityCtx)).toBe(
        'anthropic'
      )
    })

    it('returns null for non-antigravity platform', () => {
      const ctx: ChannelInferenceContext = {
        channelType: 'sub-2-api',
        platform: null,
        baseUrl: 'https://api.example.com',
      }
      expect(inferrer.inferFromModel('claude-3-opus', ctx)).toBeNull()
    })
  })

  describe('getBaseUrl', () => {
    it('returns baseUrl as-is for non-antigravity', () => {
      expect(
        inferrer.getBaseUrl('anthropic', 'https://api.example.com', 'anthropic')
      ).toBe('https://api.example.com')
    })

    it('appends /antigravity for anthropic on antigravity', () => {
      expect(
        inferrer.getBaseUrl(
          'anthropic',
          'https://api.example.com',
          'antigravity'
        )
      ).toBe('https://api.example.com/antigravity')
    })

    it('appends /antigravity/v1beta for google-ai on antigravity', () => {
      expect(
        inferrer.getBaseUrl(
          'google-ai',
          'https://api.example.com',
          'antigravity'
        )
      ).toBe('https://api.example.com/antigravity/v1beta')
    })

    it('does not duplicate suffix', () => {
      expect(
        inferrer.getBaseUrl(
          'anthropic',
          'https://api.example.com/antigravity',
          'antigravity'
        )
      ).toBe('https://api.example.com/antigravity')
    })
  })
})

describe('NewApiInferrer', () => {
  const inferrer = new NewApiInferrer()

  describe('inferFromChannel', () => {
    it('always returns null', () => {
      expect(
        inferrer.inferFromChannel({
          channelType: 'new-api',
          platform: null,
          baseUrl: 'https://api.example.com',
        })
      ).toBeNull()
    })
  })

  describe('inferFromModel', () => {
    const ctx: ChannelInferenceContext = {
      channelType: 'new-api',
      platform: null,
      baseUrl: 'https://api.example.com',
    }

    it('infers anthropic for claude models', () => {
      expect(inferrer.inferFromModel('claude-3-opus', ctx)).toBe('anthropic')
    })

    it('infers openai for gpt models', () => {
      expect(inferrer.inferFromModel('gpt-4', ctx)).toBe('openai')
    })

    it('returns null for other models', () => {
      expect(inferrer.inferFromModel('gemini-pro', ctx)).toBeNull()
      expect(inferrer.inferFromModel('llama-3', ctx)).toBeNull()
    })
  })

  describe('getBaseUrl', () => {
    it('returns baseUrl as-is for anthropic', () => {
      expect(inferrer.getBaseUrl('anthropic', 'https://api.example.com')).toBe(
        'https://api.example.com'
      )
    })

    it('appends /v1 for openai', () => {
      expect(inferrer.getBaseUrl('openai', 'https://api.example.com')).toBe(
        'https://api.example.com/v1'
      )
    })

    it('appends /v1 for openai-compatible', () => {
      expect(
        inferrer.getBaseUrl('openai-compatible', 'https://api.example.com')
      ).toBe('https://api.example.com/v1')
    })

    it('appends /v1 for google-ai', () => {
      expect(inferrer.getBaseUrl('google-ai', 'https://api.example.com')).toBe(
        'https://api.example.com/v1'
      )
    })

    it('does not duplicate /v1', () => {
      expect(inferrer.getBaseUrl('openai', 'https://api.example.com/v1')).toBe(
        'https://api.example.com/v1'
      )
    })
  })
})

describe('CliProxyInferrer', () => {
  const inferrer = new CliProxyInferrer()

  it('behaves the same as NewApiInferrer', () => {
    const ctx: ChannelInferenceContext = {
      channelType: 'cli-proxy-api',
      platform: null,
      baseUrl: 'https://api.example.com',
    }

    expect(inferrer.inferFromChannel(ctx)).toBeNull()
    expect(inferrer.inferFromModel('claude-3-opus', ctx)).toBe('anthropic')
    expect(inferrer.inferFromModel('gpt-4', ctx)).toBe('openai')
    expect(inferrer.inferFromModel('unknown', ctx)).toBeNull()
    expect(inferrer.getBaseUrl('anthropic', 'https://api.example.com')).toBe(
      'https://api.example.com'
    )
    expect(inferrer.getBaseUrl('openai', 'https://api.example.com')).toBe(
      'https://api.example.com/v1'
    )
  })
})
