import {
  findModelByIdOrAlias,
  type ModelRegistryEntry,
} from '@/lib/model-registry'
import type { OmpModel } from '@/lib/bindings'

export function createOmpModelFromRegistry(
  entry: ModelRegistryEntry
): OmpModel {
  return {
    id: entry.id,
    name: entry.name,
    api: null,
    reasoning: entry.reasoning,
    input: entry.input,
    thinkingLevelMap: entry.thinkingLevelMap
      ? { ...entry.thinkingLevelMap }
      : null,
    contextWindow: entry.contextWindow,
    maxTokens: entry.maxOutputTokens,
    cost: null,
    compat: null,
  }
}

export function enrichOmpModelFromRegistry(model: OmpModel): OmpModel {
  const entry = findModelByIdOrAlias(model.id)
  if (!entry) return model

  return {
    ...model,
    name: entry.name,
    reasoning: entry.reasoning,
    input: entry.input,
    thinkingLevelMap: entry.thinkingLevelMap
      ? { ...entry.thinkingLevelMap }
      : null,
    contextWindow: entry.contextWindow,
    maxTokens: entry.maxOutputTokens ?? model.maxTokens,
  }
}
