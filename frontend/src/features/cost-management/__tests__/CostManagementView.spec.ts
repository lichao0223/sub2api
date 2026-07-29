import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import CostManagementView from '../CostManagementView.vue'

const api = vi.hoisted(() => ({
  overview: vi.fn(),
  analysis: vi.fn(),
  accounts: vi.fn(),
  plans: vi.fn(),
  modelOptions: vi.fn(),
  createPlan: vi.fn(),
  updatePlan: vi.fn(),
}))

vi.mock('@/api/admin/costManagement', () => ({ default: api }))
vi.mock('@/stores', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }),
}))
vi.mock('chart.js/auto', () => ({
  Chart: class {
    destroy() {}
  },
}))

const SelectStub = defineComponent({
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  template: `<select :value="modelValue" @change="$emit('update:modelValue', $event.target.value);$emit('change', $event.target.value)">
    <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
  </select>`,
})
const BaseDialogStub = defineComponent({
  props: ['show', 'title'],
  emits: ['close'],
  template: '<div v-if="show" data-test="dialog"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>',
})

const mountView = () => mount(CostManagementView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      Select: SelectStub,
      BaseDialog: BaseDialogStub,
      DateRangePicker: true,
      Pagination: true,
    },
  },
})

describe('CostManagementView', () => {
  beforeEach(() => {
    api.overview.mockReset().mockResolvedValue({
      dynamic_cost_cny: '0', fixed_cost_cny: '0', total_cost_cny: '0',
      pending_count: 0, error_count: 0, eligible_count: 0, calculated_count: 0,
      coverage_complete: true, previous_total_cost_cny: '0',
    })
    api.analysis.mockReset().mockResolvedValue({ period: 'day', trend: [], top: [] })
    api.accounts.mockReset().mockResolvedValue({
      items: [{ account_id: 7, account_name: 'GLM Anthropic', platform: 'anthropic', account_status: 'active', cost_mode: '', plan_name: '', pending_count: 0, exclude_reason: '' }],
      total: 1,
    })
    api.plans.mockReset().mockResolvedValue({
      items: [{ id: 11, name: 'GLM 按量', plan_type: 'metered', status: 'active' }],
      total: 1,
    })
    api.modelOptions.mockReset().mockResolvedValue({ items: [], total: 0 })
    api.createPlan.mockReset().mockResolvedValue({})
    api.updatePlan.mockReset().mockResolvedValue({})
  })

  it('derives the cost mode from the selected plan', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '账号成本')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '配置')!.trigger('click')

    const dialog = wrapper.get('[data-test="dialog"]')
    expect(dialog.text()).toContain('成本方案')
    expect(dialog.text()).not.toContain('成本方式')
    expect(dialog.text()).not.toContain('排除原因')
    await dialog.get('select').setValue('excluded')
    expect(dialog.text()).toContain('排除原因')
  })

  it('shows only fields used by the selected billing mode', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '成本方案')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '新建成本方案')!.trigger('click')

    const dialog = wrapper.get('[data-test="dialog"]')
    expect(dialog.text()).toContain('输入 Token')
    expect(dialog.text()).not.toContain('每次请求')
    await dialog.findAll('select').at(-1)!.setValue('request')
    expect(dialog.text()).not.toContain('输入 Token')
    expect(dialog.text()).toContain('每次请求')
  })

  it('serializes numeric fixed costs as strings', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '成本方案')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '新建成本方案')!.trigger('click')

    const dialog = wrapper.get('[data-test="dialog"]')
    await dialog.findAll('select').at(0)!.setValue('fixed')
    await dialog.findAll('select').at(-1)!.setValue('yearly')
    const inputs = dialog.findAll('input')
    await inputs.at(0)!.setValue('GLM MAX 套餐')
    await inputs.at(3)!.setValue(4500)
    await dialog.findAll('button').find(button => button.text() === '保存')!.trigger('click')
    await flushPromises()

    expect(api.createPlan).toHaveBeenCalledWith(expect.objectContaining({
      fixed_unit_cost_cny: '4500',
      monthly_unit_cost_cny: '0',
    }))
  })
})
