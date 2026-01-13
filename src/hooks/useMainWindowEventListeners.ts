import { useEffect } from 'react'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { exit } from '@tauri-apps/plugin-process'
import { useCommandContext } from './use-command-context'
import { useKeyboardShortcuts } from './use-keyboard-shortcuts'
import { useModelStore } from '@/store/model-store'
import { logger } from '@/lib/logger'

/**
 * Main window event listeners - handles global keyboard shortcuts and window events.
 *
 * This hook composes specialized hooks for different event types:
 * - useKeyboardShortcuts: Global keyboard shortcuts (Cmd+, Cmd+1, Cmd+2)
 * - Close request listener: Unsaved changes protection
 */
export function useMainWindowEventListeners() {
  const commandContext = useCommandContext()

  useKeyboardShortcuts(commandContext)

  // Listen for window close request - check for unsaved changes
  useEffect(() => {
    let isMounted = true
    let unlisten: (() => void) | null = null

    const appWindow = getCurrentWindow()

    appWindow
      .onCloseRequested(async event => {
        const { hasChanges } = useModelStore.getState()

        if (hasChanges) {
          // Prevent default close behavior
          event.preventDefault()

          // Show confirmation dialog
          const shouldClose = confirm(
            'You have unsaved changes. Are you sure you want to exit?'
          )

          if (shouldClose) {
            // User confirmed, exit the entire app
            await exit(0)
          }
        } else {
          // No unsaved changes, exit the entire app
          await exit(0)
        }
      })
      .then(unlistenFn => {
        if (!isMounted) {
          unlistenFn()
        } else {
          unlisten = unlistenFn
        }
      })
      .catch(error => {
        logger.error('Failed to setup close request listener', { error })
      })

    return () => {
      isMounted = false
      if (unlisten) {
        unlisten()
      }
    }
  }, [])
}
