import { Window } from '@wailsio/runtime'

const LIGHT = [255, 255, 255] as const
const DARK = [15, 15, 15] as const

/**
 * Keep the native window background colour in sync with the active theme.
 *
 * The Go side picks the initial colour from the persisted preference
 * (`startupBackgroundColour` in wails/main.go); this observer covers every
 * subsequent change (theme toggle, system-scheme flip) so the window edges
 * and the area outside the webview never show the wrong colour — the
 * "window looks misaligned" artefact under a light theme (round2 §3.5).
 *
 * Deliberately Wails-only: the Tauri shell decides its background itself,
 * so this must not move into the shared ThemeProvider.
 *
 * @returns disconnect callback for the MutationObserver.
 * @see docs/developer/wails-round2-layout-and-gaps.zh.md §B6
 */
export function syncWindowBackground(): () => void {
  const root = document.documentElement
  const apply = () => {
    const [r, g, b] = root.classList.contains('dark') ? DARK : LIGHT
    void Window.SetBackgroundColour(r, g, b, 255)
  }
  apply()
  const observer = new MutationObserver(apply)
  observer.observe(root, { attributes: true, attributeFilter: ['class'] })
  return () => observer.disconnect()
}
