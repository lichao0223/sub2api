import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import WorkInsightView from '../WorkInsightView.vue'
import { TASK_CATEGORIES } from '../types'
import type { WorkInsightConfig } from '../types'

const api = vi.hoisted(() => ({ getConfig: vi.fn(), updateConfig: vi.fn(), getRuntime: vi.fn(), analyzeNow: vi.fn(), listSamples: vi.fn(), listBatches: vi.fn(), clearLogs: vi.fn(), retryBatch: vi.fn(), listAnalyzerAccounts: vi.fn(), probe: vi.fn(), listRanking: vi.fn(), getOverview: vi.fn(), getDaily: vi.fn(), listRepresentativeItems: vi.fn() }))
vi.mock('../api', () => ({ default: api }))

const config = (): WorkInsightConfig => ({
  enabled: false, config_version: 1, sample_rate: 20, session_idle_minutes: 5, user_daily_limit: 5000, global_daily_limit: 200000,
  timezone: 'Asia/Shanghai', excluded_user_ids: [], excluded_user_emails: [], queue_capacity: 10000, worker_count: 4,
  analysis_idle_minutes: 15, analysis_max_wait_minutes: 60, analysis_trigger_mode: 'hybrid', analysis_fixed_interval_minutes: 30,
  analysis_fixed_times: [], max_samples_per_batch: 50, context_window_tokens: 128000, max_input_tokens: 64000,
  reserved_output_tokens: 4000, analysis_timeout_seconds: 60, max_job_age_minutes: 90, payload_ttl_minutes: 120,
  daily_finalize_time: '00:15', store_redacted_preview: false, sample_retention_days: 90, insight_retention_days: 180,
  cleanup_enabled: true, cleanup_time: '03:30', cleanup_batch_size: 5000, analyzer_source: 'account', analyzer_account_id: 1,
  analyzer_model: 'model-canary', analyzer_token_set: false, updated_at: '', updated_by: 0,
})

const row = { id: 7, user_id: 9, username: '测试用户', insight_date: '2026-08-11', business_request_count: 10, business_total_tokens: 1234, business_input_tokens: 700, business_output_tokens: 300, business_cache_creation_tokens: 100, business_cache_read_tokens: 134, business_error_count: 1, average_duration_ms: 800, p95_duration_ms: 1500, model_usage: { 'model-canary': 10 }, sample_count: 3, failed_sample_count: 0, eligible_active_session_count: 2, covered_active_session_count: 1, task_category_stats: { 问题排查: 2 }, explicit_projects: ['sub2api'], explicit_modules: ['gateway'], change_types: ['Bug 修复'], business_topics: ['路由'], daily_summary: '排查网关问题。', last_analyzed_at: '2026-08-11T02:00:00Z' }
const rankingRow = { latest_insight_id: 7, user_id: 9, username: '测试用户', start_date: '2026-08-10', end_date: '2026-08-11', insight_days: 2, business_request_count: 20, business_total_tokens: 2468, sample_count: 6, failed_sample_count: 0, eligible_active_session_count: 4, covered_active_session_count: 2, latest_summary: '排查网关问题。', analyzed: true }

const DataTableStub = defineComponent({
  props: ['data'], emits: ['rowClick'],
  template: '<div><button v-for="row in data" :key="row.latest_insight_id" data-test="row" @click="$emit(\'rowClick\', row)">{{ row.username }}</button></div>'
})
const BaseDialogStub = defineComponent({ props: ['show', 'title'], emits: ['close'], template: '<div v-if="show" role="dialog"><h2>{{ title }}</h2><slot /></div>' })
const ConfirmDialogStub = defineComponent({ props: ['show'], emits: ['confirm', 'cancel'], template: '<div v-if="show" data-test="clear-confirm"><button data-test="clear-confirm-action" @click="$emit(\'confirm\')">confirm</button></div>' })

function mountView() {
  return mount(WorkInsightView, {
    global: { stubs: {
      AppLayout: { template: '<main><slot /></main>' }, DataTable: DataTableStub, Pagination: true,
      StatusBadge: true, Toggle: true, BaseDialog: BaseDialogStub, ConfirmDialog: ConfirmDialogStub,
    } }
  })
}

