import type {
  ChannelType,
  CustomModel,
  OmpCustomModel,
  OmpCustomProvider,
} from '@/lib/bindings'
import { findModelByIdOrAlias } from '@/lib/model-registry'
import {
  ensureOpenAICompatibleV1,
  needsOpenAICompatibleV1,
  normalizeBaseUrl,
} from '@/lib/sub2api-platform'

/**
 * API protocol identifiers accepted by OMP's `models.yml`.
 */
export type OmpApiType =
  | 'openai-completions'
  | 'openai-responses'
  | 'anthropic-messages'
  | 'google-generative-ai'

const DEFAULT_CONTEXT_WINDOW = 128000
const DEFAULT_MAX_TOKENS = 16384

/** Provider IDs live in YAML keys, so keep them to a safe slug. */
export function sanitizeProviderId(raw: string): string {
  return raw
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

/**
 * Infer the OMP `api` protocol from the channel platform.
 *
 * Mirrors the Pi importer: an untagged platform is treated as OpenAI
 * Chat Completions, while `antigravity` is decided per model.
 */
export function inferOmpApiType(
  platform: string | null | undefined,
  modelId?: string
): OmpApiType {
  const platformLower = platform?.toLowerCase()
  if (platformLower === 'anthropic' || platformLower === 'claude')
    return 'anthropic-messages'
  if (platformLower === 'gemini') return 'google-generative-ai'
  if (platformLower === 'antigravity') {
    return (modelId ?? '').toLowerCase().startsWith('claude-')
      ? 'anthropic-messages'
      : 'openai-completions'
  }
  return 'openai-completions'
}

/**
 * Normalize the channel base URL for the inferred OMP protocol.
 *
 * `sub-2-api` channels serving Chat Completions and the generic compatible
 * protocol must carry the `/v1` suffix; Anthropic / Gemini / Responses keep
 * the channel base URL as-is.
 */
export function normalizeOmpBaseUrl(
  channelType: ChannelType,
  baseUrl: string,
  platform: string | null | undefined
): string {
  const api = inferOmpApiType(platform)
  if (
    platform !== 'antigravity' &&
    needsOpenAICompatibleV1(
      channelType,
      undefined,
      api === 'openai-completions'
    )
  ) {
    return ensureOpenAICompatibleV1(baseUrl)
  }
  if (!platform || platform === 'openai') {
    return normalizeBaseUrl(baseUrl, '/v1')
  }
  return baseUrl.replace(/\/+$/, '')
}

/** Suggest a non-conflicting provider ID derived from the channel/token name. */
export function computeDefaultProviderId(
  channelName: string,
  tokenName: string | undefined,
  existingProviderIds: readonly string[]
): string {
  const base = sanitizeProviderId(
    tokenName ? `${channelName}-${tokenName}` : channelName
  )
  const fallback = base || 'imported-provider'
  if (!existingProviderIds.includes(fallback)) return fallback
  let i = 2
  while (existingProviderIds.includes(`${fallback}-${i}`)) i++
  return `${fallback}-${i}`
}

/**
 * Build an `OmpCustomModel` from a channel model, enriching specs
 * (name / reasoning / input / limits) from the local model registry.
 */
export function createOmpModelFromChannel(model: CustomModel): OmpCustomModel {
  const entry = findModelByIdOrAlias(model.model)
  const registryInput = entry?.input
  const input = registryInput
    ? registryInput.filter(
        modality => !(model.noImageSupport && modality === 'image')
      )
    : undefined
  return {
    id: model.model,
    name: entry?.name ?? model.displayName ?? undefined,
    reasoning: entry?.reasoning ?? false,
    input,
    contextWindow: entry?.contextWindow ?? DEFAULT_CONTEXT_WINDOW,
    maxTokens:
      entry?.maxOutputTokens ?? model.maxOutputTokens ?? DEFAULT_MAX_TOKENS,
  }
}

/**
 * Merge imported models into the existing list by id. Imported specs win,
 * unknown fields already present on a model are preserved.
 */
export function mergeOmpModels(
  existing: readonly OmpCustomModel[],
  imported: readonly OmpCustomModel[]
): OmpCustomModel[] {
  const byId = new Map(existing.map(model => [model.id, model]))
  for (const model of imported) {
    byId.set(model.id, { ...byId.get(model.id), ...model })
  }
  return Array.from(byId.values())
}

export interface BuildOmpProviderParams {
  channelType: ChannelType
  /** Channel platform of the selected token (null for api-key channels). */
  platform: string | null | undefined
  baseUrl: string
  apiKey: string
  /** Models selected in the dialog. */
  models: readonly CustomModel[]
  /** Existing provider when importing into an already-configured provider. */
  existing?: OmpCustomProvider | null
}

/**
 * Assemble the `models.yml` provider entry for a channel import. Unknown
 * fields of an existing provider are preserved so a re-import only touches
 * credentials, protocol and models.
 */
export function buildOmpProvider({
  channelType,
  platform,
  baseUrl,
  apiKey,
  models,
  existing,
}: BuildOmpProviderParams): OmpCustomProvider {
  const imported = models.map(model => createOmpModelFromChannel(model))
  return {
    ...(existing ?? {}),
    baseUrl: normalizeOmpBaseUrl(channelType, baseUrl, platform),
    api: inferOmpApiType(platform, models[0]?.model),
    apiKey,
    auth: existing?.auth ?? 'apiKey',
    models: mergeOmpModels(existing?.models ?? [], imported),
  }
}
