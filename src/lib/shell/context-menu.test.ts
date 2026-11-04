import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { showContextMenu } from './context-menu'

function menuItems(): HTMLElement[] {
  return Array.from(
    document.querySelectorAll<HTMLElement>(
      '[data-context-menu] [role="menuitem"]'
    )
  )
}

function itemAt(index: number): HTMLElement {
  const item = menuItems()[index]
  if (!item) throw new Error(`no menu item at index ${index}`)
  return item
}

describe('context menu', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
    // Anchor the menu near the top-left so it never clamps off-screen.
    document.dispatchEvent(
      new MouseEvent('contextmenu', { clientX: 40, clientY: 60 })
    )
  })

  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('renders items, separators and accelerators at the pointer', async () => {
    await showContextMenu([
      { id: 'copy', label: 'Copy', accelerator: 'Ctrl+C' },
      { type: 'separator' },
      { id: 'delete', label: 'Delete', disabled: true },
    ])

    expect(menuItems()).toHaveLength(2)
    expect(itemAt(0).textContent).toContain('Copy')
    expect(itemAt(0).textContent).toContain('Ctrl+C')
    expect(itemAt(1).dataset.disabled).toBe('true')
    expect(
      document.querySelector('[data-context-menu] [role="separator"]')
    ).not.toBeNull()

    const menu = document.querySelector<HTMLElement>('[data-context-menu]')
    expect(menu?.style.left).toBe('40px')
    expect(menu?.style.top).toBe('60px')
  })

  it('runs the action and closes when an item is clicked', async () => {
    let called = 0
    await showContextMenu([
      { id: 'open', label: 'Open', action: () => (called += 1) },
    ])

    itemAt(0).click()

    expect(called).toBe(1)
    expect(document.querySelector('[data-context-menu]')).toBeNull()
  })

  it('ignores clicks on disabled items but keeps the menu open', async () => {
    let called = 0
    await showContextMenu([
      {
        id: 'gone',
        label: 'Gone',
        disabled: true,
        action: () => (called += 1),
      },
    ])

    itemAt(0).click()

    expect(called).toBe(0)
    expect(document.querySelector('[data-context-menu]')).not.toBeNull()
  })

  it('closes on Escape and on an outside pointerdown', async () => {
    await showContextMenu([{ id: 'a', label: 'A' }])
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(document.querySelector('[data-context-menu]')).toBeNull()

    await showContextMenu([{ id: 'a', label: 'A' }])
    document.body.dispatchEvent(
      new PointerEvent('pointerdown', { bubbles: true })
    )
    expect(document.querySelector('[data-context-menu]')).toBeNull()
  })

  it('replaces a menu that is already open', async () => {
    await showContextMenu([{ id: 'first', label: 'First' }])
    await showContextMenu([{ id: 'second', label: 'Second' }])

    expect(document.querySelectorAll('[data-context-menu]')).toHaveLength(1)
    expect(itemAt(0).textContent).toContain('Second')
  })

  it('does nothing for an empty entry list', async () => {
    await showContextMenu([])
    expect(document.querySelector('[data-context-menu]')).toBeNull()
  })
})
