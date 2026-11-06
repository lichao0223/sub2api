import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PersonDrawer from '../PersonDrawer.vue'
import type { AnalyticsQuery, AnalyticsReport, AnalyticsRow, AnalyticsValues } from '@/api/admin/dataAnalytics'

const api=vi.hoisted(()=>({report:vi.fn()}))
vi.mock('@/api/admin/dataAnalytics',()=>({dataAnalyticsAPI:api}))
const values:AnalyticsValues={input:'1000000',output:'0',cache_write:'1000',cache_read:'2000',tokens:'1003000',requests:'1',metered:'0',fixed:'0',cost:'0',pending:'0',excluded_tokens:'1003000',input_cost:'0',output_cost:'0',write_cost:'0',read_cost:'0',image_cost:'0',request_cost:'0'}
const person={...values,user_id:7,name:'张三',person_id:'p',org_ids:['company'],org_names:['公司'],model:'免费模型',mode:'excluded',source_id:0,source:'不计成本',issue:'',attribution:1} satisfies AnalyticsRow
const query:AnalyticsQuery={year:2026,mode:'months',months:'1,3',org_id:'company'}
const versions={'2026-01-01':2,'2026-03-01':4}
afterEach(()=>vi.clearAllMocks())
describe('personal usage drawer',()=>{
  it('pins the selected report, includes excluded models and loads a selected model month on demand',async()=>{
    api.report.mockImplementation(async(q:AnalyticsQuery)=>({
      period:{year:2026,months:q.mode==='month'?[Number(q.months)]:[1,3],keys:['2026-01-01','2026-03-01']},
      summary:values,models:[person],fixed_shares:[],versions,coverage:{provisional:false},
      person_months:[{...values,month:'2026-01-01',generation:2},{...values,month:'2026-02-01',generation:0},{...values,month:'2026-03-01',generation:4}],
    } as unknown as AnalyticsReport))
    const wrapper=mount(PersonDrawer,{props:{person,query,versions,rangeLabel:'2026 年 1、3 月'},global:{stubs:{
      BaseDialog:{props:['show','placement'],template:'<section v-if="show" :data-placement="placement"><slot/></section>'},
      Select:{props:['modelValue','options','ariaLabel'],emits:['update:modelValue','change'],template:`<select :aria-label="ariaLabel" :value="modelValue" @change="e=>{$emit('update:modelValue',e.target.value);$emit('change')}"><option v-for="o in options" :value="o.value" :key="o.value">{{o.label}}</option></select>`},
    }}})
    try{
      await flushPromises()
      expect(wrapper.get('section').attributes('data-placement')).toBe('right')
      expect(api.report).toHaveBeenLastCalledWith(expect.objectContaining({user_id:7,search:'',versions:JSON.stringify(versions)}),expect.anything())
      expect(wrapper.text()).toContain('免费模型')
      expect(wrapper.text()).toContain('不计成本')
      expect(wrapper.text()).toContain('缓存写入')
      expect(wrapper.text()).toContain('缓存读取')
      expect(wrapper.text()).toContain('尚未发布')
      expect(wrapper.findAll('.analytics-selected-month')).toHaveLength(2)
      await wrapper.get('[aria-label="个人模型月份"]').setValue('3');await flushPromises()
      expect(api.report).toHaveBeenLastCalledWith(expect.objectContaining({user_id:7,mode:'month',months:'3',versions:JSON.stringify({'2026-03-01':4})}),expect.anything())
      const count=api.report.mock.calls.length
      await wrapper.get('[aria-label="个人模型月份"]').setValue('');await flushPromises()
      expect(api.report).toHaveBeenCalledTimes(count)
    }finally{wrapper.unmount()}
  })
  it('cancels a month request when closed and resets loading and model pagination when reopened',async()=>{
    const data={
      period:{year:2026,months:[1,3],keys:['2026-01-01','2026-03-01']},
      summary:values,models:Array.from({length:41},(_,index)=>({...person,model:`model-${index+1}`})),
      fixed_shares:[],versions,coverage:{provisional:false},person_months:[],
    } as unknown as AnalyticsReport
    api.report.mockResolvedValue(data)
    const wrapper=mount(PersonDrawer,{props:{person,query,versions,rangeLabel:'2026 年 1、3 月'},global:{stubs:{
      BaseDialog:{props:['show'],template:'<section v-if="show"><slot/></section>'},
      Select:{name:'Select',props:['modelValue','options'],emits:['update:modelValue','change'],template:'<div />'},
      Pagination:{name:'Pagination',props:['page','total'],emits:['update:page'],template:'<button @click="$emit(\'update:page\',3)">末页模型</button>'},
    }}})
    try{
      await flushPromises()
      expect(wrapper.text()).not.toContain('model-41')
      await wrapper.findComponent({name:'Pagination'}).trigger('click')
      expect(wrapper.text()).toContain('model-41')
      api.report.mockImplementationOnce(()=>new Promise(()=>{}))
      const select=wrapper.findComponent({name:'Select'})
      select.vm.$emit('update:modelValue','3')
      select.vm.$emit('change')
      await flushPromises()
      const signal=api.report.mock.calls.at(-1)![1] as AbortSignal
      expect(wrapper.text()).toContain('正在加载所选月份')
      await wrapper.setProps({person:undefined})
      expect(signal.aborted).toBe(true)
      await wrapper.setProps({person:{...person}})
      await flushPromises()
      expect(wrapper.text()).not.toContain('正在加载所选月份')
      expect(wrapper.findComponent({name:'Pagination'}).props('page')).toBe(1)
      expect(wrapper.text()).toContain('model-1')
      expect(wrapper.text()).not.toContain('model-41')
    }finally{wrapper.unmount()}
  })
})
