import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { formatDateLocalInput } from '@/utils/format'
import CostManagementView from '../CostManagementView.vue'

const api = vi.hoisted(() => ({
  overview: vi.fn(),
  breakdown: vi.fn(),
  pendingDetails: vi.fn(),
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
const chartConfigs = vi.hoisted(() => [] as any[])
const xlsx = vi.hoisted(() => ({
  aoaToSheet: vi.fn(() => ({})),
  bookNew: vi.fn(() => ({})),
  bookAppendSheet: vi.fn(),
  writeFile: vi.fn(),
}))

vi.mock('@/api/admin/costManagement', () => ({ default: api }))
vi.mock('@/stores', () => ({
  useAppStore: () => app,
}))
vi.mock('chart.js/auto', () => ({
  Chart: class {
    constructor(_canvas: unknown, config: any) {
      chartConfigs.push(config)
    }
    destroy() {}
  },
}))
vi.mock('xlsx', () => ({
  utils: {
    aoa_to_sheet: xlsx.aoaToSheet,
    book_new: xlsx.bookNew,
    book_append_sheet: xlsx.bookAppendSheet,
  },
  writeFile: xlsx.writeFile,
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
const TimePricingSectionStub = defineComponent({
  props: ['modelValue'],
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () => h('div', props.modelValue.periods.map((period: any, index: number) => h('input', {
      inputmode: 'numeric',
      value: period.start_time,
      onInput: (event: Event) => emit('update:modelValue', {
        ...props.modelValue,
        periods: props.modelValue.periods.map((item: any, current: number) => current === index
          ? { ...item, start_time: (event.target as HTMLInputElement).value }
          : item),
      }),
    })))
  },
})

const mountView = () => mount(CostManagementView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      Select: SelectStub,
      BaseDialog: BaseDialogStub,
      ConfirmDialog: ConfirmDialogStub,
      TimePricingSection: TimePricingSectionStub,
      DateRangePicker: true,
      Pagination: true,
    },
  },
})

describe('CostManagementView', () => {
  beforeEach(() => {
    chartConfigs.length = 0
    Object.values(xlsx).forEach(mock => mock.mockClear())
    api.overview.mockReset().mockResolvedValue({
      dynamic_cost_cny: '0', fixed_cost_cny: '0', total_cost_cny: '0',
      pending_count: 0, error_count: 0, eligible_count: 0, calculated_count: 0,
      coverage_complete: true, previous_total_cost_cny: '0',
    })
    api.analysis.mockReset().mockResolvedValue({ period: 'day', trend: [], top: [] })
    api.breakdown.mockReset().mockResolvedValue({ total_cost_cny: '375', items: [] })
    api.pendingDetails.mockReset().mockResolvedValue({ total_count: 0, items: [] })
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

  it('shows cost composition for the selected overview range', async () => {
    api.overview.mockResolvedValue({
      dynamic_cost_cny: '0', fixed_cost_cny: '375', total_cost_cny: '375',
      pending_count: 0, error_count: 0, eligible_count: 0, calculated_count: 0,
      coverage_complete: true, previous_total_cost_cny: '0',
    })
    api.breakdown.mockResolvedValue({
      total_cost_cny: '375',
      items: [{
        cost_mode: 'fixed', plan_name: 'Kimi 套餐 199', account_name: '', subscription_unit_name: 'Kimi 199', upstream_model: '',
        billing_mode: '', input_price_cny: '0', output_price_cny: '0', cache_write_price_cny: '0', cache_read_price_cny: '0', per_request_price_cny: '0',
        billing_cycle: 'monthly', fixed_unit_cost_cny: '375', monthly_unit_cost_cny: '375',
        request_count: 0, input_tokens: 0, output_tokens: 0, cache_write_tokens: 0, cache_read_tokens: 0, amount_cny: '375',
      }],
    })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('真实总成本'))!.trigger('click')
    await flushPromises()

    expect(api.breakdown).toHaveBeenCalledWith(expect.objectContaining({ scope: 'total', start_date: expect.any(String), end_date: expect.any(String) }))
    const dialog = wrapper.findAll('[data-test="dialog"]').find(item => item.text().includes('真实总成本组成'))!
    expect(dialog.text()).toContain('Kimi 套餐 199')
    expect(dialog.text()).toContain('Kimi 199')
    expect(dialog.text()).toContain('¥375.00')
    await dialog.findAll('button').find(button => button.text() === '导出 Excel')!.trigger('click')
    await flushPromises()
    expect(xlsx.aoaToSheet.mock.calls[0][0][4]).toEqual(['类型','成本方案','账号/订阅实例','上游模型','计价依据','请求数','输入 Token','缓存写入 Token','缓存读取 Token','输出 Token','金额（CNY）'])
    expect(xlsx.aoaToSheet.mock.calls[0][0][5]).toEqual(expect.arrayContaining(['固定','Kimi 套餐 199','Kimi 199',375]))
    expect(xlsx.writeFile).toHaveBeenCalledWith(expect.anything(),expect.stringMatching(/^成本组成_total_.+\.xlsx$/))
    expect(app.showSuccess).toHaveBeenCalledWith('成本组成已导出')
  })

  it('shows the full-month estimate for the current month', async () => {
    api.overview.mockResolvedValue({
      dynamic_cost_cny: '2683.22', fixed_cost_cny: '1370.33', total_cost_cny: '4053.55',
      estimated_total_cost_cny: '7033.22', pending_count: 0, error_count: 0, eligible_count: 0, calculated_count: 0,
      coverage_complete: true, previous_total_cost_cny: '0',
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('本月预估成本')
    expect(wrapper.text()).toContain('¥7,033.22')
    expect(wrapper.text()).toContain('整月固定成本 + 当前按量成本')
  })

  it('groups cost rows by plan and compacts token counts', async () => {
    const item = {
      cost_mode: 'metered' as const, plan_name: '阿里云 GLM', subscription_unit_name: '', upstream_model: 'glm-5.2',
      billing_mode: 'token', input_price_cny: '4', output_price_cny: '14', cache_write_price_cny: '4', cache_read_price_cny: '1', per_request_price_cny: '0',
      billing_cycle: '', fixed_unit_cost_cny: '0', monthly_unit_cost_cny: '0', cache_write_tokens: 0,
    }
    api.breakdown.mockResolvedValue({
      total_cost_cny: '2379.82',
      items: [
        { ...item, account_name: 'GLM Anthropic', request_count: 16895, input_tokens: 157530205, cache_read_tokens: 1097089257, output_tokens: 8729142, amount_cny: '1849.42' },
        { ...item, account_name: 'GLM OpenAI', request_count: 4105, input_tokens: 33709495, cache_read_tokens: 352375296, output_tokens: 3084749, amount_cny: '530.40' },
      ],
    })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('真实总成本'))!.trigger('click')
    await flushPromises()

    const dialog = wrapper.findAll('[data-test="dialog"]').find(node => node.text().includes('真实总成本组成'))!
    expect(dialog.findAll('td').filter(cell => cell.text() === '阿里云 GLM')).toHaveLength(1)
    expect(dialog.get('td[rowspan="2"]').text()).toBe('阿里云 GLM')
    expect(dialog.text()).toContain('157.5m')
    expect(dialog.text()).toContain('1.1b')
    expect(dialog.get('td[title="1,097,089,257"]').text()).toBe('1.1b')
  })

  it('shows pending reasons and creates a recalculation for the current range', async () => {
    api.overview.mockResolvedValue({
      dynamic_cost_cny: '0', fixed_cost_cny: '0', total_cost_cny: '0',
      pending_count: 12, error_count: 0, eligible_count: 0, calculated_count: 0,
      coverage_complete: true, previous_total_cost_cny: '0',
    })
    api.pendingDetails.mockResolvedValue({
      total_count: 12,
      items: [{
        account_id: 7, account_name: 'DeepSeek', start_date: '2026-07-01', end_date: '2026-07-03',
        issue_code: 'missing_model_price', upstream_model: 'deepseek-v4-pro', pending_count: 12,
      }],
    })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('待核算'))!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('成本方案缺少该上游模型价格')
    expect(wrapper.text()).toContain('deepseek-v4-pro')
    await wrapper.findAll('button').find(button => button.text() === '补算当前统计范围')!.trigger('click')
    await wrapper.get('[data-test="confirm-dialog"]').find('button').trigger('click')
    await flushPromises()
    expect(api.createRecalculation).toHaveBeenCalledWith(expect.objectContaining({ start_date: expect.any(String), end_date: formatDateLocalInput(new Date()) }))
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

  it('renders the cost trend as stacked bars', async () => {
    const wrapper = mountView()
    await flushPromises()

    const config = chartConfigs[chartConfigs.length - 1]
    expect(config.type).toBe('bar')
    expect(config.data.datasets.map((dataset: any) => dataset.backgroundColor)).toEqual(['#0866ed', '#ff7800'])
    expect(config.data.datasets.every((dataset: any) => dataset.stack === 'cost')).toBe(true)
    expect(wrapper.text()).toContain('时间指标')
  })

  it('shows cost plan details in the stacked bar tooltip', async () => {
    api.analysis.mockResolvedValue({
      period: 'day', total_cost_cny: '909.014', top: [],
      trend: [{ bucket: '2026-08-28', dynamic_cost_cny: '710.014', fixed_cost_cny: '199', total_cost_cny: '909.014', plans: [
        { plan_id: 1, plan_name: '阿里云', plan_type: 'metered', amount_cny: '600' },
        { plan_id: 2, plan_name: 'Kimi 套餐 199', plan_type: 'fixed', amount_cny: '199' },
      ] }],
    })
    mountView()
    await flushPromises()

    const callbacks = chartConfigs.at(-1).options.plugins.tooltip.callbacks
    expect(callbacks.afterBody([{ dataIndex: 0, datasetIndex: 0 }])).toEqual(['', '阿里云: ¥600.00'])
    expect(callbacks.afterBody([{ dataIndex: 0, datasetIndex: 1 }])).toEqual(['', 'Kimi 套餐 199: ¥199.00'])
    expect(callbacks.footer([{ dataIndex: 0 }])).toBe('总成本: ¥909.01')
  })

  it('requests analysis using the selected date range', async () => {
    api.analysis.mockResolvedValue({
      period: 'day', total_cost_cny: '909.014', top: [],
      trend: [{ bucket: '2026-08-28', dynamic_cost_cny: '710.014', fixed_cost_cny: '199', total_cost_cny: '909.014', plans: [
        { plan_id: 1, plan_name: '阿里云', plan_type: 'metered', amount_cny: '600' },
        { plan_id: 2, plan_name: 'Kimi 套餐 199', plan_type: 'fixed', amount_cny: '199' },
      ] }],
    })
    mountView()
    await flushPromises()

    expect(api.analysis).toHaveBeenCalledWith(expect.objectContaining({ start_date: expect.any(String), end_date: expect.any(String), period: expect.any(String) }))
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

  it('shows complete token prices and time rules in price history', async () => {
    api.priceHistory.mockResolvedValue([{
      id: 21, plan_id: 11, version_no: 1, effective_from: '2026-08-01T00:00:00+08:00',
      subscription_unit_name: '', billing_cycle: '', fixed_unit_cost_cny: '0', monthly_unit_cost_cny: '0',
      prices: [{
        upstream_model: 'deepseek-v4-flash', billing_mode: 'token', input_price_cny: '1.5', output_price_cny: '4.5',
        cache_write_price_cny: '1.5', cache_read_price_cny: '0.05', image_input_price_cny: '0', image_output_price_cny: '0', per_request_price_cny: '0',
        time_pricing: { timezone: 'Asia/Shanghai', weekdays_only: true, periods: [{ start_time: '09:00:00', end_time: '12:00:00', multiplier: 2 }] },
      }],
    }])
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '成本方案')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '价格历史')!.trigger('click')
    await flushPromises()

    const dialog = wrapper.findAll('[data-test="dialog"]').find(item => item.text().includes('价格历史 · GLM 按量'))!
    expect(dialog.text()).toContain('缓存读取 ¥0.05/MTok')
    expect(dialog.text()).toContain('仅工作日 · Asia/Shanghai · 09:00:00-12:00:00 ×2')
  })

  it('copies model prices and time periods without sharing state', async () => {
    api.plan.mockResolvedValue({
      id: 11, name: 'GLM 按量', plan_type: 'metered', status: 'active',
      effective_from: '2026-01-01T02:12:00Z', billing_cycle: 'monthly', fixed_unit_cost_cny: '0', note: '',
      prices: [{
        upstream_model: 'glm', billing_mode: 'token', input_price_cny: '1', output_price_cny: '2',
        cache_write_price_cny: '3', cache_read_price_cny: '4', image_input_price_cny: '5',
        image_output_price_cny: '6', per_request_price_cny: '7',
        time_pricing: { timezone: 'Asia/Shanghai', weekdays_only: true, periods: [{ start_time: '00:00:00', end_time: '08:00:00', multiplier: 2 }] },
      }],
    })
    api.modelOptions.mockResolvedValue({ items: [{ model: 'glm' }], total: 1 })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '成本方案')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '价格变更')!.trigger('click')
    await flushPromises()

    await wrapper.get('[data-test="copy-price"]').trigger('click')
    const prices = wrapper.findAll('[data-test="model-price"]')
    expect(prices).toHaveLength(2)
    expect((prices[0].get('select').element as HTMLSelectElement).value).toBe('glm')
    expect((prices[1].get('select').element as HTMLSelectElement).value).toBe('')
    expect(prices[1].findAll('input[type="number"]').map(input => (input.element as HTMLInputElement).value))
      .toEqual(prices[0].findAll('input[type="number"]').map(input => (input.element as HTMLInputElement).value))

    await prices[1].get('input[inputmode="numeric"]').setValue('01:00:00')
    expect((prices[0].get('input[inputmode="numeric"]').element as HTMLInputElement).value).toBe('00:00:00')
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
