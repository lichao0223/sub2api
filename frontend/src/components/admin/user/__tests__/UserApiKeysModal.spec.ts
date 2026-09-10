import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import UserApiKeysModal from '../UserApiKeysModal.vue'

const { getUserApiKeys, getAll, showError } = vi.hoisted(() => ({
  getUserApiKeys: vi.fn(),
  getAll: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { getUserApiKeys },
    groups: { getAll },
    apiKeys: {}
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() })
}))

vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => key })
}))

const user = {
  id: 7,
  email: 'user@example.test',
  username: 'User'
}

function mountModal() {
  return mount(UserApiKeysModal, {
    props: { show: false, user: user as never },
    global: {
      stubs: {
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot /><slot name="footer" /></div>'
        },
        GroupBadge: true,
        GroupOptionItem: true,
        Select: true,
        Icon: true
      }
    }
  })
}

describe('UserApiKeysModal concurrency refresh', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    getUserApiKeys.mockReset()
    getAll.mockReset()
    showError.mockReset()
    getUserApiKeys.mockResolvedValue({ items: [], total: 0 })
    getAll.mockResolvedValue([])
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('refreshes while open and stops polling after close', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(getUserApiKeys).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(getUserApiKeys).toHaveBeenCalledTimes(2)

    await wrapper.setProps({ show: false })
    await vi.advanceTimersByTimeAsync(4000)
    expect(getUserApiKeys).toHaveBeenCalledTimes(2)

    wrapper.unmount()
  })

  it('does not show repeated errors when a background refresh fails', async () => {
    getUserApiKeys
      .mockResolvedValueOnce({ items: [], total: 0 })
      .mockRejectedValueOnce(new Error('temporary failure'))
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()

    expect(showError).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
