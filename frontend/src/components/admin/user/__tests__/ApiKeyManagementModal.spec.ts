import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ApiKeyManagementModal from '../ApiKeyManagementModal.vue'

const { getAllIncludingInactive, getGroupApiKeys, batchUpdate } = vi.hoisted(() => ({
  getAllIncludingInactive: vi.fn(),
  getGroupApiKeys: vi.fn(),
  batchUpdate: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: { getAllIncludingInactive, getGroupApiKeys },
    apiKeys: { batchUpdate }
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
    getAllIncludingInactive.mockResolvedValue([
      { id: 7, name: 'Large group', status: 'active', is_exclusive: false, subscription_type: 'standard' },
      { id: 8, name: 'Target group', status: 'active', is_exclusive: false, subscription_type: 'standard' }
    ])
    getGroupApiKeys.mockResolvedValue({ items: [], total: 900, page: 1, page_size: 20, pages: 45 })
    batchUpdate.mockReset()
    batchUpdate.mockResolvedValue({ affected: 900, created: 900 })
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

  it('submits the target group for the whole-group selection', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mount(ApiKeyManagementModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
          Select: {
            props: ['modelValue'],
            template: '<button :data-test="$attrs[\'data-test\'] || \'choose-group\'" @click="$emit(\'update:modelValue\', $attrs[\'data-test\'] ? 8 : 7)">choose</button>'
          },
          DataTable: { template: '<div />' },
          Pagination: true,
          Toggle: { template: '<button @click="$emit(\'update:modelValue\', true)" />' },
          Icon: true
        }
      }
    })

    await wrapper.get('[data-test="choose-group"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="select-all-in-group"]').trigger('click')
    await wrapper.get('[data-test="edit-group"]').trigger('click')
    await wrapper.get('[data-test="target-group"]').trigger('click')
    await wrapper.get('[data-test="recreate-in-source"]').setValue(true)
    await wrapper.get('button.btn-primary:last-child').trigger('click')
    await flushPromises()

    expect(batchUpdate).toHaveBeenCalledWith({
      group_id: 7,
      all: true,
      target_group_id: 8,
      recreate_in_source_group: true
    })
  })
})
