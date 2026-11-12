import type { AnalyticsReport, AnalyticsRow, AnalyticsValues } from '@/api/admin/dataAnalytics'
import type { CostBreakdown } from '@/api/admin/costManagement'
import { groupBreakdownItems, pricingText } from '@/features/cost-management/breakdown'
import { sumDecimalStrings, tokenText } from './format'
import { addTrendChart } from './exportChart'

export interface AnalyticsExportCosts extends CostBreakdown { month: string; start_date: string; end_date: string }
export interface AnalyticsExportData { report: AnalyticsReport; costs: AnalyticsExportCosts[] }
const usage = (r: AnalyticsValues) => ({
  'Token总数': tokenText(r.tokens), '输入': tokenText(r.input), '输出': tokenText(r.output),
  '缓存写入': tokenText(r.cache_write), '缓存读取': tokenText(r.cache_read), '请求数': r.requests,
  '按量成本(CNY)': r.metered, '固定成本(CNY)': r.fixed, '总成本(CNY)': r.cost,
  '不计成本Token': tokenText(r.excluded_tokens), '待核算请求': r.pending,
})
const identity = (r: AnalyticsRow) => ({ '姓名': r.name, '组织路径': r.org_names.slice(0, 3).join(' / ') || '待确认归属' })
export async function buildAnalyticsWorkbook({ report, costs }: AnalyticsExportData) {
  const XLSX = await import('xlsx')
  const workbook = XLSX.utils.book_new()
  const append = (name: string, rows: Record<string, string | number>[]) => {
    const sheet = XLSX.utils.json_to_sheet(rows.length ? rows : [{ '说明': '当前范围暂无数据' }])
    // Untrusted text stays literal; numeric chart sources stay numeric.
    for (const key of Object.keys(sheet)) {
      if (!key.startsWith('!') && sheet[key]) {
        if (typeof sheet[key].v === 'string') sheet[key].t = 's'
        delete sheet[key].f
      }
    }
    sheet['!cols'] = Array.from({ length: Object.keys(rows[0] || {}).length }, () => ({ wch: 22 }))
    XLSX.utils.book_append_sheet(workbook, sheet, name)
    return sheet
  }
  append('总览', [{ ...usage(report.summary), '使用人数': String(report.users), '公共固定成本(CNY)': report.public_cost }])
  append('部门使用', report.departments.map(r => ({ '部门': r.name, '上级部门': report.departments.find(d => d.id === r.parent)?.name || '', '人数': String(r.users), ...usage(r) })))
  append('个人使用', report.people.map(r => ({ ...identity(r), ...usage(r) })))
  append('个人模型明细', report.models.map(r => ({ ...identity(r), '模型': r.model,
    '成本方式': ({ fixed: '固定成本', metered: '按量成本', excluded: '不计成本' } as Record<string, string>)[r.mode] || r.mode,
    '成本来源': r.source, ...usage(r) })))
  const costSheet = append('成本组成', costs.flatMap(cost => groupBreakdownItems(cost.items).map(({ item }) => ({
    '月份': cost.month, '类型': item.cost_mode === 'metered' ? '按量' : '固定', '成本方案': item.plan_name || '未匹配方案',
    '账号/订阅实例': item.account_name || item.subscription_unit_name || '', '上游模型': item.upstream_model || '',
    '计价依据': pricingText(item), '请求数': item.request_count, '输入 Token': item.input_tokens,
    '缓存写入 Token': item.cache_write_tokens, '缓存读取 Token': item.cache_read_tokens, '输出 Token': item.output_tokens,
    '金额（CNY）': item.amount_cny,
  }))))
  costSheet['!cols'] = [10, 8, 24, 28, 24, 90, 12, 18, 18, 18, 18, 24].map(wch => ({ wch }))
  XLSX.utils.sheet_add_aoa(costSheet, [[], ['统计范围', '成本管理：全部账号/订阅实例，不按部门或人员分摊'],
    ['统计日期', costs.map(c => `${c.start_date} 至 ${c.end_date}`).join('；')],
    ['合计（CNY）', sumDecimalStrings(costs.map(c => c.total_cost_cny))]], { origin: -1 })
  const trendSheet = append('Token趋势', report.trend.map(r => ({ '时间': r.bucket, 'Token总数': Number(r.tokens),
    '按量方案': r.metered_tokens === undefined ? '待补算' : Number(r.metered_tokens),
    '固定成本': r.fixed_tokens === undefined ? '待补算' : Number(r.fixed_tokens),
    '不纳入核算': r.excluded_tokens === undefined ? '待补算' : Number(r.excluded_tokens),
  })))
  for (let i = 0; i < report.trend.length; i++) for (const column of ['B','C','D','E']) trendSheet[`${column}${i + 2}`].z = '#,##0'
  append('统计口径', [
    { '项目': '时间范围', '内容': `${report.period.year}年 ${report.period.months.join('、')}月（北京时间）` },
    { '项目': '部门范围', '内容': report.scope_org_id ? report.departments.find(d => d.id === report.scope_org_id)?.name || '当前所选部门' : '全部部门（含待归属和公共成本）' },
    { '项目': '可用源历史', '内容': report.coverage.raw_history_start || '尚未开始核算' },
    { '项目': '历史归属', '内容': report.coverage.history_basis },
    { '项目': '固定分摊', '内容': '每月每个订阅实例，由当月有有效使用的规范用户均摊；无人使用计公共成本。当前月为暂估，随参与人变化重分摊。' },
    { '项目': 'Token口径', '内容': '输入+输出+缓存写入+缓存读取，包括不计成本模型；使用表以B/M/K显示，趋势及成本组成保留数值。' },
    { '项目': '成本口径', '内容': '人民币采购成本。使用表按所选部门归属；成本组成直接取成本管理的真实总成本组成，为全部账号/订阅实例，不按部门或人员分摊。金额保留精确小数。' },
    { '项目': '成本组成统计范围', '内容': costs.map(c => `${c.start_date} 至 ${c.end_date}`).join('；') || '暂无数据' },
    { '项目': '成本组成合计（CNY）', '内容': sumDecimalStrings(costs.map(c => c.total_cost_cny)) },
    { '项目': '核算覆盖', '内容': `${report.coverage.completed_days}/${report.coverage.expected_days}天；待核算${report.coverage.pending_requests}条；${report.coverage.complete ? '完整' : '尚未完整'}` },
  ])
  const buffer = XLSX.write(workbook, { bookType: 'xlsx', type: 'array' }) as ArrayBuffer
  return {
    buffer: addTrendChart(XLSX, buffer, workbook.SheetNames.indexOf('Token趋势') + 1, report.trend, report.period.grain === 'day'),
    filename: `数据分析_${report.period.year}_${report.period.months.join('-')}.xlsx`,
  }
}
