import type { AnalyticsReport, AnalyticsRow, AnalyticsValues } from '@/api/admin/dataAnalytics'
import { tokenText } from './format'

const usage = (r: AnalyticsValues) => ({
  'Token总数': tokenText(r.tokens), '输入': tokenText(r.input), '输出': tokenText(r.output),
  '缓存写入': tokenText(r.cache_write), '缓存读取': tokenText(r.cache_read), '请求数': r.requests,
  '按量成本(CNY)': r.metered, '固定成本(CNY)': r.fixed, '总成本(CNY)': r.cost,
  '不计成本Token': tokenText(r.excluded_tokens), '待核算请求': r.pending,
})
const identity = (r: AnalyticsRow) => ({ '平台用户ID': String(r.user_id), '姓名': r.name, '外部人员ID': r.person_id, '组织路径': r.org_names.slice(0,3).join(' / ') || '待确认归属' })
export async function buildAnalyticsWorkbook(report: AnalyticsReport) {
  const XLSX = await import('xlsx')
  const workbook = XLSX.utils.book_new()
  const append = (name: string, rows: Record<string, string>[]) => {
    const sheet = XLSX.utils.json_to_sheet(rows.length ? rows : [{ '说明': '当前范围暂无数据' }])
    // All cells remain literal strings, including names beginning with =, +, -, or @.
    for (const key of Object.keys(sheet)) {
      if (!key.startsWith('!') && sheet[key]) { sheet[key].t = 's'; delete sheet[key].f }
    }
    sheet['!cols'] = Array.from({ length: Object.keys(rows[0] || {}).length }, () => ({ wch: 22 }))
    XLSX.utils.book_append_sheet(workbook, sheet, name)
  }
  append('总览', [{ ...usage(report.summary), '使用人数': String(report.users), '公共固定成本(CNY)': report.public_cost }])
  append('部门使用', report.departments.map(r => ({ '部门ID': r.id, '部门': r.name, '上级ID': r.parent, '级别': String(r.level), '人数': String(r.users), ...usage(r) })))
  append('个人使用', report.people.map(r => ({ ...identity(r), ...usage(r) })))
  append('个人模型明细', report.models.map(r => ({ ...identity(r), '模型': r.model, '成本方式': r.mode, '成本来源': r.source, '待核算原因': r.issue, ...usage(r) })))
  append('成本组成', report.models.map(r => ({ ...identity(r), '模型': r.model, '成本来源': r.source,
    '输入成本(CNY)': r.input_cost, '输出成本(CNY)': r.output_cost, '缓存写入成本(CNY)': r.write_cost,
    '缓存读取成本(CNY)': r.read_cost, '图片成本(CNY)': r.image_cost, '请求成本(CNY)': r.request_cost,
    '固定分摊(CNY)': r.fixed, '总成本(CNY)': r.cost })))
  append('固定成本分摊', report.fixed_shares.map(r => ({ '月份': r.month.slice(0, 7), '订阅实例ID': String(r.unit_id), '订阅实例': r.name,
    '平台用户ID': String(r.user_id), '实例总成本(CNY)': r.total, '参与人数': String(r.participants), '个人分摊(CNY)': r.amount, '公共成本(CNY)': r.public })))
  append('Token趋势', report.trend.map(r => ({ '时间': r.bucket, 'Token总数': tokenText(r.tokens) })))
  append('统计口径', [
    { '项目': '时间范围', '内容': `${report.period.year}年 ${report.period.months.join('、')}月（北京时间）` },
    { '项目': '部门范围', '内容': report.scope_org_id || '全部部门（含待归属和公共成本）' },
    { '项目': '可用源历史', '内容': report.coverage.raw_history_start || '尚未开始核算' },
    { '项目': '历史归属', '内容': report.coverage.history_basis },
    { '项目': '固定分摊', '内容': '每月每个订阅实例，由当月有有效使用的规范用户均摊；无人使用计公共成本。当前月为暂估，随参与人变化重分摊。' },
    { '项目': 'Token口径', '内容': '输入+输出+缓存写入+缓存读取，包括不计成本模型；均格式化为B/M/K，非零不足1K显示＜1K。' },
    { '项目': '成本口径', '内容': '人民币采购成本，按量依据成本方案历史价格，固定依据订阅实例有效期；不使用销售金额。金额为精确小数字符串。' },
    { '项目': '发布版本', '内容': JSON.stringify(report.versions) },
    { '项目': '核算覆盖', '内容': `${report.coverage.completed_days}/${report.coverage.expected_days}天；待核算${report.coverage.pending_requests}条；${report.coverage.complete ? '完整' : '尚未完整'}` },
  ])
  return { buffer: XLSX.write(workbook, { bookType: 'xlsx', type: 'array' }) as ArrayBuffer, filename: `数据分析_${report.period.year}_${report.period.months.join('-')}.xlsx` }
}
