<template>
  <section class="analytics-panel analytics-progress" aria-label="总体核算进度">
    <div class="analytics-panel-heading">
      <h2>总体核算进度 <span class="analytics-pill" role="status" :class="{warn: unavailable || status?.state==='unknown' || progress?.failed_days}">{{ stateLabel }}</span></h2>
      <div class="analytics-toolbar"><span>每 10 秒更新</span><button type="button" class="analytics-text-button" :disabled="loading" @click="refresh">刷新状态</button><a class="analytics-text-button" href="/admin/settings">启停设置</a></div>
    </div>
    <template v-if="progress">
      <div class="analytics-progress-summary"><strong>{{ progress.completed_days }} / {{ progress.total_days }} 天 <span>（{{ percent }}%）</span></strong><span>全部历史范围 · 待处理 {{ progress.backlog_days }} 天，其中失败 {{ progress.failed_days }} 天</span></div>
      <progress :value="progress.completed_days" :max="Math.max(progress.total_days,1)" aria-label="全部历史核算完成天数"></progress>
      <div class="analytics-progress-details">
        <span>当前核算：{{ currentWork }}<template v-if="!unavailable && status?.state==='running' && status.analytics_started_at"> · 本批已用 {{ elapsed(status.analytics_started_at) }}</template></span>
        <span>最近发布：{{ dateTime(progress.last_success_at) }}</span>
        <span>实例心跳：{{ dateTime(status?.last_seen_at) }}</span>
        <span v-if="progress.failed_days && progress.retry_at">重试时间：{{ dateTime(progress.retry_at) }}<template v-if="new Date(progress.retry_at).getTime()<=checkedAt">（等待下一轮调度）</template></span>
      </div>
      <p v-if="!progress.directory_ready" class="analytics-muted">请先在「数据维护」应用组织与人员目录。</p>
      <p v-else-if="!status?.control.enabled" class="analytics-muted">核算已关闭；暂停中的任务会回滚尚未发布的结果，已发布数据继续保留。</p>
      <p v-else-if="status.state==='unknown'" class="analytics-progress-warning" role="status">未收到有效实例心跳，无法确认任务是否仍在运行，请检查服务状态。</p>
      <p v-else-if="slow" class="analytics-progress-warning" role="status">{{ currentWork }}：本批长时间未结束，可能存在锁等待或数据库负载过高；请检查服务日志。</p>
      <p v-else-if="waitingTooLong" class="analytics-progress-warning" role="status">仍有积压，超过 15 分钟未发布新结果，请检查后台核算日志。</p>
      <p v-if="progress.last_error" class="analytics-progress-warning">最近一次异常：{{ progress.last_error }}</p>
      <p v-if="progress.pending_requests" class="analytics-muted">价格待补：{{ progress.pending_requests.toLocaleString() }} 条请求。请检查成本方案、模型价格和生效时间；该数量不计入任务完成进度。</p>
    </template>
    <p v-else class="analytics-muted">{{ loading ? '正在读取后台核算状态…' : '当前服务尚未提供总体进度，请确认后端已更新。' }}</p>
    <p v-if="unavailable" class="analytics-progress-warning" role="alert">状态更新失败，无法确认当前进度；下方保留上次成功读取的数据。请检查连接或点击「刷新状态」。</p>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import { analyticsTasksAPI, type AnalyticsTaskStatus } from '@/api/admin/dataAnalytics'

const emit = defineEmits<{ loaded: [status: AnalyticsTaskStatus | null] }>()
const status = shallowRef<AnalyticsTaskStatus | null>(null)
const loading = ref(false), unavailable = ref(false), checkedAt = ref(Date.now())
const progress = computed(()=>status.value?.progress)
const percent = computed(()=>progress.value?.total_days ? Math.min(100,Math.floor(progress.value.completed_days/progress.value.total_days*100)) : 0)
const waitingRetry = computed(()=>!!progress.value?.backlog_days && progress.value.failed_days===progress.value.backlog_days && !!progress.value.retry_at && new Date(progress.value.retry_at).getTime()>checkedAt.value)
const slow = computed(()=>!unavailable.value && status.value?.state==='running' && !!status.value.analytics_started_at && checkedAt.value-new Date(status.value.analytics_started_at).getTime()>180_000)
const waitingTooLong = computed(()=>!!progress.value?.backlog_days && !waitingRetry.value && !!progress.value.last_success_at && checkedAt.value-new Date(progress.value.last_success_at).getTime()>900_000)
const stateLabel = computed(()=>{
  if(unavailable.value || !status.value)return '状态未知'
  if(status.value.state==='pausing')return '正在暂停'
  if(!status.value.control.enabled)return '已暂停'
  if(status.value.state==='unknown')return '心跳异常'
  if(!progress.value)return '进度不可用'
  if(!progress.value.directory_ready)return '等待目录'
  if(!progress.value.seeded)return '初始化中'
  if(status.value.analytics_day)return '正在核算'
  if(status.value.analytics_stage==='prepare')return '准备核算'
  if(waitingRetry.value)return '等待重试'
  return progress.value.backlog_days ? '等待调度' : '已追平，定期更新'
})
const currentWork = computed(()=>{
  if(unavailable.value || status.value?.state!=='running' || !status.value.control.enabled)return stateLabel.value
  const {analytics_day:day, analytics_stage:stage}=status.value
  if(stage==='prepare')return '准备队列、同步目录'
  if(day)return `${day}${stage==='publish' ? ' · 发布统计' : ' · 汇总日用量'}`
  return stateLabel.value
})
function dateTime(value?: string | null){return value ? new Date(value).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',hour12:false}) : '—'}
function elapsed(value: string){const seconds=Math.max(0,Math.floor((checkedAt.value-new Date(value).getTime())/1000));return `${Math.floor(seconds/60)} 分 ${seconds%60} 秒`}
let disposed=false, timer:ReturnType<typeof setInterval>|undefined, controller:AbortController|undefined
async function refresh(){
  if(disposed || loading.value)return
  loading.value=true;controller=new AbortController()
  try{
    const result=await analyticsTasksAPI.status(controller.signal)
    if(disposed)return
    status.value=result;unavailable.value=false;checkedAt.value=Date.now();emit('loaded',result)
  }catch{
    if(!disposed){unavailable.value=true;emit('loaded',null)}
  }finally{loading.value=false}
}
onMounted(()=>{void refresh();timer=setInterval(()=>{checkedAt.value=Date.now();if(!document.hidden)void refresh()},10_000)})
onBeforeUnmount(()=>{disposed=true;controller?.abort();if(timer)clearInterval(timer)})
</script>
