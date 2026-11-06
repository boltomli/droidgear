import { beforeEach, describe, expect, it, vi } from 'vitest'
import userEvent from '@testing-library/user-event'

import { render, screen } from '@/test/test-utils'
import { useUIStore } from '@/store/ui-store'
import { LeftSideBar } from './LeftSideBar'

vi.stubGlobal(
  'ResizeObserver',
  class ResizeObserver {
    observe = vi.fn()
    unobserve = vi.fn()
    disconnect = vi.fn()
  }
)

describe('LeftSideBar module order', () => {
  beforeEach(() => {
    useUIStore.setState({ currentView: 'droid', lastToolView: 'droid' })
  })

  it('renders batch 1, batch 2 and the deprecated modules last', async () => {
    const user = userEvent.setup({ delay: null })
    render(<LeftSideBar />)

    await user.click(screen.getByRole('button', { name: /Droid/i }))

    const items = await screen.findAllByRole('menuitem')
    expect(items.map(item => item.textContent?.trim())).toEqual([
      'Codex',
      'Droid',
      'Hermes',
      'OMP',
      'Pi',
      'Claude',
      'Copilot',
      'Dsh',
      'OpenClawdeprecated',
      'OpenCodedeprecated',
    ])
  })

  it('marks the deprecated modules with a badge', async () => {
    const user = userEvent.setup({ delay: null })
    render(<LeftSideBar />)

    await user.click(screen.getByRole('button', { name: /Droid/i }))

    expect(await screen.findAllByText(/^(弃用|deprecated)$/)).toHaveLength(2)
  })
})
