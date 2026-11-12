import { afterEach, describe, expect, it, vi } from 'vitest'
import * as XLSX from 'xlsx'
import { buildAnalyticsWorkbook, type AnalyticsExportCosts } from '../exportWorkbook'
import { exportAnalytics, loadAnalyticsCosts } from '../export'
import type { AnalyticsReport, AnalyticsValues } from '@/api/admin/dataAnalytics'

const api = vi.hoisted(() => ({ breakdown: vi.fn() }))
vi.mock('@/api/admin/costManagement', () => ({ costManagementAPI: api }))
const values: AnalyticsValues = {
  input:'1000000',output:'2000',cache_write:'0',cache_read:'3000',tokens:'1005000',requests:'2',
  metered:'0.123456789012',fixed:'1.000000000001',cost:'1.123456789013',pending:'0',excluded_tokens:'0',
  input_cost:'0.123456789012',output_cost:'0',write_cost:'0',read_cost:'0',image_cost:'0',request_cost:'0',
}
const row = {...values,user_id:1,name:'=HYPERLINK("bad")',person_id:'private-person-id',org_ids:['private-org-id'],org_names:['部门'],model:'@模型',mode:'metered',source_id:1,source:'采购方案',issue:'',attribution:1}
const report = {
  scope_org_id:'private-org-id',period:{year:2026,mode:'month',months:[11],keys:['2026-11-01'],grain:'day'},summary:values,
  people:[row],models:[row],departments:[{...values,id:'private-org-id',parent:'',name:'部门',level:1,users:1}],
  trend:[{bucket:'2026-11-06',tokens:'1005000',metered_tokens:'1000000',fixed_tokens:'5000',excluded_tokens:'0'},{bucket:'2026-11-07',tokens:'2500123'}],fixed_shares:[],
  public_cost:'0',users:1,versions:{'2026-11-01':1},coverage:{history_basis:'当前目录回溯',completed_days:6,expected_days:6,pending_requests:0,complete:true},
} as unknown as AnalyticsReport
const costs: AnalyticsExportCosts[] = [{
  month:'2026-11',start_date:'2026-11-01',end_date:'2026-11-07',total_cost_cny:'375.123456789012',
  items:[{cost_mode:'metered',plan_name:'=成本方案',account_name:'采购账号',subscription_unit_name:'',upstream_model:'@上游模型',
    billing_mode:'token',input_price_cny:'0.000012',output_price_cny:'2',cache_write_price_cny:'0',cache_read_price_cny:'0',per_request_price_cny:'0',
    billing_cycle:'',fixed_unit_cost_cny:'0',monthly_unit_cost_cny:'0',request_count:3,input_tokens:2000000,output_tokens:1000,cache_write_tokens:0,cache_read_tokens:0,amount_cny:'375.123456789012'}],
}]
afterEach(() => { vi.useRealTimers(); vi.clearAllMocks(); vi.unstubAllGlobals() })
const zipText = (zip: unknown, path: string) => new TextDecoder().decode(XLSX.CFB.find(zip, `/${path}`).content)

