import type { CostBreakdownItem } from '@/api/admin/costManagement'

export const priceMoney = (value: string | number) => '¥' + Number(value || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 12 })
const money = (value: string) => '¥' + Number(value || 0).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })

export const pricingText = (item: CostBreakdownItem) => item.cost_mode === 'fixed'
  ? `${item.billing_cycle === 'yearly' ? '年付' : '月付'} ${money(item.fixed_unit_cost_cny)}（月均 ${money(item.monthly_unit_cost_cny)}），按订阅有效时长分摊`
  : `输入 ${priceMoney(item.input_price_cny)}/MTok · 缓存写入 ${priceMoney(item.cache_write_price_cny)}/MTok · 缓存读取 ${priceMoney(item.cache_read_price_cny)}/MTok · 输出 ${priceMoney(item.output_price_cny)}/MTok${Number(item.per_request_price_cny) ? ` · 请求 ${priceMoney(item.per_request_price_cny)}/次` : ''}`

export function groupBreakdownItems(items: CostBreakdownItem[]) {
  const groups = new Map<string, CostBreakdownItem[]>()
  for (const item of items) {
    const key = `${item.cost_mode}\0${item.plan_name}`
    const group = groups.get(key)
    group ? group.push(item) : groups.set(key, [item])
  }
  return [...groups.values()]
    .sort((a, b) => b.reduce((sum, item) => sum + Number(item.amount_cny), 0) - a.reduce((sum, item) => sum + Number(item.amount_cny), 0))
    .flatMap(group => group.map((item, index) => ({ item, rowspan: index ? 0 : group.length })))
}
