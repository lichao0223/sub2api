import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import DataAnalyticsView from '../DataAnalyticsView.vue'
import type { AnalyticsQuery, AnalyticsReport, AnalyticsValues } from '@/api/admin/dataAnalytics'

const api=vi.hoisted(()=>({ report:vi.fn(),directory:vi.fn() }))
vi.mock('@/api/admin/dataAnalytics',()=>({dataAnalyticsAPI:api}))
vi.mock('@/stores/app',()=>({useAppStore:()=>({showError:vi.fn(),showSuccess:vi.fn()})}))
vi.mock('chart.js/auto',()=>({Chart:class {destroy(){}}}))
const values:AnalyticsValues={input:'1000000',output:'0',cache_write:'0',cache_read:'0',tokens:'1000000',requests:'1',metered:'1',fixed:'0',cost:'1',pending:'0',excluded_tokens:'0',input_cost:'1',output_cost:'0',write_cost:'0',read_cost:'0',image_cost:'0',request_cost:'0'}
const Select={props:['modelValue','options'],emits:['update:modelValue','change'],template:`<select :value="modelValue" @change="e=>{$emit('update:modelValue',typeof modelValue==='number'?Number(e.target.value):e.target.value);$emit('change')}"><option v-for="o in options" :key="o.value" :value="o.value">{{o.label}}</option></select>`}
afterEach(()=>{vi.useRealTimers();vi.clearAllMocks()})
describe('data analytics linked period selection',()=>{
  it('links the trend to the top month/year selection without a second grain selector',async()=>{
    vi.useFakeTimers();vi.setSystemTime(new Date('2026-11-06T10:00:00Z'))
    api.directory.mockResolvedValue({version:1,directory:{orgs:[],people:[]},identities:[],imports:[]})
    api.report.mockImplementation(async(q:AnalyticsQuery)=>({
      period:{year:q.year,mode:q.mode,months:q.mode==='year'?[1,2,3,4,5,6,7,8,9,10,11]:q.months.split(',').map(Number),keys:[],grain:q.mode==='month'?'day':'month'},
      summary:values,users:1,people:[],people_total:0,models:[],departments:[],trend:[],fixed_shares:[],public_cost:'0',versions:{},
      coverage:{directory_ready:true,issues:[],complete:true,completed_days:1,expected_days:1,backlog_days:0,failed_days:0,pending_requests:0,last_updated:null,last_error:'',provisional:true},
      task_status:{control:{enabled:true,version:1},state:'idle',running:0},
    } as unknown as AnalyticsReport))
    const wrapper=mount(DataAnalyticsView,{global:{stubs:{AppLayout:{template:'<main><slot/></main>'},AnalyticsTaskControl:true,DirectoryDialog:true,Pagination:true,Select,BaseDialog:{props:['show'],template:'<div v-if="show"><slot/></div>'}}}})
    try {
      await flushPromises()
      expect(api.report).toHaveBeenLastCalledWith(expect.objectContaining({year:2026,mode:'month',months:'11'}),expect.anything())
      expect(wrapper.text()).toContain('趋势按天')
      expect(wrapper.text()).toContain('1M')
      await wrapper.findAll('select')[1]!.setValue('year');await flushPromises()
      expect(api.report).toHaveBeenLastCalledWith(expect.objectContaining({mode:'year'}),expect.anything())
      expect(wrapper.text()).toContain('趋势按月')
      expect(wrapper.text()).not.toContain('趋势粒度')
    } finally {wrapper.unmount()}
  })
})
