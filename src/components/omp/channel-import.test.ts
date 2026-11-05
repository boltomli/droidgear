import { describe, expect, it } from 'vitest'
import type { CustomModel, OmpCustomProvider } from '@/lib/bindings'
import { findModelByIdOrAlias } from '@/lib/model-registry'
import {
  buildOmpProvider,
  computeDefaultProviderId,
  createOmpModelFromChannel,
  inferOmpApiType,
  mergeOmpModels,
  normalizeOmpBaseUrl,
  sanitizeProviderId,
} from './channel-import'

const registryModel = findModelByIdOrAlias('gpt-5.2')

if (!registryModel) {
  throw new Error('Expected gpt-5.2 in the model registry')
}

function channelModel(overrides: Partial<CustomModel> = {}): CustomModel {
  return {
    model: 'gpt-5.2',
    baseUrl: 'https://api.example.com/v1',
    apiKey: 'sk-test',
    provider: 'generic-chat-completion-api',
    ...overrides,
  }
}

describe('omp channel import', () => {
  it('sanitizes provider IDs into YAML-safe slugs', () => {
    expect(sanitizeProviderId('My Channel / Token #1')).toBe(
      'my-channel-token-1'
    )
    expect(sanitizeProviderId('  ###  ')).toBe('')
  })

  it('infers the OMP api protocol from the channel platform', () => {
    expect(inferOmpApiType(null)).toBe('openai-completions')
    expect(inferOmpApiType('openai')).toBe('openai-completions')
    expect(inferOmpApiType('anthropic')).toBe('anthropic-messages')
    expect(inferOmpApiType('claude')).toBe('anthropic-messages')
    expect(inferOmpApiType('gemini')).toBe('google-generative-ai')
    expect(inferOmpApiType('antigravity', 'claude-sonnet-4-5')).toBe(
      'anthropic-messages'
    )
    expect(inferOmpApiType('antigravity', 'gemini-3-pro')).toBe(
      'openai-completions'
    )
  })

  it('adds /v1 for Chat Completions endpoints only', () => {
    // sub-2-api + OpenAI chat completions → /v1 suffix
    expect(
      normalizeOmpBaseUrl('sub-2-api', 'https://api.example.com', 'openai')
    ).toBe('https://api.example.com/v1')
    // Anthropic keeps the channel base URL untouched
    expect(
      normalizeOmpBaseUrl('sub-2-api', 'https://api.example.com', 'anthropic')
    ).toBe('https://api.example.com')
    // An already versioned URL is not double-suffixed
    expect(
      normalizeOmpBaseUrl('sub-2-api', 'https://api.example.com/v1', 'openai')
    ).toBe('https://api.example.com/v1')
    // Antigravity has its own path rules and is left alone
    expect(
      normalizeOmpBaseUrl('sub-2-api', 'https://api.example.com', 'antigravity')
    ).toBe('https://api.example.com')
  })

  it('suggests a non-conflicting provider ID', () => {
    expect(computeDefaultProviderId('My API', undefined, [])).toBe('my-api')
    expect(computeDefaultProviderId('My API', 'Token A', [])).toBe(
      'my-api-token-a'
    )
    expect(computeDefaultProviderId('My API', undefined, ['my-api'])).toBe(
      'my-api-2'
    )
    expect(
      computeDefaultProviderId('My API', undefined, ['my-api', 'my-api-2'])
    ).toBe('my-api-3')
    expect(computeDefaultProviderId('###', undefined, [])).toBe(
      'imported-provider'
    )
  })

  it('enriches imported models from the registry', () => {
    const model = createOmpModelFromChannel(channelModel())

    expect(model.id).toBe('gpt-5.2')
    expect(model.name).toBe(registryModel.name)
    expect(model.reasoning).toBe(registryModel.reasoning)
    expect(model.input).toEqual(registryModel.input)
    expect(model.contextWindow).toBe(registryModel.contextWindow)
    expect(model.maxTokens).toBe(registryModel.maxOutputTokens)
  })

  it('drops image input for models flagged as text-only', () => {
    const model = createOmpModelFromChannel(
      channelModel({ noImageSupport: true })
    )

    expect(model.input).toEqual(['text'])
  })

  it('falls back to channel metadata for unknown models', () => {
    const model = createOmpModelFromChannel(
      channelModel({
        model: 'private-model',
        displayName: 'Private Model',
        maxOutputTokens: 4096,
      })
    )

    expect(model).toEqual({
      id: 'private-model',
      name: 'Private Model',
      reasoning: false,
      input: undefined,
      contextWindow: 128000,
      maxTokens: 4096,
    })
  })

  it('merges models by id, keeping unknown fields', () => {
    const merged = mergeOmpModels(
      [{ id: 'a', name: 'A', maxTokens: 1, headers: { X: '1' } }],
      [
        { id: 'a', name: 'A new', maxTokens: 2 },
        { id: 'b', name: 'B' },
      ]
    )

    expect(merged).toEqual([
      { id: 'a', name: 'A new', maxTokens: 2, headers: { X: '1' } },
      { id: 'b', name: 'B' },
    ])
  })

  it('builds a provider entry for a new import', () => {
    const provider = buildOmpProvider({
      channelType: 'sub-2-api',
      platform: 'openai',
      baseUrl: 'https://api.example.com',
      apiKey: 'sk-test',
      models: [channelModel()],
    })

    expect(provider.baseUrl).toBe('https://api.example.com/v1')
    expect(provider.api).toBe('openai-completions')
    expect(provider.apiKey).toBe('sk-test')
    expect(provider.auth).toBe('apiKey')
    expect(provider.models.map(model => model.id)).toEqual(['gpt-5.2'])
  })

  it('preserves unrelated provider fields when re-importing', () => {
    const existing: OmpCustomProvider = {
      baseUrl: 'https://old.example.com/v1',
      api: 'openai-completions',
      apiKey: 'sk-old',
      auth: 'apiKey',
      headers: { 'X-Custom': 'keep' },
      disableStrictTools: true,
      models: [{ id: 'gpt-5.2', name: 'Old label', maxTokens: 1 }],
    }

    const provider = buildOmpProvider({
      channelType: 'new-api',
      platform: 'anthropic',
      baseUrl: 'https://api.example.com',
      apiKey: 'sk-new',
      models: [
        channelModel({
          baseUrl: 'https://api.example.com',
          apiKey: 'sk-new',
          provider: 'anthropic',
        }),
      ],
      existing,
    })

    expect(provider).toMatchObject({
      baseUrl: 'https://api.example.com',
      api: 'anthropic-messages',
      apiKey: 'sk-new',
      auth: 'apiKey',
      headers: { 'X-Custom': 'keep' },
      disableStrictTools: true,
    })
    expect(provider.models).toHaveLength(1)
    expect(provider.models[0]).toMatchObject({
      id: 'gpt-5.2',
      name: registryModel.name,
    })
  })
})
