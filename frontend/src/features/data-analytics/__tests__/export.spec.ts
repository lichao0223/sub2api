import { describe, expect, it } from 'vitest'
import * as XLSX from 'xlsx'
import { buildAnalyticsWorkbook } from '../exportWorkbook'
import type { AnalyticsReport, AnalyticsValues } from '@/api/admin/dataAnalytics'

const values: AnalyticsValues = {
  input:'1000000',output:'2000',cache_write:'0',cache_read:'3000',tokens:'1005000',requests:'2',
  metered:'0.123456789012',fixed:'1.000000000001',cost:'1.123456789013',pending:'0',excluded_tokens:'0',
  input_cost:'0.123456789012',output_cost:'0',write_cost:'0',read_cost:'0',image_cost:'0',request_cost:'0',
}
describe('analytics workbook', () => {
  it('writes eight sheets with compact tokens, exact costs and literal untrusted names', async () => {
    const row={...values,user_id:1,name:'=HYPERLINK("bad")',person_id:'p',org_ids:['org'],org_names:['部门'],model:'@模型',mode:'metered',source_id:1,source:'采购方案',issue:'',attribution:1}
    const report={period:{year:2026,mode:'month',months:[11],keys:['2026-11-01'],grain:'day'},summary:values,
      people:[row],models:[row],departments:[],trend:[{bucket:'2026-11-06',tokens:'1005000'}],fixed_shares:[],
      public_cost:'0',users:1,versions:{'2026-11-01':1},coverage:{history_basis:'当前目录回溯',completed_days:6,expected_days:6,pending_requests:0,complete:true},
    } as unknown as AnalyticsReport
    const result=await buildAnalyticsWorkbook(report)
    const book=XLSX.read(result.buffer,{type:'array'})
    expect(book.SheetNames).toHaveLength(8)
    const sheet=book.Sheets['个人使用']!
    expect(sheet['B2'].t).toBe('s')
    expect(sheet['B2'].f).toBeUndefined()
    expect(sheet['B2'].v).toBe(row.name)
    const rows=XLSX.utils.sheet_to_json<Record<string,string>>(sheet)
    expect(rows[0]!['Token总数']).toBe('1M')
    expect(rows[0]!['总成本(CNY)']).toBe(values.cost)
  })
})
