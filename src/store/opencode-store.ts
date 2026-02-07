import { create } from 'zustand'
import { devtools } from 'zustand/middleware'
import {
  commands,
  type OpenCodeProfile,
  type OpenCodeProviderConfig,
  type OpenCodeConfigStatus,
  type ProviderTemplate,
  type JsonValue,
} from '@/lib/bindings'

interface OpenCodeState {
  profiles: OpenCodeProfile[]
  activeProfileId: string | null
  currentProfile: OpenCodeProfile | null
  isLoading: boolean
  error: string | null
  configStatus: OpenCodeConfigStatus | null
  providerTemplates: ProviderTemplate[]

  loadProfiles: () => Promise<void>
  loadActiveProfileId: () => Promise<void>
  loadConfigStatus: () => Promise<void>
  loadProviderTemplates: () => Promise<void>
  selectProfile: (id: string) => void
  createProfile: (name: string) => Promise<void>
  saveProfile: () => Promise<void>
  deleteProfile: (id: string) => Promise<void>
  duplicateProfile: (id: string, newName: string) => Promise<void>
  applyProfile: (id: string) => Promise<void>
  updateProfileName: (name: string) => Promise<void>
  updateProfileDescription: (description: string) => Promise<void>
  addProvider: (
    id: string,
    config: OpenCodeProviderConfig,
    auth?: JsonValue
  ) => Promise<void>
  updateProvider: (
    id: string,
    config: OpenCodeProviderConfig,
    auth?: JsonValue
  ) => Promise<void>
  deleteProvider: (id: string) => Promise<void>
  importProviders: (
    providers: Record<string, OpenCodeProviderConfig | undefined>,
    auth: Record<string, JsonValue | undefined>,
    strategy: 'skip' | 'replace'
  ) => Promise<void>
  setError: (error: string | null) => void
}

