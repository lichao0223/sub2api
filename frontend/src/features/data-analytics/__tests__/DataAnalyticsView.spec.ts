import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import DataAnalyticsView from '../DataAnalyticsView.vue'
import type { AnalyticsQuery, AnalyticsReport, AnalyticsValues } from '@/api/admin/dataAnalytics'

const api = vi.hoisted(()=>({report:vi.fn(),directory:vi.fn(),export:vi.fn()}))
vi.mock('@/api/admin/dataAnalytics',()=>({dataAnalyticsAPI:api}))
vi.mock('@/stores/app',()=>({useAppStore:()=>({showError:vi.fn(),showSuccess:vi.fn()})}))
vi.mock('../export',()=>({exportAnalytics:api.export}))
vi.mock('chart.js/auto',()=>({Chart:class {destroy(){}}}))
const values:AnalyticsValues = {input:'1000000',output:'0',cache_write:'0',cache_read:'0',tokens:'1000000',requests:'1',metered:'1',fixed:'0',cost:'1',pending:'0',excluded_tokens:'0',input_cost:'1',output_cost:'0',write_cost:'0',read_cost:'0',image_cost:'0',request_cost:'0'}
const person = {...values,user_id:7,name:'张三',person_id:'person-1',org_ids:['company','center','department'],org_names:['公司','研发中心','平台部'],model:'model',mode:'metered',source_id:1,source:'采购方案',issue:'',attribution:1}
const orgs = ['company','center','department','team'].map((id,index)=>({
  id,parent:index?['company','center','department'][index-1]!:'' ,
  name:['公司','研发中心','平台部','后端组'][index]!,
  path:['company','center','department','team'].slice(0,index+1),names:['公司','研发中心','平台部','后端组'].slice(0,index+1),level:index+1,
}))
const Select = {props:['modelValue','options','ariaLabel'],emits:['update:modelValue','change'],template:`<select :aria-label="ariaLabel" :value="modelValue" @change="e=>{$emit('update:modelValue',typeof modelValue==='number'?Number(e.target.value):e.target.value);$emit('change')}"><option v-for="o in options" :key="o.value" :value="o.value">{{o.label}}</option></select>`}
function mountView(){
  return mount(DataAnalyticsView,{global:{stubs:{
    AppLayout:{template:'<main><slot/></main>'},
    AnalyticsTaskControl:{name:'AnalyticsTaskControl',emits:['loaded'],template:'<div />'},
    DirectoryPanel:{name:'DirectoryPanel',props:['enabled'],template:'<section :data-enabled="enabled">维护最新组织与人员目录</section>'},
    PersonDrawer:{props:['person'],template:'<aside v-if="person">个人使用详情 {{person.name}}</aside>'},
    Icon:true,Pagination:true,Select,
    BaseDialog:{props:['show'],template:'<section v-if="show"><slot/><slot name="footer"/></section>'},
  }}})
}
beforeEach(()=>{
  vi.useFakeTimers();vi.setSystemTime(new Date('2026-11-06T10:00:00Z'))
  api.directory.mockResolvedValue({version:1,directory:{orgs,people:[{id:'person-1',name:'张三',org:'team',active:true}]},identities:[{user_id:7,username:'张三',person_id:'person-1',basis:'manual'}],imports:[]})
  api.report.mockImplementation(async(q:AnalyticsQuery)=>({
    scope_org_id:q.org_id,period:{year:q.year,mode:q.mode,months:q.mode==='year'?[1,2,3,4,5,6,7,8,9,10,11]:q.months.split(',').map(Number),keys:['2026-11-01'],grain:q.mode==='month'?'day':'month'},
    summary:values,users:1,people:[person],people_total:1,models:[],departments:orgs.filter(org=>org.level<=3).map(org=>({...values,id:org.id,name:org.name,parent:org.parent,level:org.level,users:1})),trend:[],fixed_shares:[],public_cost:'0',versions:{'2026-11-01':8},
    coverage:{directory_ready:true,issues:[],complete:true,completed_days:6,expected_days:6,backlog_days:0,failed_days:0,pending_requests:0,last_updated:null,last_error:'',provisional:true},
    task_status:{control:{enabled:true,version:1},state:'idle',running:0},
  } as unknown as AnalyticsReport))
  api.export.mockResolvedValue(undefined)
})
afterEach(()=>{vi.useRealTimers();vi.clearAllMocks()})
describe('prototype analytics workflow',()=>{
  it('restores the four tabs, organization tree and linked top-level year selection',async()=>{
    const wrapper=mountView()
    try{
      await flushPromises()
      expect(wrapper.text()).toContain('看清用量，也看清投入。')
      expect(wrapper.findAll('[role="tab"]')).toHaveLength(4)
      expect(wrapper.findAll('.analytics-stat')).toHaveLength(4)
      expect(wrapper.find('[aria-label="组织架构"]').exists()).toBe(true)
      expect(wrapper.text()).toContain('部门用量排行')
      expect(wrapper.text()).toContain('趋势按天')
      expect(wrapper.text()).toContain('1M')
      await wrapper.get('[aria-label="时间范围模式"]').findAll('button')[0]!.trigger('click')
      await flushPromises()
      expect(api.report).toHaveBeenLastCalledWith(expect.objectContaining({year:2026,mode:'year'}),expect.anything())
      expect(wrapper.text()).toContain('趋势按月')
      expect(wrapper.text()).not.toContain('趋势粒度')
      await wrapper.get('#analytics-tab-maintenance').trigger('click')
      expect(wrapper.text()).toContain('维护最新组织与人员目录')
      expect(wrapper.find('[aria-label="组织架构"]').exists()).toBe(false)
    }finally{wrapper.unmount()}
  })
  it('scopes real data by the organization tree and opens a person from the personnel tab',async()=>{
    const wrapper=mountView()
    try{
      await flushPromises()
      const department=wrapper.findAll('.analytics-tree-label').find(button=>button.text().includes('平台部'))!
      await department.trigger('click');await flushPromises()
      expect(api.report).toHaveBeenLastCalledWith(expect.objectContaining({org_id:'department',page:1}),expect.anything())
      expect(wrapper.find('[aria-label="组织范围"]').text()).toContain('公司')
      await wrapper.get('#analytics-tab-people').trigger('click');await flushPromises()
      await wrapper.get('[aria-label="人员Token与实际成本"] .analytics-table-link').trigger('click')
      expect(wrapper.text()).toContain('个人使用详情 张三')
    }finally{wrapper.unmount()}
  })
  it('only applies nonconsecutive month changes after confirmation',async()=>{
    const wrapper=mountView()
    try{
      await flushPromises()
      await wrapper.get('[aria-label="时间范围模式"]').findAll('button')[2]!.trigger('click');await flushPromises()
      await wrapper.get('[aria-label="选择统计月份"]').trigger('click')
      const count=api.report.mock.calls.length
      await wrapper.get('.analytics-month-grid input[value="1"]').setValue(true)
      await wrapper.get('.analytics-month-grid input[value="3"]').setValue(true)
      await wrapper.get('.analytics-month-grid input[value="11"]').setValue(false)
      expect(api.report).toHaveBeenCalledTimes(count)
      await wrapper.findAll('button').find(button=>button.text()==='应用所选月份')!.trigger('click');await flushPromises()
      expect(api.report).toHaveBeenLastCalledWith(expect.objectContaining({mode:'months',months:'1,3'}),expect.anything())
    }finally{wrapper.unmount()}
  })
  it('exports the full selected organization with pinned versions and no personnel search',async()=>{
    const wrapper=mountView()
    try{
      await flushPromises()
      await wrapper.get('#analytics-tab-people').trigger('click')
      await wrapper.get('[aria-label="搜索人员明细"]').setValue('张三')
      await wrapper.get('[aria-label="搜索人员明细"]').trigger('keyup.enter');await flushPromises()
      expect(api.report).toHaveBeenLastCalledWith(expect.objectContaining({search:'张三'}),expect.anything())
      await wrapper.findAll('button').find(button=>button.text()==='导出报表')!.trigger('click');await flushPromises()
      expect(api.report).toHaveBeenLastCalledWith(expect.objectContaining({search:'',page:1,export:true,versions:JSON.stringify({'2026-11-01':8})}),expect.anything())
      expect(api.export).toHaveBeenCalledOnce()
    }finally{wrapper.unmount()}
  })
  it('updates directory actions from task polling without querying the full report again',async()=>{
    const wrapper=mountView()
    try{
      await flushPromises()
      await wrapper.get('#analytics-tab-maintenance').trigger('click')
      const count=api.report.mock.calls.length
      const control=wrapper.findComponent({name:'AnalyticsTaskControl'})
      control.vm.$emit('loaded',null)
      await flushPromises()
      expect(wrapper.get('[data-enabled]').attributes('data-enabled')).toBe('false')
      control.vm.$emit('loaded',{control:{enabled:true,version:2},state:'idle',running:0})
      await flushPromises()
      expect(wrapper.get('[data-enabled]').attributes('data-enabled')).toBe('true')
      expect(api.report).toHaveBeenCalledTimes(count)
    }finally{wrapper.unmount()}
  })
  it('ignores an older time-range response after a newer selection has loaded',async()=>{
    const wrapper=mountView()
    try{
      await flushPromises()
      const base=await api.report.mock.results[0]!.value as AnalyticsReport
      let finishOld!: (data:AnalyticsReport)=>void
      let finishNew!: (data:AnalyticsReport)=>void
      api.report.mockImplementationOnce(()=>new Promise(resolve=>{finishOld=resolve}))
      api.report.mockImplementationOnce(()=>new Promise(resolve=>{finishNew=resolve}))
      const modes=wrapper.get('[aria-label="时间范围模式"]').findAll('button')
      await modes[0]!.trigger('click')
      await modes[2]!.trigger('click')
      finishNew({...base,summary:{...values,tokens:'2000000'},period:{...base.period,mode:'months',grain:'month'}})
      await flushPromises()
      expect(wrapper.findAll('.analytics-stat')[0]!.text()).toContain('2M')
      finishOld({...base,period:{...base.period,mode:'year',grain:'month'}})
      await flushPromises()
      expect(wrapper.findAll('.analytics-stat')[0]!.text()).toContain('2M')
    }finally{wrapper.unmount()}
  })
})
