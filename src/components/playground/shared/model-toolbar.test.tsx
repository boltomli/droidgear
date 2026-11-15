import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@/test/test-utils'
import { ModelToolbar } from './model-toolbar'

const models = [
  { id: 'vm_1', name: 'Seedance', modelId: 'doubao-seedance-2-0-mini' },
  { id: 'vm_2', name: 'Kling', modelId: 'kling-v3' },
]

function renderToolbar(
  overrides: {
    onSelect?: (id: string) => void
    onAdd?: () => void
    onManage?: () => void
  } = {}
) {
  const onSelect = overrides.onSelect ?? vi.fn()
  const onAdd = overrides.onAdd ?? vi.fn()
  const onManage = overrides.onManage ?? vi.fn()
  render(
    <ModelToolbar
      models={models}
      selectedId=""
      loading={false}
      onSelect={onSelect}
      onAdd={onAdd}
      onManage={onManage}
      ariaLabel="Model"
      placeholder="Select a model"
      addLabel="Add video model"
      manageLabel="Manage"
    />
  )
  return { onSelect, onAdd, onManage }
}

describe('ModelToolbar', () => {
  it('selects a model from the dropdown', async () => {
    const { onSelect, onAdd } = renderToolbar()
    const user = userEvent.setup({ delay: null })

    await user.click(screen.getByRole('combobox', { name: 'Model' }))
    await user.click(await screen.findByRole('option', { name: /Kling/ }))

    expect(onSelect).toHaveBeenCalledWith('vm_2')
    expect(onAdd).not.toHaveBeenCalled()
  })

  it('adds a model from the dropdown', async () => {
    const { onSelect, onAdd } = renderToolbar()
    const user = userEvent.setup({ delay: null })

    await user.click(screen.getByRole('combobox', { name: 'Model' }))
    await user.click(
      await screen.findByRole('option', { name: /Add video model/ })
    )

    expect(onAdd).toHaveBeenCalledOnce()
    expect(onSelect).not.toHaveBeenCalled()
  })
})
