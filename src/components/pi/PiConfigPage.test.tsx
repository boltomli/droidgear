import { beforeEach, describe, expect, it, vi } from 'vitest'
import userEvent from '@testing-library/user-event'

const { commandMocks } = vi.hoisted(() => ({
  commandMocks: {
    listPiProfiles: vi.fn(),
    createDefaultPiProfile: vi.fn(),
    getActivePiProfileId: vi.fn(),
    listPiSessions: vi.fn(),
    getPiConfigStatus: vi.fn(),
    readPiSettings: vi.fn(),
    readPiCurrentConfig: vi.fn(),
    savePiSettings: vi.fn(),
    savePiProfile: vi.fn(),
  },
}))

vi.mock('@/lib/bindings', () => ({
  commands: commandMocks,
}))

import { render, screen, waitFor } from '@/test/test-utils'
import { PiConfigPage } from './PiConfigPage'

vi.stubGlobal(
  'ResizeObserver',
  class ResizeObserver {
    observe = vi.fn()
    unobserve = vi.fn()
    disconnect = vi.fn()
  }
)

const profile = {
  id: 'p1',
  name: 'Default',
  description: null,
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z',
  providers: {},
}

describe('PiConfigPage default model', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    commandMocks.listPiProfiles.mockResolvedValue({
      status: 'ok',
      data: [profile],
    })
    commandMocks.createDefaultPiProfile.mockResolvedValue({
      status: 'ok',
      data: profile,
    })
    commandMocks.getActivePiProfileId.mockResolvedValue({
      status: 'ok',
      data: 'p1',
    })
    commandMocks.getPiConfigStatus.mockResolvedValue({
      status: 'ok',
      data: {
        configExists: true,
        configPath: '/home/test/.pi/agent/models.json',
      },
    })
    commandMocks.readPiSettings.mockResolvedValue({
      status: 'ok',
      data: {
        defaultProvider: null,
        defaultModel: null,
        defaultThinkingLevel: null,
        settingsExists: false,
        settingsPath: '/home/test/.pi/agent/settings.json',
      },
    })
    commandMocks.readPiCurrentConfig.mockResolvedValue({
      status: 'ok',
      data: { providers: {} },
    })
    commandMocks.savePiSettings.mockResolvedValue({ status: 'ok', data: null })
    commandMocks.savePiProfile.mockResolvedValue({ status: 'ok', data: null })
  })

  it('writes the provider/model pair into settings.json', async () => {
    const user = userEvent.setup({ delay: null })
    render(<PiConfigPage />)

    const providerInput = await screen.findByPlaceholderText(/wududu/)
    const modelInput = screen.getByPlaceholderText(/deepseek-v4\.1-flash/)
    await user.type(providerInput, 'wududu')
    await user.type(modelInput, 'deepseek-v4.1-flash')

    await user.click(
      screen.getByRole('button', { name: /默认模型|default model/i })
    )

    await waitFor(() => {
      expect(commandMocks.savePiSettings).toHaveBeenCalledWith({
        defaultProvider: 'wududu',
        defaultModel: 'deepseek-v4.1-flash',
        defaultThinkingLevel: null,
      })
    })
  })

  it('clears the keys when both fields are emptied (pi picks automatically)', async () => {
    commandMocks.readPiSettings.mockResolvedValue({
      status: 'ok',
      data: {
        defaultProvider: 'wududu',
        defaultModel: 'deepseek-v4.1-flash',
        defaultThinkingLevel: 'high',
        settingsExists: true,
        settingsPath: '/home/test/.pi/agent/settings.json',
      },
    })
    const user = userEvent.setup({ delay: null })
    render(<PiConfigPage />)

    const providerInput = await screen.findByPlaceholderText(/wududu/)
    await waitFor(() => expect(providerInput).toHaveValue('wududu'))
    await user.clear(providerInput)
    await user.clear(screen.getByPlaceholderText(/deepseek-v4\.1-flash/))

    await user.click(
      screen.getByRole('button', { name: /默认模型|default model/i })
    )

    await waitFor(() => {
      expect(commandMocks.savePiSettings).toHaveBeenCalledWith({
        defaultProvider: null,
        defaultModel: null,
        defaultThinkingLevel: 'high',
      })
    })
  })
})
