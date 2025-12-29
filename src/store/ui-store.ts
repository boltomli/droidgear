import { create } from 'zustand'
import { devtools, persist } from 'zustand/middleware'

type NavigationView = 'droid' | 'channels'
export type DroidSubView = 'models' | 'helpers' | 'specs'

interface UIState {
  leftSidebarVisible: boolean
  rightSidebarVisible: boolean
  commandPaletteOpen: boolean
  preferencesOpen: boolean
  lastQuickPaneEntry: string | null
  currentView: NavigationView
  droidSubView: DroidSubView
  lastSpecExportPath: string | null

  toggleLeftSidebar: () => void
  setLeftSidebarVisible: (visible: boolean) => void
  toggleRightSidebar: () => void
  setRightSidebarVisible: (visible: boolean) => void
  toggleCommandPalette: () => void
  setCommandPaletteOpen: (open: boolean) => void
  togglePreferences: () => void
  setPreferencesOpen: (open: boolean) => void
  setLastQuickPaneEntry: (text: string) => void
  setCurrentView: (view: NavigationView) => void
  setDroidSubView: (view: DroidSubView) => void
  setLastSpecExportPath: (path: string) => void
}

export const useUIStore = create<UIState>()(
  devtools(
    persist(
      set => ({
        leftSidebarVisible: true,
        rightSidebarVisible: false,
        commandPaletteOpen: false,
        preferencesOpen: false,
        lastQuickPaneEntry: null,
        currentView: 'channels',
        droidSubView: 'models',
        lastSpecExportPath: null,

        toggleLeftSidebar: () =>
          set(
            state => ({ leftSidebarVisible: !state.leftSidebarVisible }),
            undefined,
            'toggleLeftSidebar'
          ),

        setLeftSidebarVisible: visible =>
          set(
            { leftSidebarVisible: visible },
            undefined,
            'setLeftSidebarVisible'
          ),

        toggleRightSidebar: () =>
          set(
            state => ({ rightSidebarVisible: !state.rightSidebarVisible }),
            undefined,
            'toggleRightSidebar'
          ),

        setRightSidebarVisible: visible =>
          set(
            { rightSidebarVisible: visible },
            undefined,
            'setRightSidebarVisible'
          ),

        toggleCommandPalette: () =>
          set(
            state => ({ commandPaletteOpen: !state.commandPaletteOpen }),
            undefined,
            'toggleCommandPalette'
          ),

        setCommandPaletteOpen: open =>
          set({ commandPaletteOpen: open }, undefined, 'setCommandPaletteOpen'),

        togglePreferences: () =>
          set(
            state => ({ preferencesOpen: !state.preferencesOpen }),
            undefined,
            'togglePreferences'
          ),

        setPreferencesOpen: open =>
          set({ preferencesOpen: open }, undefined, 'setPreferencesOpen'),

        setLastQuickPaneEntry: text =>
          set({ lastQuickPaneEntry: text }, undefined, 'setLastQuickPaneEntry'),

        setCurrentView: view =>
          set({ currentView: view }, undefined, 'setCurrentView'),

        setDroidSubView: view =>
          set({ droidSubView: view }, undefined, 'setDroidSubView'),

        setLastSpecExportPath: path =>
          set({ lastSpecExportPath: path }, undefined, 'setLastSpecExportPath'),
      }),
      {
        name: 'ui-store',
        partialize: state => ({
          lastSpecExportPath: state.lastSpecExportPath,
        }),
      }
    ),
    {
      name: 'ui-store',
    }
  )
)
