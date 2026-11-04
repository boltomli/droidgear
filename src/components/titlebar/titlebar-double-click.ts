import type { MouseEvent } from 'react'
import { getCurrentWindow } from '@/lib/shell/window'

/**
 * Interactive elements inside the drag region that must not trigger a
 * maximise toggle on double-click. Mirrors the `--wails-draggable: none`
 * opt-out list in `src/shell.css` — keep the two in sync.
 */
const INTERACTIVE_SELECTOR =
  'button, a, input, select, textarea, [role="button"], [role="menuitem"], ' +
  '[role="combobox"], [tabindex]:not([tabindex="-1"]), ' +
  '[data-slot="dropdown-menu-trigger"], [data-dropdown-menu-trigger], ' +
  '[data-no-drag]'

/**
 * Double-click on the title bar toggles maximise.
 *
 * - The runtime only forwards `dblclick` to the OS on
 *   macOS (`drag.ts` `suppressEvent` is `IsMac()`-guarded and stops the event
 *   before it reaches React), so Windows/Linux shells need this handler.
 *
 * @see docs/developer/wails-round2-layout-and-gaps.zh.md §B4
 */
export function handleTitleBarDoubleClick(event: MouseEvent<HTMLDivElement>) {
  const target = event.target as HTMLElement
  if (target.closest(INTERACTIVE_SELECTOR)) return
  void getCurrentWindow().toggleMaximize()
}
