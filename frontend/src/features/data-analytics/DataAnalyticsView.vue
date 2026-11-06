<template>
  <AppLayout>
    <div class="space-y-5" :aria-busy="loading">
      <AnalyticsTaskControl />
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div><h1 class="text-2xl font-semibold">数据分析</h1><p class="mt-1 text-sm text-gray-500">按真实组织与人员分析 Token 使用量及人民币采购成本</p></div>
        <div class="flex flex-wrap gap-2"><button class="btn btn-secondary" @click="directoryOpen=true">组织与人员维护</button><button class="btn btn-secondary" :disabled="loading" @click="load(true)">刷新</button><button class="btn btn-primary" :disabled="!report || loading || exporting || !enabled" @click="download">{{ exporting ? '正在准备…' : '导出 Excel' }}</button></div>
      </div>
      <div class="card flex flex-wrap items-end gap-4 p-4">
        <label class="w-32 text-sm">年份<Select v-model="year" class="mt-1" :options="yearOptions" @change="changePeriod" /></label>
        <label class="w-36 text-sm">时间范围<Select v-model="mode" class="mt-1" :options="periodOptions" @change="changePeriod" /></label>
        <label v-if="mode==='month'" class="w-32 text-sm">月份<Select v-model="month" class="mt-1" :options="monthOptions" @change="changePeriod" /></label>
        <div v-if="mode==='months'" class="flex flex-wrap gap-2" aria-label="选择月份"><label v-for="m in monthOptions" :key="m.value" class="flex items-center gap-1 rounded border px-2 py-2 text-sm dark:border-dark-600"><input v-model="months" type="checkbox" :value="m.value" @change="changePeriod">{{ m.label }}</label></div>
        <label class="min-w-52 flex-1 text-sm">部门范围<Select v-model="org" class="mt-1" :options="orgOptions" searchable @change="resetAndLoad" /></label>
      </div>
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-4 text-red-700 dark:bg-red-900/20">{{ error }}</p>
      <template v-if="report">
        <div v-if="!report.coverage.directory_ready" class="rounded-lg bg-primary-50 p-4 text-sm text-primary-700 dark:bg-primary-900/20">请先在「组织与人员维护」预检并应用两份 SQL，再开始首轮统计。启用前已经清理的原始历史无法恢复。</div>
        <div class="flex flex-wrap justify-between gap-2 text-xs text-gray-500"><span>统计范围：{{ rangeLabel }} · 趋势{{ mode==='month' ? '按天' : '按月' }} · 北京时间</span><span>最近核算：{{ report.coverage.last_updated ? new Date(report.coverage.last_updated).toLocaleString() : '尚未核算' }}</span></div>
        <div v-if="!report.coverage.complete || report.coverage.last_error" class="rounded-lg bg-amber-50 p-4 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-300">
          已完成 {{ report.coverage.completed_days }} / {{ report.coverage.expected_days }} 天；后台待处理 {{ report.coverage.backlog_days }} 天，失败 {{ report.coverage.failed_days }} 天；待核算价格 {{ report.coverage.pending_requests }} 条。未完成期间展示已发布结果。
          <p v-if="report.coverage.history_gap" class="mt-1">可用原始请求起于 {{ report.coverage.raw_history_start?.slice(0,10) }}，此前已清理的请求无法恢复，历史用量可能不完整。</p>
          <p v-for="issue in report.coverage.issues" :key="issue.day" class="mt-1">{{ issue.day }}：{{ issue.message }}</p>
          <p v-if="report.coverage.last_error" class="mt-1">{{ report.coverage.last_error }}</p>
        </div>
        <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-5"><div v-for="card in cards" :key="card.label" class="card p-5"><div class="text-sm text-gray-500">{{ card.label }}</div><div class="mt-2 text-2xl font-semibold tabular-nums">{{ card.value }}</div><p class="mt-2 text-xs text-gray-400">{{ card.note }}</p></div></div>
        <div class="grid items-stretch gap-5 xl:grid-cols-[1.5fr_1fr]">
          <section class="card flex min-w-0 flex-col p-5"><h2 class="font-semibold">Token 使用趋势 <span class="text-sm font-normal text-gray-400">· {{ rangeLabel }}</span></h2><div class="relative mt-4 min-h-72 flex-1"><canvas ref="canvas" role="img" :aria-label="`Token使用趋势，${rangeLabel}`"></canvas></div></section>
          <section class="card p-5"><h2 class="font-semibold">成本构成</h2><div class="mt-5 space-y-5"><div v-for="item in costCards" :key="item.label" class="flex items-center justify-between gap-4 border-b border-gray-100 pb-4 dark:border-dark-700"><span class="text-sm text-gray-500">{{ item.label }}</span><span class="font-semibold tabular-nums">{{ moneyText(item.value) }}</span></div></div><p class="mt-4 text-xs leading-6 text-gray-500">固定成本：当月实际用过该订阅实例的人均摊，未使用者不分摊。无人使用计公共成本。{{ report.coverage.provisional ? '本月随使用人数与已发生成本变化重新分摊。' : '' }}<br>不计成本模型仍计入全部 Token。</p></section>
        </div>
        <section class="card overflow-hidden"><div class="flex flex-wrap items-center justify-between gap-3 p-5"><h2 class="font-semibold">各部门使用情况</h2><label class="flex items-center gap-2 text-sm">显示层级<Select v-model="level" class="w-28" :options="levelOptions" /></label></div><div class="overflow-x-auto"><table class="table analytics-table"><thead><tr><th>部门（含下级）</th><th>人数</th><th>Token 总数</th><th>输入</th><th>输出</th><th>缓存写入</th><th>缓存读取</th><th>按量成本</th><th>固定成本</th><th>总成本</th></tr></thead><tbody><tr v-for="d in departments" :key="d.id"><td><button class="text-left text-primary-600" :style="{paddingLeft: `${(d.level-1)*16}px`}" @click="org=d.id;resetAndLoad()">{{ d.name }}</button></td><td>{{ d.users }}</td><td>{{ tokenText(d.tokens) }}</td><td>{{ tokenText(d.input) }}</td><td>{{ tokenText(d.output) }}</td><td>{{ tokenText(d.cache_write) }}</td><td>{{ tokenText(d.cache_read) }}</td><td>{{ moneyText(d.metered) }}</td><td>{{ moneyText(d.fixed) }}</td><td class="font-semibold">{{ moneyText(d.cost) }}</td></tr><tr v-if="!departments.length"><td colspan="10" class="py-8 text-center text-gray-400">当前范围暂无部门使用数据</td></tr></tbody></table></div><p class="px-5 py-3 text-xs text-gray-400">仅显示第一级至第三级；更深组织归入第三级。上下级各行包含下级数据，不应逐行相加。</p></section>
        <section class="card overflow-hidden"><div class="flex flex-wrap items-center justify-between gap-3 p-5"><h2 class="font-semibold">个人使用情况</h2><div class="flex flex-wrap gap-2"><input v-model="search" class="input" placeholder="搜索姓名（筛选个人表）" @keyup.enter="resetAndLoad"><Select v-model="sort" class="w-36" :options="sortOptions" @change="resetAndLoad" /><button class="btn btn-secondary" :disabled="loading" @click="resetAndLoad">查询</button></div></div><div class="overflow-x-auto"><table class="table analytics-table"><thead><tr><th>人员 / 部门</th><th>Token 总数</th><th>输入</th><th>输出</th><th>缓存写入</th><th>缓存读取</th><th>按量成本</th><th>固定成本</th><th>总成本</th><th>明细</th></tr></thead><tbody><tr v-for="p in report.people" :key="p.user_id"><td><span class="font-medium">{{ p.name }}</span><span class="mt-1 block max-w-72 text-xs text-gray-400">{{ p.org_names.slice(0,3).join(' / ') || '待确认归属' }}</span></td><td>{{ tokenText(p.tokens) }}</td><td>{{ tokenText(p.input) }}</td><td>{{ tokenText(p.output) }}</td><td>{{ tokenText(p.cache_write) }}</td><td>{{ tokenText(p.cache_read) }}</td><td>{{ moneyText(p.metered) }}</td><td>{{ moneyText(p.fixed) }}</td><td class="font-semibold">{{ moneyText(p.cost) }}</td><td><button class="text-primary-600" @click="openPerson(p)">模型与成本</button></td></tr><tr v-if="!report.people.length"><td colspan="10" class="py-8 text-center text-gray-400">当前范围暂无人员使用数据</td></tr></tbody></table></div><Pagination :page="page" :page-size="pageSize" :total="report.people_total" @update:page="page=$event;load()" @update:pageSize="pageSize=$event;resetAndLoad()" /></section>
        <details class="card p-5 text-sm"><summary class="cursor-pointer font-medium">统计口径与补算</summary><p class="mt-3 leading-7 text-gray-500">{{ report.coverage.history_basis }}。按量成本取成本方案在请求发生时的价格；不计成本模型只计 Token。日模型明细保留13个月，月模型统计、部门每日趋势保留60个月。补算必须仍有完整原始请求，每次最多31天。</p><div class="mt-3 flex flex-wrap items-end gap-3"><label>开始日期<input v-model="rebuildStart" class="input mt-1 block" type="date"></label><label>结束日期<input v-model="rebuildEnd" class="input mt-1 block" type="date"></label><button class="btn btn-secondary" :disabled="!enabled || rebuilding" @click="rebuild">提交补算任务</button></div></details>
      </template>
      <p v-else-if="loading" class="p-10 text-center text-gray-400" role="status">正在加载分析数据…</p>
    </div>
    <DirectoryDialog :show="directoryOpen" :enabled="enabled" @close="directoryOpen=false" @changed="directoryChanged" />
    <BaseDialog :show="!!selectedPerson" :title="`${selectedPerson?.name || ''} · 模型使用与成本组成`" width="extra-wide" @close="closePerson">
      <div v-if="personLoading" class="p-8 text-center" role="status">加载中…</div>
      <div v-else-if="personReport" class="space-y-4"><div class="flex flex-wrap gap-5 text-sm"><span>{{ rangeLabel }}</span><b>Token {{ tokenText(personReport.summary.tokens) }}</b><b>总成本 {{ moneyText(personReport.summary.cost) }}</b><span>按量 {{ moneyText(personReport.summary.metered) }}</span><span>固定 {{ moneyText(personReport.summary.fixed) }}</span></div>
        <p class="text-xs text-gray-500">包含全部模型。每行按模型、采购来源和历史组织分别展示；同一模型可能有多行。待核算价格不会被当作零成本已完成。</p>
        <div class="max-h-[45vh] overflow-auto"><table class="table analytics-table"><thead><tr><th>模型 / 采购来源</th><th>组织归属</th><th>Token 总数</th><th>输入</th><th>输出</th><th>缓存写入</th><th>缓存读取</th><th>输入成本</th><th>输出成本</th><th>缓存写入成本</th><th>缓存读取成本</th><th>图片成本</th><th>请求成本</th><th>固定分摊</th><th>总成本</th></tr></thead><tbody><tr v-for="(r,i) in personReport.models" :key="i"><td class="min-w-48">{{ r.model }}<span class="mt-1 block text-xs text-gray-400">{{ r.source }} · {{ modeLabel(r.mode) }}</span><span v-if="r.issue" class="block text-xs text-amber-600">{{ issueLabel(r.issue) }}</span></td><td class="min-w-40 text-xs">{{ r.org_names.slice(0,3).join(' / ') || '待确认归属' }}</td><td>{{ tokenText(r.tokens) }}</td><td>{{ tokenText(r.input) }}</td><td>{{ tokenText(r.output) }}</td><td>{{ tokenText(r.cache_write) }}</td><td>{{ tokenText(r.cache_read) }}</td><td>{{ moneyText(r.input_cost) }}</td><td>{{ moneyText(r.output_cost) }}</td><td>{{ moneyText(r.write_cost) }}</td><td>{{ moneyText(r.read_cost) }}</td><td>{{ moneyText(r.image_cost) }}</td><td>{{ moneyText(r.request_cost) }}</td><td>{{ moneyText(r.fixed) }}</td><td>{{ moneyText(r.cost) }}</td></tr></tbody></table></div>
        <h3 class="font-semibold">固定订阅分摊依据</h3><p class="text-xs text-gray-500">下方为个人整月、所有部门的唯一份额；上方模型费用只包含当前部门范围分配到的部分。</p><div class="overflow-x-auto"><table class="table analytics-table"><thead><tr><th>月份 / 订阅实例</th><th>实例已发生成本</th><th>当月使用人数</th><th>个人整月份额（所有部门）</th></tr></thead><tbody><tr v-for="s in personReport.fixed_shares" :key="`${s.month}/${s.unit_id}`"><td>{{ s.month.slice(0,7) }} · {{ s.name }}</td><td>{{ moneyText(s.total) }}</td><td>{{ s.participants }}</td><td>{{ moneyText(s.amount) }}</td></tr><tr v-if="!personReport.fixed_shares.length"><td colspan="4" class="text-center text-gray-400">没有固定订阅使用</td></tr></tbody></table></div>
      </div><p v-else class="text-gray-400">明细未加载，请重新打开。</p>
      <template #footer><button class="btn btn-secondary" @click="closePerson">关闭</button></template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import { Chart } from 'chart.js/auto'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Pagination from '@/components/common/Pagination.vue'