export const useOpenCodeStore = create<OpenCodeState>()(
  devtools(
    (set, get) => ({
      profiles: [],
      activeProfileId: null,
      currentProfile: null,
      isLoading: false,
      error: null,
      configStatus: null,
      providerTemplates: [],

      loadProfiles: async () => {
        set({ isLoading: true, error: null }, undefined, 'loadProfiles/start')
        try {
          const result = await commands.listOpencodeProfiles()
          if (result.status === 'ok') {
            let profiles = result.data
            // Create default profile if none exists
            if (profiles.length === 0) {
              const createResult = await commands.createDefaultProfile()
              if (createResult.status === 'ok') {
                profiles = [createResult.data]
              }
            }
            set(
              { profiles, isLoading: false },
              undefined,
              'loadProfiles/success'
            )
          } else {
            set(
              { error: result.error, isLoading: false },
              undefined,
              'loadProfiles/error'
            )
          }
        } catch (e) {
          set(
            { error: String(e), isLoading: false },
            undefined,
            'loadProfiles/exception'
          )
        }
      },

      loadActiveProfileId: async () => {
        try {
          const result = await commands.getActiveOpencodeProfileId()
          if (result.status === 'ok') {
            const activeId = result.data
            set({ activeProfileId: activeId }, undefined, 'loadActiveProfileId')
            // Auto-select active profile
            if (activeId) {
              get().selectProfile(activeId)
            } else {
              // Select first profile if no active
              const { profiles } = get()
              if (profiles.length > 0 && profiles[0]) {
                get().selectProfile(profiles[0].id)
              }
            }
          }
        } catch {
          // Silently ignore
        }
      },

      loadConfigStatus: async () => {
        try {
          const result = await commands.getOpencodeConfigStatus()
          if (result.status === 'ok') {
            set({ configStatus: result.data }, undefined, 'loadConfigStatus')
          }
        } catch {
          // Silently ignore
        }
      },

      loadProviderTemplates: async () => {
        try {
          const result = await commands.getOpencodeProviderTemplates()
          if (result.status === 'ok') {
            set(
              { providerTemplates: result.data },
              undefined,
              'loadProviderTemplates'
            )
          }
        } catch {
          // Silently ignore
        }
      },

      selectProfile: id => {
        const { profiles } = get()
        const profile = profiles.find(p => p.id === id)
        if (profile) {
          set(
            {
              currentProfile: JSON.parse(JSON.stringify(profile)),
            },
            undefined,
            'selectProfile'
          )
        }
      },

      createProfile: async name => {
        const now = new Date().toISOString()
        const newProfile: OpenCodeProfile = {
          id: crypto.randomUUID(),
          name,
          description: null,
          createdAt: now,
          updatedAt: now,
          providers: {},
          auth: {},
        }
        try {
          const result = await commands.saveOpencodeProfile(newProfile)
          if (result.status === 'ok') {
            await get().loadProfiles()
            get().selectProfile(newProfile.id)
          } else {
            set({ error: result.error }, undefined, 'createProfile/error')
          }
        } catch (e) {
          set({ error: String(e) }, undefined, 'createProfile/exception')
        }
      },

      saveProfile: async () => {
        const { currentProfile } = get()
        if (!currentProfile) return

        try {
          const result = await commands.saveOpencodeProfile(currentProfile)
          if (result.status === 'ok') {
            await get().loadProfiles()
            get().selectProfile(currentProfile.id)
          } else {
            set({ error: result.error }, undefined, 'saveProfile/error')
          }
        } catch (e) {
          set({ error: String(e) }, undefined, 'saveProfile/exception')
        }
      },

      deleteProfile: async id => {
        try {
          const result = await commands.deleteOpencodeProfile(id)
          if (result.status === 'ok') {
            const { currentProfile } = get()
            await get().loadProfiles()
            if (currentProfile?.id === id) {
              const { profiles } = get()
              if (profiles.length > 0 && profiles[0]) {
                get().selectProfile(profiles[0].id)
              } else {
                set({ currentProfile: null }, undefined, 'deleteProfile/clear')
              }
            }
          } else {
            set({ error: result.error }, undefined, 'deleteProfile/error')
          }
        } catch (e) {
          set({ error: String(e) }, undefined, 'deleteProfile/exception')
        }
      },

      duplicateProfile: async (id, newName) => {
        try {
          const result = await commands.duplicateOpencodeProfile(id, newName)
          if (result.status === 'ok') {
            await get().loadProfiles()
            get().selectProfile(result.data.id)
          } else {
            set({ error: result.error }, undefined, 'duplicateProfile/error')
          }
        } catch (e) {
          set({ error: String(e) }, undefined, 'duplicateProfile/exception')
        }
      },

      applyProfile: async id => {
        set({ isLoading: true, error: null }, undefined, 'applyProfile/start')
        try {
          const result = await commands.applyOpencodeProfile(id)
          if (result.status === 'ok') {
            set(
              { activeProfileId: id, isLoading: false },
              undefined,
              'applyProfile/success'
            )
            await get().loadConfigStatus()
          } else {
            set(
              { error: result.error, isLoading: false },
              undefined,
              'applyProfile/error'
            )
          }
        } catch (e) {
          set(
            { error: String(e), isLoading: false },
            undefined,
            'applyProfile/exception'
          )
        }
      },

      updateProfileName: async name => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const updated = {
          ...currentProfile,
          name,
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'updateProfileName')
        await get().saveProfile()
      },

      updateProfileDescription: async description => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const updated = {
          ...currentProfile,
          description: description || null,
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'updateProfileDescription')
        await get().saveProfile()
      },

      addProvider: async (id, config, auth) => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const updated = {
          ...currentProfile,
          providers: { ...currentProfile.providers, [id]: config },
          auth: auth
            ? { ...currentProfile.auth, [id]: auth }
            : currentProfile.auth,
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'addProvider')
        await get().saveProfile()
      },

      updateProvider: async (id, config, auth) => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const updated = {
          ...currentProfile,
          providers: { ...currentProfile.providers, [id]: config },
          auth: auth
            ? { ...currentProfile.auth, [id]: auth }
            : currentProfile.auth,
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'updateProvider')
        await get().saveProfile()
      },

      deleteProvider: async id => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const { [id]: _removed, ...providers } = currentProfile.providers
        const { [id]: _removedAuth, ...auth } = currentProfile.auth
        const updated = {
          ...currentProfile,
          providers,
          auth,
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'deleteProvider')
        await get().saveProfile()
      },

      importProviders: async (providers, auth, strategy) => {
        const { currentProfile } = get()
        if (!currentProfile) return

        const newProviders = { ...currentProfile.providers }
        const newAuth = { ...currentProfile.auth }

        for (const [id, config] of Object.entries(providers)) {
          if (!config) continue
          const exists = id in currentProfile.providers
          if (exists && strategy === 'skip') continue
          newProviders[id] = config
        }

        for (const [id, authValue] of Object.entries(auth)) {
          if (authValue === undefined) continue
          const exists = id in currentProfile.auth
          if (exists && strategy === 'skip') continue
          newAuth[id] = authValue
        }

        const updated = {
          ...currentProfile,
          providers: newProviders,
          auth: newAuth,
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'importProviders')
        await get().saveProfile()
      },

      setError: error => set({ error }, undefined, 'setError'),
    }),
    { name: 'opencode-store' }
  )
)