describe('analytics workbook', () => {
  it('exports readable sheets without internal IDs, exact costs and literal untrusted names', async () => {
    const result = await buildAnalyticsWorkbook({report,costs})
    const book = XLSX.read(result.buffer,{type:'array'})
    expect(book.SheetNames).toEqual(['总览','部门使用','个人使用','个人模型明细','成本组成','Token趋势','统计口径'])
    const sheet = book.Sheets['个人使用']!
    expect(sheet.A2).toMatchObject({t:'s',v:row.name})
    expect(sheet.A2.f).toBeUndefined()
    const rows = XLSX.utils.sheet_to_json<Record<string,string>>(sheet)
    expect(rows[0]!['Token总数']).toBe('1M')
    expect(rows[0]!['总成本(CNY)']).toBe(values.cost)
    const costRows = XLSX.utils.sheet_to_json<Record<string,string>>(book.Sheets['成本组成']!)
    expect(costRows[0]).toMatchObject({'成本方案':'=成本方案','账号/订阅实例':'采购账号','上游模型':'@上游模型','金额（CNY）':'375.123456789012'})
    expect(costRows[0]!['计价依据']).toContain('¥0.000012/MTok')
    expect(book.Sheets['成本组成']!.C2.f).toBeUndefined()
    const content = book.SheetNames.map(name => XLSX.utils.sheet_to_csv(book.Sheets[name]!)).join('\n')
    for (const text of ['private-person-id','private-org-id','用户ID','发布版本']) expect(content).not.toContain(text)
    expect(content).toContain('全部账号/订阅实例，不按部门或人员分摊')
    expect(content).toContain('375.123456789012')
  })
  it.each(['day','month'])('embeds a native %s trend chart with numeric sources and valid relationships', async grain => {
    const result = await buildAnalyticsWorkbook({report:{...report,period:{...report.period,grain}},costs})
    const zip = XLSX.CFB.read(new Uint8Array(result.buffer),{type:'buffer'})
    const chart = zipText(zip,'xl/charts/chart1.xml')
    const doc = new DOMParser().parseFromString(chart,'application/xml')
    expect(doc.querySelector('parsererror')).toBeNull()
    expect(doc.getElementsByTagNameNS('*','lineChart')).toHaveLength(1)
    expect(doc.getElementsByTagNameNS('*','ser')).toHaveLength(4)
    expect(chart).toContain("'Token趋势'!$B$2:$B$3")
    expect(chart).toContain('<c:v>2500123</c:v>')
    for (const path of ['xl/worksheets/sheet6.xml','xl/worksheets/_rels/sheet6.xml.rels','xl/drawings/drawing1.xml','xl/drawings/_rels/drawing1.xml.rels','[Content_Types].xml']) {
      expect(new DOMParser().parseFromString(zipText(zip,path),'application/xml').querySelector('parsererror')).toBeNull()
    }
    expect(zipText(zip,'xl/worksheets/sheet6.xml')).toContain('r:id="rIdTrendChart"')
    expect(zipText(zip,'xl/worksheets/_rels/sheet6.xml.rels')).toContain('Target="../drawings/drawing1.xml"')
    expect(zipText(zip,'xl/drawings/_rels/drawing1.xml.rels')).toContain('Target="../charts/chart1.xml"')
    expect(zipText(zip,'[Content_Types].xml')).toContain('PartName="/xl/charts/chart1.xml"')
    const book = XLSX.read(result.buffer,{type:'array'})
    expect(book.Sheets['Token趋势']!.B2).toMatchObject({t:'n',v:1005000})
    expect(book.Sheets['Token趋势']!.B3.v).toBe(2500123)
    expect(book.Sheets['Token趋势']!.C2).toMatchObject({t:'n',v:1000000})
    expect(book.Sheets['Token趋势']!.D2).toMatchObject({t:'n',v:5000})
    expect(book.Sheets['Token趋势']!.E2).toMatchObject({t:'n',v:0})
    expect(book.Sheets['Token趋势']!.C3.v).toBe('待补算')
  })
  it('exports an empty period without an invalid chart range', async () => {
    const result = await buildAnalyticsWorkbook({report:{...report,trend:[]},costs:[]})
    const book = XLSX.read(result.buffer,{type:'array'})
    expect(book.Sheets['Token趋势']!.A2.v).toBe('当前范围暂无数据')
    expect(XLSX.CFB.find(XLSX.CFB.read(new Uint8Array(result.buffer),{type:'buffer'}),'/xl/charts/chart1.xml')).toBeNull()
  })
})
describe('cost management export source', () => {
  it('fetches only selected months and caps current month at today in Beijing', async () => {
    vi.useFakeTimers(); vi.setSystemTime(new Date('2026-11-08T17:00:00Z'))
    api.breakdown.mockResolvedValue({total_cost_cny:'1',items:[]})
    const signal = new AbortController().signal
    const result = await loadAnalyticsCosts({...report,period:{...report.period,months:[1,3,11,12]}},signal)
    expect(api.breakdown.mock.calls).toEqual([
      [{start_date:'2026-01-01',end_date:'2026-01-31',scope:'total'},signal],
      [{start_date:'2026-03-01',end_date:'2026-03-31',scope:'total'},signal],
      [{start_date:'2026-11-01',end_date:'2026-11-09',scope:'total'},signal],
    ])
    expect(result.map(c => c.month)).toEqual(['2026-01','2026-03','2026-11'])
  })
  it('fails when cost data fails instead of downloading an empty cost sheet', async () => {
    api.breakdown.mockRejectedValueOnce(new Error('成本组成加载失败'))
    const worker = vi.fn(); vi.stubGlobal('Worker',worker)
    await expect(exportAnalytics(report)).rejects.toThrow('成本组成加载失败')
    expect(worker).not.toHaveBeenCalled()
  })
  it('does not fetch cost data after cancellation', async () => {
    const abort = new AbortController(); abort.abort()
    await expect(loadAnalyticsCosts(report,abort.signal)).rejects.toThrow()
    expect(api.breakdown).not.toHaveBeenCalled()
  })
})