import AnalyticsTaskControl from '@/components/admin/settings/AnalyticsTaskControl.vue'
import DirectoryDialog from './DirectoryDialog.vue'
import { dataAnalyticsAPI, type AnalyticsDirectoryState, type AnalyticsQuery, type AnalyticsReport, type AnalyticsRow } from '@/api/admin/dataAnalytics'
import { useAppStore } from '@/stores/app'
import { moneyText, tokenText } from './format'
import { exportAnalytics } from './export'
const app = useAppStore()
const now = new Date(new Date().toLocaleString('en-US', {timeZone: 'Asia/Shanghai'}))
const year=ref(now.getFullYear()), mode=ref('month'), month=ref(now.getMonth()+1), months=ref([now.getMonth()+1]), org=ref('')
const page=ref(1),pageSize=ref(20),sort=ref('tokens'),search=ref(''),level=ref(3),directoryOpen=ref(false)
const report=shallowRef<AnalyticsReport>(),directory=shallowRef<AnalyticsDirectoryState>(),loading=ref(false),error=ref(''),exporting=ref(false)
const selectedPerson=shallowRef<AnalyticsRow>(),personReport=shallowRef<AnalyticsReport>(),personLoading=ref(false)
const rebuildStart=ref(''),rebuildEnd=ref(''),rebuilding=ref(false),canvas=ref<HTMLCanvasElement>()
const appliedQuery=ref<AnalyticsQuery>()
let chart: Chart | undefined, abort: AbortController | undefined, detailAbort: AbortController | undefined, timer: ReturnType<typeof setInterval> | undefined, sequence=0, disposed=false
const enabled=computed(()=>report.value?.task_status?.control.enabled ?? false)
const yearOptions=Array.from({length:6},(_,i)=>({value:now.getFullYear()-i,label:`${now.getFullYear()-i}年`}))
const periodOptions=[{value:'year',label:'全年'},{value:'month',label:'单月'},{value:'months',label:'多个月份'}]
const cutoff=new Date(now.getFullYear(),now.getMonth()-59,1)
const monthOptions=computed(()=>Array.from({length:year.value===now.getFullYear()?now.getMonth()+1:12},(_,i)=>({value:i+1,label:`${i+1}月`})).filter(m=>new Date(year.value,m.value-1,1)>=cutoff))
const levelOptions=[{value:1,label:'第一级'},{value:2,label:'至第二级'},{value:3,label:'至第三级'}]
const sortOptions=[{value:'tokens',label:'Token 从高到低'},{value:'cost',label:'成本从高到低'}]
const orgOptions=computed(()=>{
  const options=[{value:'',label:'全部部门'},{value:'0',label:'待确认归属'},...(directory.value?.directory.orgs || []).filter(o=>o.level<=3).map(o=>({value:o.id,label:o.names.join(' / ')}))]
  const ids=new Set(options.map(o=>o.value))
  for(const d of report.value?.departments || [])if(!ids.has(d.id)){options.push({value:d.id,label:`历史组织 · ${d.name}`});ids.add(d.id)}
  if(org.value&&!ids.has(org.value))options.push({value:org.value,label:'已选历史组织'})
  return options
})
const departments=computed(()=>{
  const rows=(report.value?.departments || []).filter(d=>d.level<=level.value)
  const result: typeof rows=[],children=new Map<string,typeof rows>(),ids=new Set(rows.map(d=>d.id)),visited=new Set<string>()
  for(const d of rows){const parent=ids.has(d.parent)?d.parent:'';const siblings=children.get(parent)||[];siblings.push(d);children.set(parent,siblings)}
  const walk=(parent:string)=>{for(const d of children.get(parent)||[]){if(visited.has(d.id))continue;visited.add(d.id);result.push(d);walk(d.id)}}
  walk('');return result
})
const rangeLabel=computed(()=>report.value ? `${report.value.period.year}年 ${report.value.period.months.join('、')}月` : '')
const cards=computed(()=>report.value ? [
  {label:'Token 总数',value:tokenText(report.value.summary.tokens),note:'含输入、输出及缓存'},
  {label:'使用人数',value:String(report.value.users),note:'按规范用户去重'},
  {label:'采购总成本',value:moneyText(report.value.summary.cost),note:'已核算成本（CNY）'},
  {label:'按量成本',value:moneyText(report.value.summary.metered),note:'按请求发生时成本价格'},
  {label:'固定成本',value:moneyText(report.value.summary.fixed),note:'含公共未分摊成本'},
]:[])
const costCards=computed(()=>report.value ? [{label:'按量采购成本',value:report.value.summary.metered},{label:'固定订阅成本（含公共）',value:report.value.summary.fixed},{label:'其中：无人使用的公共成本',value:report.value.public_cost}] : [])
const modeLabels:Record<string,string>={fixed:'固定成本',metered:'按量成本',excluded:'不计成本'}
const modeLabel=(v:string)=>modeLabels[v] || v
const issueLabels:Record<string,string>={missing_account_config:'待配置账号成本',missing_subscription:'订阅实例不存在或不在有效期',missing_subscription_version:'缺少订阅价格版本',missing_plan_version:'缺少生效价格版本',missing_model_price:'缺少模型成本价格'}
const issueLabel=(v:string)=>issueLabels[v] || v
function query():AnalyticsQuery {return {year:year.value,mode:mode.value,months:(mode.value==='month'?[month.value]:months.value).join(','),org_id:org.value,search:search.value.trim(),page:page.value,page_size:pageSize.value,sort:sort.value}}
function changePeriod(){const valid=monthOptions.value.map(m=>m.value);if(!valid.includes(month.value))month.value=valid[valid.length-1]!;months.value=months.value.filter(m=>valid.includes(m));if(!months.value.length)months.value=[month.value];resetAndLoad()}
function resetAndLoad(){page.value=1;closePerson();chart?.destroy();chart=undefined;report.value=undefined;void load()}
async function load(refreshDirectory=false) {
  const current=++sequence;abort?.abort();abort=new AbortController();loading.value=true;error.value=''
  if(mode.value==='months'&&!months.value.length){error.value='请至少选择一个月份';loading.value=false;return}
  const params=query()
  try { const result=await dataAnalyticsAPI.report(params,abort.signal);if(disposed || current!==sequence)return;report.value=result;appliedQuery.value=params;if(refreshDirectory || !directory.value)directory.value=await dataAnalyticsAPI.directory();await nextTick();if(current===sequence)draw() }
  catch(e){if(current===sequence && !abort.signal.aborted)error.value=e instanceof Error?e.message:'报表加载失败'}
  finally {if(current===sequence)loading.value=false}
}
function draw(){if(!canvas.value || !report.value)return;chart?.destroy();const points=report.value.trend;chart=new Chart(canvas.value,{type:'line',data:{labels:points.map(p=>p.bucket),datasets:[{label:'Token',data:points.map(p=>Number(p.tokens)),borderColor:'#6366f1',backgroundColor:'rgba(99,102,241,.10)',fill:true,tension:.25,pointRadius:2}]},options:{responsive:true,maintainAspectRatio:false,plugins:{legend:{display:false},tooltip:{callbacks:{label:c=>`Token ${tokenText(points[c.dataIndex]?.tokens)}`}}},scales:{y:{beginAtZero:true,ticks:{callback:v=>tokenText(Math.trunc(Number(v)).toLocaleString('en-US',{useGrouping:false,maximumFractionDigits:0}))}},x:{grid:{display:false}}}}})}
async function directoryChanged(){await load(true)}
async function openPerson(p:AnalyticsRow){closePerson();selectedPerson.value=p;personLoading.value=true;const controller=new AbortController();detailAbort=controller
  try { const data=await dataAnalyticsAPI.report({...appliedQuery.value!,search:'',page:1,user_id:p.user_id,versions:JSON.stringify(report.value?.versions)},controller.signal);if(!controller.signal.aborted)personReport.value=data }
  catch(e){if(!controller.signal.aborted)app.showError(e instanceof Error?e.message:'明细加载失败；发布版本变化时请刷新报表')}
  finally{if(!controller.signal.aborted)personLoading.value=false}
}
function closePerson(){detailAbort?.abort();selectedPerson.value=undefined;personReport.value=undefined;personLoading.value=false}
async function download(){if(!report.value)return;exporting.value=true;const params={...appliedQuery.value!,search:'',export:true,versions:JSON.stringify(report.value.versions)}
  try {await exportAnalytics(await dataAnalyticsAPI.report(params));app.showSuccess('分析报表已导出（8个工作表）')}
  catch(e){app.showError(e instanceof Error?e.message:'导出失败，请刷新报表后重试')}finally{exporting.value=false}
}
async function rebuild(){if(!rebuildStart.value || !rebuildEnd.value){app.showError('请选择补算开始和结束日期');return}rebuilding.value=true
  try{await dataAnalyticsAPI.rebuild(rebuildStart.value,rebuildEnd.value);app.showSuccess('补算已排队，后台每5分钟分批处理');await load()}
  catch(e){app.showError(e instanceof Error?e.message:'补算提交失败')}finally{rebuilding.value=false}
}
onMounted(()=>{void load(true);timer=setInterval(()=>{if(!document.hidden && !loading.value && !directoryOpen.value && !selectedPerson.value)void load()},30000)})
onBeforeUnmount(()=>{disposed=true;++sequence;abort?.abort();detailAbort?.abort();if(timer)clearInterval(timer);chart?.destroy()})
</script>
<style scoped>
.analytics-table th:not(:first-child), .analytics-table td:not(:first-child) { text-align: right; font-variant-numeric: tabular-nums; white-space: nowrap; }
.analytics-table th:first-child, .analytics-table td:first-child { text-align: left; }
</style>