describe('WorkInsightView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getConfig.mockResolvedValue(config())
    api.getRuntime.mockResolvedValue({ enabled: false, queue_depth: 0, queue_capacity: 10000, dropped: 0, processed: 0, failed: 0 })
    api.analyzeNow.mockResolvedValue({ created_batches: 2 })
    api.clearLogs.mockResolvedValue({ samples: 9, batches: 1 })
    api.retryBatch.mockResolvedValue({ batch_id: 12 })
    api.listSamples.mockResolvedValue([{ id: 11, user_id: 9, username: '测试用户', provider: 'openai', requested_model: 'model-canary', sample_reason: 'compact', estimated_tokens: 4082, prompt_chars: 12651, analyzed_chars: 12246, truncated: false, status: 'pending_batch', error_code: '', created_at: '2026-08-11T02:00:00Z' }])
    api.listBatches.mockResolvedValue([
      { id: 13, user_id: 9, username: '测试用户', sample_count: 1, trigger_reason: 'manual', status: 'processing', attempts: 2, error_code: 'summary_write_failed', analyzer_model: '', analyzer_input_tokens: 0, analyzer_output_tokens: 0, created_at: '2026-08-11T02:02:00Z' },
      { id: 12, user_id: 9, username: '测试用户', sample_count: 1, trigger_reason: 'manual', status: 'failed', attempts: 3, error_code: 'summary_write_conflict', analyzer_model: '', analyzer_input_tokens: 0, analyzer_output_tokens: 0, created_at: '2026-08-11T02:01:00Z' },
    ])
    api.listAnalyzerAccounts.mockResolvedValue([{ id: 1, name: '分析账号', platform: 'openai', models: ['model-canary'] }])
    api.probe.mockResolvedValue({ ok: true, status: 'ok', message: '连接正常', latency_ms: 12, checked_at: '' })
    api.listRanking.mockResolvedValue({ items: [rankingRow], total: 1, page: 1, page_size: 20, pages: 1 })
    api.getOverview.mockResolvedValue({ active_users: 1, insight_users: 1, active_sessions: 2, covered_sessions: 1, sample_requests: 3, business_tokens: 1234, failed_samples: 0, analyzer_input_tokens: 30, analyzer_output_tokens: 20 })
    api.getDaily.mockResolvedValue({ insight: row, representative_items: [{ source_sample_ids: [1], summary: '排查网关问题', task_categories: ['问题排查'], explicit_projects: ['sub2api'], explicit_modules: ['gateway'] }], representative_item_count: 1, representative_items_expired: false })
  })

  it('keeps the documented defaults and fixed category contract', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(api.listRanking).toHaveBeenCalled()
    expect(wrapper.text()).toContain('用户洞察排名')
    expect(TASK_CATEGORIES).toEqual(['代码开发', '问题排查', '测试用例', '接口文档', '需求分析', '方案设计', '数据分析', 'SQL/报表', '运维部署', '日志分析', '文档写作', '翻译润色', '会议纪要', '客服支持', '培训学习', '其他'])
    const tabs = wrapper.findAll('[role="tab"]')
    expect(wrapper.find('[role="tablist"]').exists()).toBe(true)
    expect(tabs[0].attributes('aria-controls')).toBe('work-insight-panel')
    expect(tabs[1].attributes('aria-controls')).toBe('work-config-panel')
    await tabs[1].trigger('click')
    await nextTick()
    expect(wrapper.get('[data-test="sample-rate-config"]').text()).toContain('请求采样率')
    expect(wrapper.get('[data-test="sample-rate-config"]').text()).toContain('默认 20%')
    expect(wrapper.text()).toContain('首请求必采 · 后续 20%')
    const values = wrapper.findAll('input[type="number"]').map(input => input.element.value)
    expect(values).toContain('20')
    expect(values).toContain('200000')
    expect(values).toContain('10000')
    expect(wrapper.text()).toContain('样本/批次保留')
    expect(wrapper.text()).toContain('每日洞察保留')
    expect(wrapper.text()).toContain('分析超时')
    expect(wrapper.text()).toContain('保存脱敏预览')
    wrapper.unmount()
  })

  it('loads the list and opens a privacy-safe daily detail', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('测试用户')
    await wrapper.get('[data-test="row"]').trigger('click')
    await flushPromises()
    expect(api.getDaily).toHaveBeenCalledWith(7)
    expect(wrapper.get('[role="dialog"]').text()).toContain('排查网关问题')
    expect(wrapper.get('[role="dialog"]').text()).toContain('90.9%')
    expect(wrapper.get('[role="dialog"]').text()).toContain('model-canary · 10 次')
    expect(wrapper.get('[role="dialog"]').text()).toContain('P95 耗时')
    expect(wrapper.get('[role="dialog"]').text()).toContain('最后分析')
    expect(wrapper.get('[role="dialog"]').text()).toContain('路由')
    expect(wrapper.get('[role="dialog"]').text()).toContain('不展示 Redis 临时文本')
    wrapper.unmount()
  })

  it('shows privacy-safe logs and manually creates analysis batches', async () => {
    const enabled = config()
    enabled.enabled = true
    api.getConfig.mockResolvedValue(enabled)
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="open-logs"]').trigger('click')
    await flushPromises()
    expect(api.listSamples).toHaveBeenCalled()
    expect(api.listBatches).toHaveBeenCalled()
    expect(wrapper.get('[role="dialog"]').text()).toContain('会话压缩快照')
    expect(wrapper.get('[role="dialog"]').text()).toContain('分析文本估算')
    expect(wrapper.get('[role="dialog"]').text()).toContain('相同字符数不代表输入内容相同')
    expect(wrapper.get('[role="dialog"]').text()).toContain('脱敏后 12,246 / 原文 12,651 字符')
    expect(wrapper.get('[role="dialog"]').text()).not.toContain('已截断')
    expect(wrapper.get('[role="dialog"]').text()).toContain('管理员手动触发')
    expect(wrapper.get('[role="dialog"]').text()).toContain('分析失败')
    expect(wrapper.get('[role="dialog"]').text()).toContain('失败 1')
    expect(wrapper.get('[role="dialog"]').text()).toContain('完成后统计')
    expect(wrapper.get('[role="dialog"]').text()).not.toContain('摘要写入数据库失败')
    const retry = wrapper.findAll('button').find(button => button.text() === '重新分析')
    expect(retry).toBeDefined()
    await retry!.trigger('click')
    await flushPromises()
    expect(api.retryBatch).toHaveBeenCalledWith(12)
    await wrapper.get('[data-test="clear-logs"]').trigger('click')
    await wrapper.get('[data-test="clear-confirm-action"]').trigger('click')
    await flushPromises()
    expect(api.clearLogs).toHaveBeenCalled()
    expect(wrapper.get('[role="dialog"]').text()).toContain('摘要写入版本冲突，自动重试后仍未成功')
    expect(wrapper.get('[role="dialog"]').text()).not.toContain('summary_write_conflict')
    await wrapper.findAll('[role="tab"]')[1].trigger('click')
    await wrapper.get('[data-test="analyze-now"]').trigger('click')
    await flushPromises()
    expect(api.analyzeNow).toHaveBeenCalled()
    expect(wrapper.text()).toContain('已创建 2 个分析批次')
    wrapper.unmount()
  })
})
