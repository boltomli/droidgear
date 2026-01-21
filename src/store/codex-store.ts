import { create } from 'zustand'
import { devtools } from 'zustand/middleware'
import {
  commands,
  type CodexProfile,
  type CodexConfigStatus,
  type CodexCurrentConfig,
  type JsonValue,
} from '@/lib/bindings'

interface CodexState {
  profiles: CodexProfile[]
  activeProfileId: string | null
  currentProfile: CodexProfile | null
  originalProfile: CodexProfile | null
  hasChanges: boolean
  isLoading: boolean
  error: string | null
  configStatus: CodexConfigStatus | null

  loadProfiles: () => Promise<void>
  loadActiveProfileId: () => Promise<void>
  loadConfigStatus: () => Promise<void>
  selectProfile: (id: string) => void
  createProfile: (name: string) => Promise<void>
  saveProfile: () => Promise<void>
  deleteProfile: (id: string) => Promise<void>
  duplicateProfile: (id: string, newName: string) => Promise<void>
  applyProfile: (id: string) => Promise<void>
  loadFromLiveConfig: () => Promise<void>
  updateProfileName: (name: string) => void
  updateProfileDescription: (description: string) => void
  updateAuthValue: (key: string, value: JsonValue) => void
  updateConfigToml: (toml: string) => void
  resetChanges: () => void
  setError: (error: string | null) => void
}

function profilesEqual(a: CodexProfile | null, b: CodexProfile | null): boolean {
  if (!a || !b) return a === b
  return JSON.stringify(a) === JSON.stringify(b)
}

export const useCodexStore = create<CodexState>()(
  devtools(
    (set, get) => ({
      profiles: [],
      activeProfileId: null,
      currentProfile: null,
      originalProfile: null,
      hasChanges: false,
      isLoading: false,
      error: null,
      configStatus: null,

      loadProfiles: async () => {
        set({ isLoading: true, error: null }, undefined, 'codex/loadProfiles/start')
        try {
          const result = await commands.listCodexProfiles()
          if (result.status === 'ok') {
            let profiles = result.data
            if (profiles.length === 0) {
              const created = await commands.createDefaultCodexProfile()
              if (created.status === 'ok') {
                profiles = [created.data]
              }
            }
            set({ profiles, isLoading: false }, undefined, 'codex/loadProfiles/success')
          } else {
            set({ error: result.error, isLoading: false }, undefined, 'codex/loadProfiles/error')
          }
        } catch (e) {
          set({ error: String(e), isLoading: false }, undefined, 'codex/loadProfiles/exception')
        }
      },

      loadActiveProfileId: async () => {
        try {
          const result = await commands.getActiveCodexProfileId()
          if (result.status === 'ok') {
            set({ activeProfileId: result.data }, undefined, 'codex/loadActiveProfileId')
            if (result.data) get().selectProfile(result.data)
          }
        } catch {
          // ignore
        }
      },

      loadConfigStatus: async () => {
        try {
          const result = await commands.getCodexConfigStatus()
          if (result.status === 'ok') {
            set({ configStatus: result.data }, undefined, 'codex/loadConfigStatus')
          }
        } catch {
          // ignore
        }
      },

      selectProfile: id => {
        const profile = get().profiles.find(p => p.id === id) || null
        set(
          {
            currentProfile: profile ? JSON.parse(JSON.stringify(profile)) : null,
            originalProfile: profile ? JSON.parse(JSON.stringify(profile)) : null,
            hasChanges: false,
          },
          undefined,
          'codex/selectProfile'
        )
      },

      createProfile: async name => {
        const now = new Date().toISOString()
        const profile: CodexProfile = {
          id: '',
          name,
          description: '',
          createdAt: now,
          updatedAt: now,
          auth: { OPENAI_API_KEY: '' } as Record<string, JsonValue>,
          configToml: '',
        }
        const result = await commands.saveCodexProfile(profile)
        if (result.status !== 'ok') throw new Error(result.error)
        await get().loadProfiles()
      },

      saveProfile: async () => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const result = await commands.saveCodexProfile(currentProfile)
        if (result.status !== 'ok') {
          set({ error: result.error }, undefined, 'codex/saveProfile/error')
          return
        }
        await get().loadProfiles()
        get().selectProfile(currentProfile.id)
      },

      deleteProfile: async id => {
        const result = await commands.deleteCodexProfile(id)
        if (result.status !== 'ok') {
          set({ error: result.error }, undefined, 'codex/deleteProfile/error')
          return
        }
        await get().loadProfiles()
        const next = get().profiles[0]?.id || null
        if (next) get().selectProfile(next)
      },

      duplicateProfile: async (id, newName) => {
        const result = await commands.duplicateCodexProfile(id, newName)
        if (result.status !== 'ok') {
          set({ error: result.error }, undefined, 'codex/duplicateProfile/error')
          return
        }
        await get().loadProfiles()
        get().selectProfile(result.data.id)
      },

      applyProfile: async id => {
        const result = await commands.applyCodexProfile(id)
        if (result.status !== 'ok') {
          set({ error: result.error }, undefined, 'codex/applyProfile/error')
          return
        }
        set({ activeProfileId: id }, undefined, 'codex/applyProfile/success')
        await get().loadConfigStatus()
      },

      loadFromLiveConfig: async () => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const result = await commands.readCodexCurrentConfig()
        if (result.status !== 'ok') {
          set({ error: result.error }, undefined, 'codex/loadFromLiveConfig/error')
          return
        }
        const live: CodexCurrentConfig = result.data
        const updated: CodexProfile = {
          ...currentProfile,
          auth: live.auth as Record<string, JsonValue>,
          configToml: live.configToml,
          updatedAt: new Date().toISOString(),
        }
        set(
          {
            currentProfile: updated,
            hasChanges: !profilesEqual(updated, get().originalProfile),
          },
          undefined,
          'codex/loadFromLiveConfig/success'
        )
      },

      updateProfileName: name => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const updated = { ...currentProfile, name, updatedAt: new Date().toISOString() }
        set(
          { currentProfile: updated, hasChanges: !profilesEqual(updated, get().originalProfile) },
          undefined,
          'codex/updateProfileName'
        )
      },

      updateProfileDescription: description => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const updated = { ...currentProfile, description, updatedAt: new Date().toISOString() }
        set(
          { currentProfile: updated, hasChanges: !profilesEqual(updated, get().originalProfile) },
          undefined,
          'codex/updateProfileDescription'
        )
      },

      updateAuthValue: (key, value) => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const auth = { ...(currentProfile.auth as Record<string, JsonValue>) }
        auth[key] = value
        const updated = { ...currentProfile, auth, updatedAt: new Date().toISOString() }
        set(
          { currentProfile: updated, hasChanges: !profilesEqual(updated, get().originalProfile) },
          undefined,
          'codex/updateAuthValue'
        )
      },

      updateConfigToml: toml => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const updated = { ...currentProfile, configToml: toml, updatedAt: new Date().toISOString() }
        set(
          { currentProfile: updated, hasChanges: !profilesEqual(updated, get().originalProfile) },
          undefined,
          'codex/updateConfigToml'
        )
      },

      resetChanges: () => {
        const { originalProfile } = get()
        set(
          {
            currentProfile: originalProfile ? JSON.parse(JSON.stringify(originalProfile)) : null,
            hasChanges: false,
            error: null,
          },
          undefined,
          'codex/resetChanges'
        )
      },

      setError: error => set({ error }, undefined, 'codex/setError'),
    }),
    { name: 'codex-store' }
  )
)

