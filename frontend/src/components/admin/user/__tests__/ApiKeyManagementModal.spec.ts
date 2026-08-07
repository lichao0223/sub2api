import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ApiKeyManagementModal from '../ApiKeyManagementModal.vue'

const { getAllIncludingInactive, getGroupApiKeys } = vi.hoisted(() => ({
  getAllIncludingInactive: vi.fn(),
  getGroupApiKeys: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: { getAllIncludingInactive, getGroupApiKeys },
    apiKeys: { batchUpdate: vi.fn() }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key
  })
}))

describe('ApiKeyManagementModal', () => {
  beforeEach(() => {
    getAllIncludingInactive.mockResolvedValue([{ id: 7, name: 'Large group' }])
    getGroupApiKeys.mockResolvedValue({ items: [], total: 900, page: 1, page_size: 20, pages: 45 })
  })

  it('offers whole-group selection before any page rows are selected', async () => {
    const wrapper = mount(ApiKeyManagementModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
          Select: { template: '<button data-test="choose-group" @click="$emit(\'update:modelValue\', 7)">choose</button>' },
          DataTable: { template: '<div />' },
          Pagination: true,
          Toggle: true,
          Icon: true
        }
      }
    })

    await wrapper.get('[data-test="choose-group"]').trigger('click')
    await flushPromises()

    const selectAll = wrapper.get('[data-test="select-all-in-group"]')
    expect(selectAll.text()).toContain('900')
    await selectAll.trigger('click')
    expect(wrapper.text()).toContain('admin.users.apiKeyManagement.selectedAll:{"count":900}')
  })
})
