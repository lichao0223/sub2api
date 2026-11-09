import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AnalyticsProgress from '../AnalyticsProgress.vue'
import type { AnalyticsTaskStatus } from '@/api/admin/dataAnalytics'

const api=vi.hoisted(()=>({status:vi.fn()}))
vi.mock('@/api/admin/dataAnalytics',()=>({analyticsTasksAPI:api}))
function status():AnalyticsTaskStatus {
  return {
    control:{enabled:true,version:1,updated_at:null,updated_by:0},state:'running',running:1,last_success_at:null,
    last_seen_at:'2026-11-09T02:00:00Z',analytics_day:'2026-11-01',analytics_started_at:'2026-11-09T01:59:00Z',
    progress:{directory_ready:true,seeded:true,total_days:40,completed_days:10,backlog_days:30,failed_days:1,
      pending_requests:100,retry_at:'2026-11-09T02:05:00Z',last_success_at:'2026-11-09T01:58:00Z',last_error:''},
  }
}
const wrappers:ReturnType<typeof mount>[]=[]
function render(){const wrapper=mount(AnalyticsProgress);wrappers.push(wrapper);return wrapper}
beforeEach(()=>{vi.useFakeTimers();vi.setSystemTime(new Date('2026-11-09T02:00:00Z'));api.status.mockReset();api.status.mockResolvedValue(status())})
afterEach(()=>{wrappers.forEach(wrapper=>wrapper.unmount());wrappers.length=0;vi.useRealTimers()})
describe('overall analytics progress',()=>{
  it('shows global completion, current day, elapsed time, heartbeat and separate missing prices',async()=>{
    const wrapper=render();await flushPromises()
    expect(wrapper.text()).toContain('10 / 40 天')
    expect(wrapper.text()).toContain('25%')
    expect(wrapper.text()).toContain('正在核算')
    expect(wrapper.text()).toContain('2026-11-01')
    expect(wrapper.text()).toContain('本批已用 1 分 0 秒')
    expect(wrapper.text()).toContain('实例心跳')
    expect(wrapper.text()).toContain('价格待补：100 条请求')
    expect(wrapper.get('progress').attributes()).toMatchObject({value:'10',max:'40'})
    const updated=status();updated.progress!.completed_days=11;updated.progress!.backlog_days=29
    api.status.mockResolvedValue(updated)
    await vi.advanceTimersByTimeAsync(10_000);await flushPromises()
    expect(wrapper.text()).toContain('11 / 40 天')
    expect(wrapper.emitted('loaded')?.at(-1)).toEqual([updated])
  })
  it('shows pause and retry states without describing a known retry wait as stuck',async()=>{
    const paused=status();paused.control.enabled=false;paused.state='paused';paused.analytics_day=''
    api.status.mockResolvedValue(paused)
    const wrapper=render();await flushPromises()
    expect(wrapper.text()).toContain('已暂停')
    const retry=status();retry.state='idle';retry.running=0;retry.analytics_day='';retry.analytics_started_at=null
    retry.progress!.failed_days=30;retry.progress!.last_success_at='2026-11-09T01:00:00Z'
    api.status.mockResolvedValue(retry)
    await vi.advanceTimersByTimeAsync(10_000);await flushPromises()
    expect(wrapper.text()).toContain('等待重试')
    expect(wrapper.text()).toContain('重试时间')
    expect(wrapper.text()).not.toContain('超过 15 分钟')
  })
  it('distinguishes a lost heartbeat and an overdue batch from normal work',async()=>{
    const lost=status();lost.state='unknown'
    api.status.mockResolvedValue(lost)
    const wrapper=render();await flushPromises()
    expect(wrapper.text()).toContain('心跳异常')
    expect(wrapper.text()).toContain('无法确认任务是否仍在运行')
    const slow=status();slow.analytics_started_at='2026-11-09T01:50:00Z'
    api.status.mockResolvedValue(slow)
    await vi.advanceTimersByTimeAsync(10_000);await flushPromises()
    expect(wrapper.text()).toContain('本批长时间未结束')
  })
  it('keeps the last progress on poll failure and clears the warning on recovery',async()=>{
    const wrapper=render();await flushPromises()
    api.status.mockRejectedValue(new Error('offline'))
    await vi.advanceTimersByTimeAsync(10_000);await flushPromises()
    expect(wrapper.text()).toContain('状态未知')
    expect(wrapper.text()).toContain('10 / 40 天')
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.emitted('loaded')?.at(-1)).toEqual([null])
    api.status.mockResolvedValue(status())
    await wrapper.findAll('button').find(button=>button.text()==='刷新状态')!.trigger('click');await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount();const calls=api.status.mock.calls.length
    await vi.advanceTimersByTimeAsync(30_000)
    expect(api.status).toHaveBeenCalledTimes(calls)
  })
})
