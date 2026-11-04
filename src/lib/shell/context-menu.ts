/**
 * Right-click context menu.
 *
 * The previous implementation used the OS-level native menu. The Wails
 * runtime exposes no popup-menu API to JavaScript, so this renders a small
 * floating menu at the pointer position instead: same public surface
 * (`showContextMenu(items)`), same entries, same `action()` callbacks.
 *
 * The anchor comes from the most recent `contextmenu` event, captured once
 * at module load — callers never pass coordinates, exactly like before.
 *
 * @example
 * ```typescript
 * await showContextMenu([
 *   { id: 'copy', label: 'Copy', accelerator: 'Ctrl+C', action: handleCopy },
 *   { type: 'separator' },
 *   { id: 'delete', label: 'Delete', action: handleDelete },
 * ])
 * ```
 */

export interface ContextMenuItem {
  id: string
  label: string
  accelerator?: string
  disabled?: boolean
  action?: () => void
}

export interface ContextMenuSeparator {
  type: 'separator'
}

export type ContextMenuEntry = ContextMenuItem | ContextMenuSeparator

function isSeparator(item: ContextMenuEntry): item is ContextMenuSeparator {
  return 'type' in item
}

/** Last pointer position from a `contextmenu` event (screen-space anchor). */
let anchor = { x: 0, y: 0 }

document.addEventListener(
  'contextmenu',
  event => {
    anchor = { x: event.clientX, y: event.clientY }
  },
  { capture: true, passive: true }
)

let openMenu: HTMLElement | null = null
let removeListeners: (() => void) | null = null

function close(): void {
  removeListeners?.()
  removeListeners = null
  openMenu?.remove()
  openMenu = null
}

/** Arrow-key navigation across the enabled items of the open menu. */
function moveFocus(step: 1 | -1): void {
  if (!openMenu) return
  const entries = Array.from(
    openMenu.querySelectorAll<HTMLElement>('[role="menuitem"]')
  ).filter(el => el.dataset.disabled !== 'true')
  if (entries.length === 0) return
  const index = entries.findIndex(el => el === document.activeElement)
  const next =
    index < 0
      ? step === 1
        ? 0
        : entries.length - 1
      : (index + step + entries.length) % entries.length
  entries[next]?.focus()
}

function buildMenu(items: ContextMenuEntry[]): HTMLElement {
  const menu = document.createElement('div')
  menu.setAttribute('role', 'menu')
  menu.dataset.contextMenu = 'true'
  menu.style.cssText = [
    'position:fixed',
    'z-index:1000',
    'min-width:200px',
    'max-width:320px',
    'padding:4px',
    'margin:0',
    'list-style:none',
    'background:var(--popover, #fff)',
    'color:var(--popover-foreground, #111)',
    'border:1px solid var(--border, rgba(0,0,0,.12))',
    'border-radius:8px',
    'box-shadow:0 8px 24px rgba(0,0,0,.16)',
    'font-size:13px',
    'line-height:1.5',
  ].join(';')

  for (const item of items) {
    if (isSeparator(item)) {
      const separator = document.createElement('div')
      separator.setAttribute('role', 'separator')
      separator.style.cssText =
        'height:1px;margin:4px 6px;background:var(--border, rgba(0,0,0,.12))'
      menu.appendChild(separator)
      continue
    }

    const entry = document.createElement('div')
    entry.setAttribute('role', 'menuitem')
    entry.tabIndex = -1
    entry.dataset.id = item.id
    if (item.disabled) entry.dataset.disabled = 'true'

    const label = document.createElement('span')
    label.textContent = item.label
    entry.appendChild(label)

    if (item.accelerator) {
      const shortcut = document.createElement('span')
      shortcut.textContent = item.accelerator
      shortcut.style.cssText = 'margin-left:auto;padding-left:16px;opacity:.6'
      entry.appendChild(shortcut)
    }

    entry.style.cssText = [
      'display:flex',
      'align-items:center',
      'gap:8px',
      'padding:6px 10px',
      'border-radius:6px',
      'cursor:' + (item.disabled ? 'default' : 'pointer'),
      'opacity:' + (item.disabled ? '.5' : '1'),
      'outline:none',
    ].join(';')
    entry.addEventListener('mouseenter', () => {
      if (item.disabled) return
      entry.style.background = 'var(--accent, rgba(0,0,0,.08))'
    })
    entry.addEventListener('mouseleave', () => {
      entry.style.background = 'transparent'
    })
    entry.addEventListener('click', () => {
      if (item.disabled) return
      close()
      item.action?.()
    })

    menu.appendChild(entry)
  }

  return menu
}

/**
 * Show a context menu at the last right-click position.
 *
 * Resolves when the menu is dismissed (it is not awaited by callers today).
 */
export async function showContextMenu(
  items: ContextMenuEntry[]
): Promise<void> {
  close()
  if (items.length === 0) return

  const menu = buildMenu(items)
  document.body.appendChild(menu)

  // Keep the menu inside the viewport.
  const rect = menu.getBoundingClientRect()
  const x = Math.min(anchor.x, window.innerWidth - rect.width - 8)
  const y = Math.min(anchor.y, window.innerHeight - rect.height - 8)
  menu.style.left = `${Math.max(8, x)}px`
  menu.style.top = `${Math.max(8, y)}px`

  openMenu = menu
  menu.querySelector<HTMLElement>('[role="menuitem"]')?.focus()

  const onPointerDown = (event: Event) => {
    if (event.target instanceof Node && menu.contains(event.target)) return
    close()
  }
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      close()
      return
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      moveFocus(1)
      return
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      moveFocus(-1)
      return
    }
    if (event.key === 'Enter' || event.key === ' ') {
      const target = document.activeElement
      if (target instanceof HTMLElement && menu.contains(target)) {
        event.preventDefault()
        target.click()
      }
    }
  }
  const onDismiss = () => close()

  document.addEventListener('pointerdown', onPointerDown, true)
  document.addEventListener('keydown', onKeyDown, true)
  window.addEventListener('blur', onDismiss)
  window.addEventListener('resize', onDismiss)
  document.addEventListener('scroll', onDismiss, true)

  removeListeners = () => {
    document.removeEventListener('pointerdown', onPointerDown, true)
    document.removeEventListener('keydown', onKeyDown, true)
    window.removeEventListener('blur', onDismiss)
    window.removeEventListener('resize', onDismiss)
    document.removeEventListener('scroll', onDismiss, true)
  }
}

/** Test hook: the menu currently on screen, if any. */
export function getOpenContextMenu(): HTMLElement | null {
  return openMenu
}
