import { create } from 'zustand'
import { devtools } from 'zustand/middleware'
import {
  commands,
  type ZedProfile,
  type ZedProviderConfig,
  type ZedModel,
  type ZedConfigStatus,
} from '@/lib/bindings'

interface ZedState {
  profiles: ZedProfile[]
  activeProfileId: string | null
  currentProfile: ZedProfile | null
  isLoading: boolean
  error: string | null
  configStatus: ZedConfigStatus | null

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
  updateProfileName: (name: string) => Promise<void>
  updateProfileDescription: (description: string) => Promise<void>
  addProvider: (id: string, config: ZedProviderConfig) => Promise<void>
  updateProvider: (id: string, config: ZedProviderConfig) => Promise<void>
  deleteProvider: (id: string) => Promise<void>
  addModel: (providerId: string, model: ZedModel) => Promise<void>
  updateModel: (
    providerId: string,
    oldModelName: string,
    model: ZedModel
  ) => Promise<void>
  deleteModel: (providerId: string, modelName: string) => Promise<void>
  importProviderFromChannel: (
    channelName: string,
    baseUrl: string,
    apiKey: string,
    models: ZedModel[]
  ) => Promise<void>
  setError: (error: string | null) => void
}

export const useZedStore = create<ZedState>()(
  devtools(
    (set, get) => ({
      profiles: [],
      activeProfileId: null,
      currentProfile: null,
      isLoading: false,
      error: null,
      configStatus: null,

      loadProfiles: async () => {
        set(
          { isLoading: true, error: null },
          undefined,
          'zed/loadProfiles/start'
        )
        try {
          const result = await commands.listZedProfilesCmd()
          if (result.status === 'ok') {
            let profiles = result.data
            // Auto-create default profile if none exist
            if (profiles.length === 0) {
              const created = await commands.createDefaultZedProfileCmd()
              if (created.status === 'ok') {
                const refreshed = await commands.listZedProfilesCmd()
                profiles =
                  refreshed.status === 'ok' ? refreshed.data : [created.data]
              }
            }
            set(
              { profiles, isLoading: false },
              undefined,
              'zed/loadProfiles/success'
            )
          } else {
            set(
              { error: result.error, isLoading: false },
              undefined,
              'zed/loadProfiles/error'
            )
          }
        } catch (e) {
          set(
            { error: String(e), isLoading: false },
            undefined,
            'zed/loadProfiles/exception'
          )
        }
      },

      loadActiveProfileId: async () => {
        try {
          const result = await commands.getActiveZedProfileIdCmd()
          if (result.status === 'ok') {
            const activeId = result.data
            set(
              { activeProfileId: activeId },
              undefined,
              'zed/loadActiveProfileId'
            )
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
          // ignore
        }
      },

      loadConfigStatus: async () => {
        try {
          const result = await commands.getZedConfigStatusCmd()
          if (result.status === 'ok') {
            set(
              { configStatus: result.data },
              undefined,
              'zed/loadConfigStatus'
            )
          }
        } catch {
          // ignore
        }
      },

      selectProfile: id => {
        const profile = get().profiles.find(p => p.id === id) || null
        set(
          {
            currentProfile: profile
              ? JSON.parse(JSON.stringify(profile))
              : null,
          },
          undefined,
          'zed/selectProfile'
        )
      },

      createProfile: async name => {
        const now = new Date().toISOString()
        const profile: ZedProfile = {
          id: '',
          name,
          description: null,
          createdAt: now,
          updatedAt: now,
          providers: {},
          apiKeys: null,
        }
        const result = await commands.saveZedProfileCmd(profile)
        if (result.status !== 'ok') {
          set({ error: result.error }, undefined, 'zed/createProfile/error')
          return
        }
        await get().loadProfiles()
        // Auto-select the newly created profile
        const { profiles } = get()
        const created = profiles.find(p => p.name === name)
        if (created) {
          get().selectProfile(created.id)
        }
      },

      saveProfile: async () => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const result = await commands.saveZedProfileCmd(currentProfile)
        if (result.status !== 'ok') {
          set({ error: result.error }, undefined, 'zed/saveProfile/error')
          return
        }
        await get().loadProfiles()
        get().selectProfile(currentProfile.id)
      },

      deleteProfile: async id => {
        const result = await commands.deleteZedProfileCmd(id)
        if (result.status !== 'ok') {
          set({ error: result.error }, undefined, 'zed/deleteProfile/error')
          return
        }
        await get().loadProfiles()
        const { profiles, currentProfile } = get()
        // If the deleted profile was the current one, select the next available
        if (currentProfile?.id === id) {
          const next = profiles[0]?.id || null
          if (next) {
            get().selectProfile(next)
          } else {
            set({ currentProfile: null }, undefined, 'zed/deleteProfile/clear')
          }
        }
      },

      duplicateProfile: async (id, newName) => {
        const result = await commands.duplicateZedProfileCmd(id, newName)
        if (result.status !== 'ok') {
          set({ error: result.error }, undefined, 'zed/duplicateProfile/error')
          return
        }
        await get().loadProfiles()
        get().selectProfile(result.data.id)
      },

      applyProfile: async id => {
        // Ensure the current profile is saved to disk before applying
        const { currentProfile } = get()
        if (currentProfile && currentProfile.id === id) {
          const saveResult = await commands.saveZedProfileCmd(currentProfile)
          if (saveResult.status !== 'ok') {
            set(
              { error: saveResult.error },
              undefined,
              'zed/applyProfile/saveError'
            )
            return
          }
        }
        const result = await commands.applyZedProfileCmd(id)
        if (result.status !== 'ok') {
          set({ error: result.error }, undefined, 'zed/applyProfile/error')
          return
        }
        set({ activeProfileId: id }, undefined, 'zed/applyProfile/success')
        await get().loadConfigStatus()
      },

      loadFromLiveConfig: async () => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const result = await commands.readZedCurrentConfigCmd()
        if (result.status !== 'ok') {
          set(
            { error: result.error },
            undefined,
            'zed/loadFromLiveConfig/error'
          )
          return
        }
        const live = result.data
        const updated: ZedProfile = {
          ...currentProfile,
          providers: live.providers || {},
          updatedAt: new Date().toISOString(),
        }
        set(
          { currentProfile: updated },
          undefined,
          'zed/loadFromLiveConfig/success'
        )
        await get().saveProfile()
      },

      updateProfileName: async name => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const updated: ZedProfile = {
          ...currentProfile,
          name,
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'zed/updateProfileName')
        await get().saveProfile()
      },

      updateProfileDescription: async description => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const updated: ZedProfile = {
          ...currentProfile,
          description: description || null,
          updatedAt: new Date().toISOString(),
        }
        set(
          { currentProfile: updated },
          undefined,
          'zed/updateProfileDescription'
        )
        await get().saveProfile()
      },

      addProvider: async (id, config) => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const updated: ZedProfile = {
          ...currentProfile,
          providers: { ...(currentProfile.providers ?? {}), [id]: config },
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'zed/addProvider')
        await get().saveProfile()
      },

      updateProvider: async (id, config) => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const updated: ZedProfile = {
          ...currentProfile,
          providers: { ...(currentProfile.providers ?? {}), [id]: config },
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'zed/updateProvider')
        await get().saveProfile()
      },

      deleteProvider: async id => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const { [id]: _removed, ...providers } = currentProfile.providers ?? {}
        const { [id]: _removedApiKey, ...apiKeys } =
          currentProfile.apiKeys ?? {}
        const updated: ZedProfile = {
          ...currentProfile,
          providers,
          apiKeys: Object.keys(apiKeys).length > 0 ? apiKeys : null,
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'zed/deleteProvider')
        await get().saveProfile()
      },

      addModel: async (providerId, model) => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const provider = currentProfile.providers?.[providerId]
        if (!provider) return

        const models = provider.availableModels ?? []
        const updatedProvider: ZedProviderConfig = {
          ...provider,
          availableModels: [...models, model],
        }
        const updated: ZedProfile = {
          ...currentProfile,
          providers: {
            ...(currentProfile.providers ?? {}),
            [providerId]: updatedProvider,
          },
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'zed/addModel')
        await get().saveProfile()
      },

      updateModel: async (providerId, oldModelName, model) => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const provider = currentProfile.providers?.[providerId]
        if (!provider) return

        const models = (provider.availableModels ?? []).map(m =>
          m.name === oldModelName ? model : m
        )
        const updatedProvider: ZedProviderConfig = {
          ...provider,
          availableModels: models,
        }
        const updated: ZedProfile = {
          ...currentProfile,
          providers: {
            ...(currentProfile.providers ?? {}),
            [providerId]: updatedProvider,
          },
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'zed/updateModel')
        await get().saveProfile()
      },

      deleteModel: async (providerId, modelName) => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const provider = currentProfile.providers?.[providerId]
        if (!provider) return

        const models = (provider.availableModels ?? []).filter(
          m => m.name !== modelName
        )
        const updatedProvider: ZedProviderConfig = {
          ...provider,
          availableModels: models.length > 0 ? models : null,
        }
        const updated: ZedProfile = {
          ...currentProfile,
          providers: {
            ...(currentProfile.providers ?? {}),
            [providerId]: updatedProvider,
          },
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'zed/deleteModel')
        await get().saveProfile()
      },

      importProviderFromChannel: async (
        channelName,
        baseUrl,
        apiKey,
        models
      ) => {
        // Sanitize name for channel import only: lowercase, special chars → hyphens
        const sanitizedName = channelName
          .toLowerCase()
          .replace(/[^a-z0-9]/g, '-')
          .replace(/-+/g, '-')
          .replace(/^-|-$/g, '')

        const config: ZedProviderConfig = {
          api_url: baseUrl,
          availableModels: models,
        }

        const { currentProfile } = get()
        if (!currentProfile) return

        const updated: ZedProfile = {
          ...currentProfile,
          providers: {
            ...(currentProfile.providers ?? {}),
            [sanitizedName]: config,
          },
          apiKeys: {
            ...(currentProfile.apiKeys ?? {}),
            [sanitizedName]: apiKey,
          },
          updatedAt: new Date().toISOString(),
        }
        set(
          { currentProfile: updated },
          undefined,
          'zed/importProviderFromChannel'
        )
        await get().saveProfile()
      },

      setError: error => set({ error }, undefined, 'zed/setError'),
    }),
    { name: 'zed-store' }
  )
)
