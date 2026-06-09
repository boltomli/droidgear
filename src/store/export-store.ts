import { create } from 'zustand'
import { devtools } from 'zustand/middleware'
import {
  commands,
  type ExportTemplate,
  type ExportResult,
} from '@/lib/bindings'

interface ExportStoreState {
  templates: ExportTemplate[]
  isLoading: boolean
  error: string | null
  runningTemplate: string | null
  lastResult: ExportResult | null

  loadTemplates: () => Promise<void>
  saveTemplate: (template: ExportTemplate) => Promise<void>
  deleteTemplate: (name: string) => Promise<void>
  runTemplate: (name: string) => Promise<void>
  clearResult: () => void
  clearError: () => void
}

export const useExportStore = create<ExportStoreState>()(
  devtools(
    (set, get) => ({
      templates: [],
      isLoading: false,
      error: null,
      runningTemplate: null,
      lastResult: null,

      loadTemplates: async () => {
        set({ isLoading: true, error: null }, undefined, 'loadTemplates/start')
        try {
          const result = await commands.loadExportTemplates()
          if (result.status === 'ok') {
            set(
              { templates: result.data, isLoading: false },
              undefined,
              'loadTemplates/success'
            )
          } else {
            set(
              { error: result.error, isLoading: false },
              undefined,
              'loadTemplates/error'
            )
          }
        } catch (e) {
          set(
            { error: String(e), isLoading: false },
            undefined,
            'loadTemplates/exception'
          )
        }
      },

      saveTemplate: async (template: ExportTemplate) => {
        set({ isLoading: true, error: null }, undefined, 'saveTemplate/start')
        try {
          const result = await commands.saveExportTemplate(template)
          if (result.status === 'ok') {
            // Reload all templates to get fresh state
            await get().loadTemplates()
          } else {
            set(
              { error: result.error, isLoading: false },
              undefined,
              'saveTemplate/error'
            )
          }
        } catch (e) {
          set(
            { error: String(e), isLoading: false },
            undefined,
            'saveTemplate/exception'
          )
        }
      },

      deleteTemplate: async (name: string) => {
        set({ isLoading: true, error: null }, undefined, 'deleteTemplate/start')
        try {
          const result = await commands.deleteExportTemplate(name)
          if (result.status === 'ok') {
            await get().loadTemplates()
          } else {
            set(
              { error: result.error, isLoading: false },
              undefined,
              'deleteTemplate/error'
            )
          }
        } catch (e) {
          set(
            { error: String(e), isLoading: false },
            undefined,
            'deleteTemplate/exception'
          )
        }
      },

      runTemplate: async (name: string) => {
        set(
          { runningTemplate: name, error: null, lastResult: null },
          undefined,
          'runTemplate/start'
        )
        try {
          const result = await commands.runExportTemplate(name)
          if (result.status === 'ok') {
            set(
              {
                runningTemplate: null,
                lastResult: result.data,
              },
              undefined,
              'runTemplate/success'
            )
          } else {
            set(
              { runningTemplate: null, error: result.error },
              undefined,
              'runTemplate/error'
            )
          }
        } catch (e) {
          set(
            { runningTemplate: null, error: String(e) },
            undefined,
            'runTemplate/exception'
          )
        }
      },

      clearResult: () => set({ lastResult: null }, undefined, 'clearResult'),

      clearError: () => set({ error: null }, undefined, 'clearError'),
    }),
    { name: 'export-store' }
  )
)
