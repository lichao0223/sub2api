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
  recalculations: vi.fn(),
  createRecalculation: vi.fn(),
  cancelRecalculation: vi.fn(),
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
const ConfirmDialogStub = defineComponent({
  props: ['show', 'title', 'message', 'loading'],
  emits: ['confirm', 'cancel'],
  template: '<div v-if="show" data-test="confirm-dialog"><h2>{{ title }}</h2><p>{{ message }}</p><button :disabled="loading" @click="$emit(\'confirm\')">确认创建</button><button @click="$emit(\'cancel\')">取消</button></div>',
})

const mountView = () => mount(CostManagementView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      Select: SelectStub,
      BaseDialog: BaseDialogStub,
      ConfirmDialog: ConfirmDialogStub,
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
    api.recalculations.mockReset().mockResolvedValue({ items: [], total: 0 })
    api.createRecalculation.mockReset().mockResolvedValue({})
    api.cancelRecalculation.mockReset().mockResolvedValue({})
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

  it('uses the app confirmation dialog and closes after queuing a recalculation', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '历史补算')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '开始补算')!.trigger('click')

    expect(wrapper.get('[data-test="confirm-dialog"]').text()).toContain('确认历史成本补算')
    await wrapper.get('[data-test="confirm-dialog"]').find('button').trigger('click')
    await flushPromises()

    expect(api.createRecalculation).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('[data-test="dialog"]').some(dialog => dialog.text().includes('核算任务'))).toBe(false)
  })

  it('shows recalculation failures with their stored error details', async () => {
    api.recalculations.mockResolvedValue({
      items: [{ id: 9, kind: 'recalculation', status: 'failed', start_date: '2026-07-01', end_date: '2026-07-07', total_days: 7, completed_days: 3, error_message: 'context deadline exceeded', created_at: '2026-07-29' }],
      total: 1,
    })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '核算任务')!.trigger('click')
    await flushPromises()

    const dialog = wrapper.findAll('[data-test="dialog"]').find(item => item.text().includes('核算任务'))!
    expect(dialog.text()).toContain('失败')
    expect(dialog.text()).toContain('context deadline exceeded')
    expect(dialog.text()).toContain('3/7 天（43%）')
  })

  it('cancels a queued recalculation after app confirmation', async () => {
    api.recalculations.mockResolvedValue({
      items: [{ id: 12, kind: 'recalculation', status: 'queued', start_date: '2026-01-01', end_date: '2026-07-29', total_days: 210, completed_days: 0, error_message: '', created_at: '2026-07-29T10:20:30Z' }],
      total: 1,
    })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '核算任务')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '取消')!.trigger('click')

    const confirm = wrapper.findAll('[data-test="confirm-dialog"]').find(dialog => dialog.text().includes('取消核算任务'))!
    await confirm.find('button').trigger('click')
    await flushPromises()

    expect(api.cancelRecalculation).toHaveBeenCalledWith(12)
  })
})
