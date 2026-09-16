import { create } from 'zustand'
import { devtools } from 'zustand/middleware'
import {
  commands,
  type OpenClawSubAgent,
  type OpenClawSubAgentSubagentsConfig,
} from '@/lib/bindings'

interface SubagentsState {
  subagents: OpenClawSubAgent[]
  isLoading: boolean
  error: string | null

  loadSubagents: () => Promise<void>
  saveSubagents: () => Promise<void>
  addSubagent: (subagent: OpenClawSubAgent) => Promise<void>
  updateSubagent: (id: string, updated: OpenClawSubAgent) => Promise<void>
  deleteSubagent: (id: string) => Promise<void>
  toggleSubagentAllow: (id: string) => Promise<void>
  setError: (error: string | null) => void
}

/**
 * Helper: update the main agent's allowAgents list within a subagents array.
 * Returns a new array with the main agent's allowAgents replaced.
 */
function updateMainAllowAgents(
  agents: OpenClawSubAgent[],
  updater: (current: string[]) => string[]
): OpenClawSubAgent[] {
  return agents.map(a => {
    if (a.id !== 'main') return a
    const current = a.subagents?.allowAgents ?? []
    const subagentsConfig: OpenClawSubAgentSubagentsConfig = {
      allowAgents: updater(current),
      maxConcurrent: a.subagents?.maxConcurrent ?? null,
    }
    return { ...a, subagents: subagentsConfig } as OpenClawSubAgent
  })
}

export const useSubagentsStore = create<SubagentsState>()(
  devtools(
    (set, get) => ({
      subagents: [],
      isLoading: false,
      error: null,

      loadSubagents: async () => {
        set({ isLoading: true, error: null }, undefined, 'subagents/load/start')
        try {
          const result = await commands.readOpenclawSubagents()
          if (result.status === 'ok') {
            set(
              { subagents: result.data, isLoading: false },
              undefined,
              'subagents/load/success'
            )
          } else {
            set(
              { error: result.error, isLoading: false },
              undefined,
              'subagents/load/error'
            )
          }
        } catch (e) {
          set(
            { error: String(e), isLoading: false },
            undefined,
            'subagents/load/exception'
          )
        }
      },

      saveSubagents: async () => {
        const { subagents } = get()
        const result = await commands.saveOpenclawSubagents(subagents)
        if (result.status !== 'ok') {
          set({ error: result.error }, undefined, 'subagents/save/error')
        }
      },

      addSubagent: async subagent => {
        const { subagents } = get()
        // Check for duplicate id
        if (subagents.some(a => a.id === subagent.id)) {
          set(
            { error: `Subagent with id "${subagent.id}" already exists` },
            undefined,
            'subagents/add/duplicate'
          )
          return
        }
        // Add subagent and auto-add to main's allowAgents
        const updated = updateMainAllowAgents(
          [...subagents, subagent],
          current => [...current, subagent.id]
        )
        set({ subagents: updated }, undefined, 'subagents/add')
        await get().saveSubagents()
      },

      updateSubagent: async (id, updatedAgent) => {
        const { subagents } = get()
        const updated = subagents.map(a => (a.id === id ? updatedAgent : a))
        set({ subagents: updated }, undefined, 'subagents/update')
        await get().saveSubagents()
      },

      deleteSubagent: async id => {
        const { subagents } = get()
        // Remove subagent and also remove from main's allowAgents
        const updated = updateMainAllowAgents(
          subagents.filter(a => a.id !== id),
          current => current.filter(a => a !== id)
        )
        set({ subagents: updated }, undefined, 'subagents/delete')
        await get().saveSubagents()
      },

      toggleSubagentAllow: async id => {
        const { subagents } = get()
        const hasMain = subagents.some(a => a.id === 'main')

        if (hasMain) {
          const updated = updateMainAllowAgents(subagents, current =>
            current.includes(id)
              ? current.filter(a => a !== id)
              : [...current, id]
          )
          set({ subagents: updated }, undefined, 'subagents/toggleAllow')
        } else {
          // No main entry yet, create one
          const mainAgent: OpenClawSubAgent = {
            id: 'main',
            subagents: { allowAgents: [id] },
          }
          set(
            { subagents: [mainAgent, ...subagents] },
            undefined,
            'subagents/toggleAllow'
          )
        }

        await get().saveSubagents()
      },

      setError: error => set({ error }, undefined, 'subagents/setError'),
    }),
    { name: 'subagents-store' }
  )
)
