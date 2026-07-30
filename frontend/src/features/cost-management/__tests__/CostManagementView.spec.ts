import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import CostManagementView from '../CostManagementView.vue'

const api = vi.hoisted(() => ({
  overview: vi.fn(),
  analysis: vi.fn(),
  accounts: vi.fn(),
  plans: vi.fn(),
  plan: vi.fn(),
  modelOptions: vi.fn(),
  subscriptionUnits: vi.fn(),
  createSubscriptionUnit: vi.fn(),
  renameSubscriptionUnit: vi.fn(),
  endSubscriptionUnit: vi.fn(),
  saveAccount: vi.fn(),
  saveAccounts: vi.fn(),
  endAccount: vi.fn(),
  createPlan: vi.fn(),
  updatePlan: vi.fn(),
  changePlanPrice: vi.fn(),
  priceHistory: vi.fn(),
  disablePlan: vi.fn(),
  recalculations: vi.fn(),
  createRecalculation: vi.fn(),
  cancelRecalculation: vi.fn(),
}))
const app = vi.hoisted(() => ({ showSuccess: vi.fn(), showError: vi.fn() }))

vi.mock('@/api/admin/costManagement', () => ({ default: api }))
vi.mock('@/stores', () => ({
  useAppStore: () => app,
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
    api.plan.mockReset().mockResolvedValue({
      id: 11, name: 'GLM 按量', plan_type: 'metered', status: 'active',
      effective_from: '2026-01-01T02:12:00Z', effective_to: '2026-12-31T15:59:59Z',
      billing_cycle: 'monthly', fixed_unit_cost_cny: '0', monthly_unit_cost_cny: '0',
      model_count: 1, account_count: 0, note: '',
      prices: [{ upstream_model: 'glm', billing_mode: 'token', input_price_cny: '1', output_price_cny: '2', cache_write_price_cny: '0', cache_read_price_cny: '0', image_input_price_cny: '0', image_output_price_cny: '0', per_request_price_cny: '0' }],
    })
    api.modelOptions.mockReset().mockResolvedValue({ items: [], total: 0 })
    api.subscriptionUnits.mockReset().mockResolvedValue([])
    api.createSubscriptionUnit.mockReset().mockResolvedValue({})
    api.renameSubscriptionUnit.mockReset().mockResolvedValue({})
    api.endSubscriptionUnit.mockReset().mockResolvedValue({})
    api.saveAccount.mockReset().mockResolvedValue({})
    api.saveAccounts.mockReset().mockResolvedValue({})
    api.endAccount.mockReset().mockResolvedValue({})
    api.createPlan.mockReset().mockResolvedValue({})
    api.updatePlan.mockReset().mockResolvedValue({})
    api.changePlanPrice.mockReset().mockResolvedValue({})
    api.priceHistory.mockReset().mockResolvedValue([])
    api.disablePlan.mockReset().mockResolvedValue({})
    api.recalculations.mockReset().mockResolvedValue({ items: [], total: 0 })
    api.createRecalculation.mockReset().mockResolvedValue({})
    api.cancelRecalculation.mockReset().mockResolvedValue({})
    app.showSuccess.mockReset()
    app.showError.mockReset()
  })

  it('derives the cost mode from the selected plan', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '账号成本')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '配置')!.trigger('click')

    const dialog = wrapper.get('[data-test="dialog"]')
    expect(api.accounts).toHaveBeenCalledWith(expect.objectContaining({ start_date: expect.any(String), end_date: expect.any(String) }))
    expect(dialog.text()).toContain('成本方案')
    expect(dialog.text()).not.toContain('成本方式')
    expect(dialog.text()).not.toContain('排除原因')
    await dialog.get('select').setValue('excluded')
    expect(dialog.text()).toContain('排除原因')
  })

  it('does not render the redundant metered accounting panel', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).not.toContain('按量成本核算')
  })

  it('rebuilds the cost chart when returning to the overview tab', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '账号成本')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '成本总览')!.trigger('click')
    await flushPromises()

    expect(api.analysis).toHaveBeenCalledTimes(2)
  })

  it('rejects an account cost configuration without a plan before submitting', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '账号成本')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '配置')!.trigger('click')

    await wrapper.get('[data-test="dialog"]').findAll('button').find(button => button.text() === '保存')!.trigger('click')

    expect(api.saveAccount).not.toHaveBeenCalled()
    expect(app.showError).toHaveBeenCalledOnce()
    expect(app.showError).toHaveBeenCalledWith('请选择成本方案')
  })

  it('groups fixed-cost accounts into an existing or new subscription instance', async () => {
    api.plans.mockResolvedValue({
      items: [{ id: 12, name: 'ChatGPT Plus', plan_type: 'fixed', status: 'active' }],
      total: 1,
    })
    api.subscriptionUnits.mockResolvedValue([
      { id: 31, plan_id: 12, name: '订阅 #1', effective_from: '2026-01-01', billing_cycle: 'monthly', fixed_unit_cost_cny: '140', monthly_unit_cost_cny: '140', version_no: 1, account_count: 2 },
    ])
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '账号成本')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '配置')!.trigger('click')
    const dialog = wrapper.get('[data-test="dialog"]')

    await dialog.get('select').setValue('12')
    await flushPromises()
    expect(dialog.text()).toContain('归属订阅实例')
    expect(dialog.text()).toContain('订阅 #1（2 个账号）')
    await dialog.findAll('select').at(1)!.setValue('new')
    expect(dialog.text()).toContain('新订阅实例名称')
    await dialog.get('input[placeholder="例如：ChatGPT Plus #3"]').setValue('订阅 #2')
    await dialog.findAll('button').find(button => button.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(api.saveAccount).toHaveBeenCalledWith(7, expect.objectContaining({
      plan_id: 12,
      subscription_unit_id: undefined,
      new_subscription_unit_name: '订阅 #2',
    }))
  })

  it('rejects an account assignment before the selected subscription starts', async () => {
    api.plans.mockResolvedValue({
      items: [{ id: 12, name: 'ChatGPT Plus', plan_type: 'fixed', status: 'active' }],
      total: 1,
    })
    api.subscriptionUnits.mockResolvedValue([
      { id: 31, plan_id: 12, name: '订阅 #1', effective_from: '2026-07-01T00:00:00Z', billing_cycle: 'monthly', fixed_unit_cost_cny: '140', monthly_unit_cost_cny: '140', version_no: 1, account_count: 0 },
    ])
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '账号成本')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '配置')!.trigger('click')
    const dialog = wrapper.get('[data-test="dialog"]')
    await dialog.get('select').setValue('12')
    await flushPromises()
    await dialog.findAll('select').at(1)!.setValue('31')
    await dialog.get('input[type="datetime-local"]').setValue('2026-02-03T14:42')
    await dialog.findAll('button').find(button => button.text() === '保存')!.trigger('click')

    expect(api.saveAccount).not.toHaveBeenCalled()
    expect(app.showError).toHaveBeenCalledWith(expect.stringContaining('不能早于订阅实例开始时间'))
  })

  it('creates, renames and ends a fixed-plan subscription instance', async () => {
    api.plans.mockResolvedValue({
      items: [{ id: 12, name: 'ChatGPT Plus', plan_type: 'fixed', status: 'active' }],
      total: 1,
    })
    api.subscriptionUnits.mockResolvedValue([
      { id: 31, plan_id: 12, name: '订阅 #1', effective_from: '2026-07-29T01:02:03Z', billing_cycle: 'monthly', fixed_unit_cost_cny: '140', monthly_unit_cost_cny: '140', version_no: 1, account_count: 2 },
    ])
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '成本方案')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '订阅实例')!.trigger('click')
    await flushPromises()

    let dialog = wrapper.findAll('[data-test="dialog"]').find(item => item.text().includes('订阅实例 · ChatGPT Plus'))!
    await dialog.get('input[placeholder="例如：ChatGPT Plus #3"]').setValue('订阅 #2')
    await dialog.findAll('button').find(button => button.text() === '新建实例')!.trigger('click')
    await flushPromises()
    expect(api.createSubscriptionUnit).toHaveBeenCalledWith(expect.objectContaining({ plan_id: 12, name: '订阅 #2', billing_cycle: 'monthly' }))

    dialog = wrapper.findAll('[data-test="dialog"]').find(item => item.text().includes('订阅实例 · ChatGPT Plus'))!
    await dialog.findAll('button').find(button => button.text() === '改名')!.trigger('click')
    await dialog.get('input').setValue('主订阅')
    await dialog.findAll('button').find(button => button.text() === '保存名称')!.trigger('click')
    await flushPromises()
    expect(api.renameSubscriptionUnit).toHaveBeenCalledWith(31, '主订阅')

    await dialog.findAll('button').find(button => button.text() === '结束')!.trigger('click')
    const endDialog = wrapper.findAll('[data-test="dialog"]').find(item => item.text().includes('结束订阅实例'))!
    await endDialog.findAll('button').find(button => button.text() === '确认结束')!.trigger('click')
    await flushPromises()
    expect(api.endSubscriptionUnit).toHaveBeenCalledWith(31, expect.any(String))
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

  it('keeps basic editing separate from creating a price version', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '成本方案')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '编辑')!.trigger('click')
    await flushPromises()

    const dialog = wrapper.get('[data-test="dialog"]')
    expect(dialog.find('input[type="datetime-local"]').exists()).toBe(false)
    expect(dialog.text()).not.toContain('模型价格')
    await dialog.findAll('button').find(button => button.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(api.updatePlan).toHaveBeenCalledWith(11, expect.objectContaining({ name: 'GLM 按量' }))

    await wrapper.findAll('button').find(button => button.text() === '价格变更')!.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="dialog"]').text()).toContain('历史价格不会被覆盖')
  })

  it('serializes numeric fixed costs as strings', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '成本方案')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '新建成本方案')!.trigger('click')

    const dialog = wrapper.get('[data-test="dialog"]')
    await dialog.findAll('select').at(0)!.setValue('fixed')
    await dialog.findAll('select').at(1)!.setValue('yearly')
    await dialog.find('input').setValue('GLM MAX 套餐')
    await dialog.get('input[type="number"]').setValue(4500)
    await dialog.findAll('button').find(button => button.text() === '保存')!.trigger('click')
    await flushPromises()

    expect(api.createPlan).toHaveBeenCalledWith(expect.objectContaining({
      fixed_unit_cost_cny: '4500',
    }))
  })

  it('uses the app confirmation dialog and closes after queuing a recalculation', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '核算任务')!.trigger('click')
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
