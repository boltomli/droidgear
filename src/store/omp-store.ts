import { create } from 'zustand'
import { devtools } from 'zustand/middleware'
import {
  commands,
  type OmpProfile,
  type OmpProviderConfig,
  type OmpConfigStatus,
  type OmpCurrentConfig,
} from '@/lib/bindings'

interface OmpState {
  profiles: OmpProfile[]
  activeProfileId: string | null
  currentProfile: OmpProfile | null
  isLoading: boolean
  error: string | null
  configStatus: OmpConfigStatus | null

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
  addProvider: (id: string, config: OmpProviderConfig) => Promise<void>
  updateProvider: (id: string, config: OmpProviderConfig) => Promise<void>
  deleteProvider: (id: string) => Promise<void>
  setError: (error: string | null) => void
}

export const useOmpStore = create<OmpState>()(
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
          'omp/loadProfiles/start'
        )
        try {
          const result = await commands.listOmpProfiles()
          if (result.status === 'ok') {
            let profiles = result.data
            if (profiles.length === 0) {
              const created = await commands.createDefaultOmpProfile()
              if (created.status === 'ok') {
                const refreshed = await commands.listOmpProfiles()
                profiles =
                  refreshed.status === 'ok' ? refreshed.data : [created.data]
              }
            }
            set(
              { profiles, isLoading: false },
              undefined,
              'omp/loadProfiles/success'
            )
          } else {
            set(
              { error: result.error, isLoading: false },
              undefined,
              'omp/loadProfiles/error'
            )
          }
        } catch (e) {
          set(
            { error: String(e), isLoading: false },
            undefined,
            'omp/loadProfiles/exception'
          )
        }
      },

      loadActiveProfileId: async () => {
        try {
          const result = await commands.getActiveOmpProfileId()
          if (result.status === 'ok') {
            const activeId = result.data
            set(
              { activeProfileId: activeId },
              undefined,
              'omp/loadActiveProfileId'
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
          const result = await commands.getOmpConfigStatus()
          if (result.status === 'ok') {
            set(
              { configStatus: result.data },
              undefined,
              'omp/loadConfigStatus'
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
          'omp/selectProfile'
        )
      },

      createProfile: async name => {
        const now = new Date().toISOString()
        const profile: OmpProfile = {
          id: '',
          name,
          description: null,
          createdAt: now,
          updatedAt: now,
          providers: {},
        }
        const result = await commands.saveOmpProfile(profile)
        if (result.status !== 'ok') throw new Error(result.error)
        await get().loadProfiles()
      },

      saveProfile: async () => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const result = await commands.saveOmpProfile(currentProfile)
        if (result.status !== 'ok') {
          set({ error: result.error }, undefined, 'omp/saveProfile/error')
          return
        }
        await get().loadProfiles()
        get().selectProfile(currentProfile.id)
      },

      deleteProfile: async id => {
        const result = await commands.deleteOmpProfile(id)
        if (result.status !== 'ok') {
          set({ error: result.error }, undefined, 'omp/deleteProfile/error')
          return
        }
        await get().loadProfiles()
        const next = get().profiles[0]?.id || null
        if (next) get().selectProfile(next)
      },

      duplicateProfile: async (id, newName) => {
        const result = await commands.duplicateOmpProfile(id, newName)
        if (result.status !== 'ok') {
          set({ error: result.error }, undefined, 'omp/duplicateProfile/error')
          return
        }
        await get().loadProfiles()
        get().selectProfile(result.data.id)
      },

      applyProfile: async id => {
        // Ensure the current profile is saved to disk before applying
        const { currentProfile } = get()
        if (currentProfile && currentProfile.id === id) {
          const saveResult = await commands.saveOmpProfile(currentProfile)
          if (saveResult.status !== 'ok') {
            set(
              { error: saveResult.error },
              undefined,
              'omp/applyProfile/saveError'
            )
            return
          }
        }
        const result = await commands.applyOmpProfile(id)
        if (result.status !== 'ok') {
          set({ error: result.error }, undefined, 'omp/applyProfile/error')
          return
        }
        set({ activeProfileId: id }, undefined, 'omp/applyProfile/success')
        await get().loadConfigStatus()
      },

      loadFromLiveConfig: async () => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const result = await commands.readOmpCurrentConfig()
        if (result.status !== 'ok') {
          set(
            { error: result.error },
            undefined,
            'omp/loadFromLiveConfig/error'
          )
          return
        }
        const live: OmpCurrentConfig = result.data
        const updated: OmpProfile = {
          ...currentProfile,
          providers: live.providers || {},
          updatedAt: new Date().toISOString(),
        }
        set(
          { currentProfile: updated },
          undefined,
          'omp/loadFromLiveConfig/success'
        )
        await get().saveProfile()
      },

      addProvider: async (id, config) => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const updated: OmpProfile = {
          ...currentProfile,
          providers: { ...(currentProfile.providers ?? {}), [id]: config },
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'omp/addProvider')
        await get().saveProfile()
      },

      updateProvider: async (id, config) => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const updated: OmpProfile = {
          ...currentProfile,
          providers: { ...(currentProfile.providers ?? {}), [id]: config },
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'omp/updateProvider')
        await get().saveProfile()
      },

      deleteProvider: async id => {
        const { currentProfile } = get()
        if (!currentProfile) return
        const { [id]: _removed, ...providers } = currentProfile.providers ?? {}
        const updated: OmpProfile = {
          ...currentProfile,
          providers,
          updatedAt: new Date().toISOString(),
        }
        set({ currentProfile: updated }, undefined, 'omp/deleteProvider')
        await get().saveProfile()
      },

      setError: error => set({ error }, undefined, 'omp/setError'),
    }),
    { name: 'omp-store' }
  )
)
