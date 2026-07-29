<template>
  <AppLayout>
    <div class="space-y-5">
      <div class="flex gap-6 border-b border-gray-200 dark:border-dark-700">
        <button v-for="item in tabs" :key="item.key" class="border-b-2 px-1 pb-3 text-sm font-medium" :class="tab===item.key?'border-primary-500 text-primary-600':'border-transparent text-gray-500'" @click="tab=item.key">{{ item.label }}</button>
      </div>

      <template v-if="tab==='overview'">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm font-medium">统计范围</span>
            <button v-for="p in presets" :key="p.key" class="btn btn-secondary btn-sm" :class="rangePreset===p.key?'!border-primary-500 !text-primary-600':''" @click="applyPreset(p.key)">{{ p.label }}</button>
            <Select v-model="rangeKind" :options="rangeKindOptions" class="w-24" />
            <Select v-if="rangeKind==='month'" v-model="monthStart" :options="monthOptions" searchable class="w-32" />
            <Select v-else v-model="yearStart" :options="yearOptions" class="w-28" />
            <span class="text-gray-400">至</span>
            <Select v-if="rangeKind==='month'" v-model="monthEnd" :options="monthOptions" searchable class="w-32" />
            <Select v-else v-model="yearEnd" :options="yearOptions" class="w-28" />
            <button class="btn btn-secondary btn-sm" @click="applySelectedRange">应用</button>
          </div>
          <span class="text-sm" :class="aggregationDelayed?'text-amber-600':'text-gray-500'">每 5 分钟聚合 · {{ aggregationDelayed?'数据更新延迟 · ':'' }}{{ lastUpdated }}</span>
        </div>
        <div class="grid gap-4 md:grid-cols-4">
          <div v-for="card in overviewCards" :key="card.label" class="card p-5"><div class="text-sm text-gray-500">{{ card.label }}</div><div class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ card.value }}</div></div>
        </div>
        <div class="flex flex-wrap items-center justify-between gap-2 text-sm text-gray-500">
          <span>较上一等长周期：{{ comparisonText }}</span>
          <span :class="overview.coverage_complete?'text-emerald-600':'text-amber-600'">{{ coverageText }}</span>
        </div>
        <div class="card p-5">
          <div class="flex items-center justify-between"><div><b>动态成本核算</b><span class="ml-3 text-sm" :class="completion===100?'text-emerald-600':'text-amber-600'">{{ completion===100?'正常':completion+'%' }} · 待核算 {{ overview.pending_count }} 条 · 异常 {{ overview.error_count }} 条</span></div><button class="btn btn-secondary btn-sm" @click="openRecalc">历史补算</button></div>
          <div v-if="completion<100" class="mt-3 h-2 overflow-hidden rounded bg-gray-100"><div class="h-full bg-primary-500" :style="{width:completion+'%'}"></div></div>
        </div>
        <div class="flex justify-end"><div class="inline-flex rounded-lg border border-gray-200 p-1 dark:border-dark-700"><button v-for="p in chartPeriods" :key="p.key" class="rounded px-3 py-1.5 text-sm" :class="chartPeriod===p.key?'bg-primary-50 text-primary-600':''" @click="loadAnalysis(p.key)">{{ p.label }}</button></div></div>
        <div class="grid gap-4 xl:grid-cols-[1.5fr_1fr]">
          <div class="card p-5"><h2 class="font-semibold">成本趋势 <span class="text-sm font-normal text-gray-400">· {{ chartRangeLabel }}</span></h2><div class="mt-4 h-80"><canvas ref="chartCanvas"></canvas></div></div>
          <div class="card p-5"><h2 class="font-semibold">成本方案 Top 5 <span class="text-sm font-normal text-gray-400">· {{ chartRangeLabel }}</span></h2><div class="mt-5 space-y-4"><div v-for="x in analysis.top" :key="x.plan_id"><div class="flex justify-between text-sm"><span>{{ x.plan_name }}</span><b>{{ money(x.amount_cny) }} · {{ topPercent(x.amount_cny) }}</b></div><div class="mt-1 h-2 rounded bg-gray-100"><div class="h-full rounded bg-primary-500" :style="{width:topWidth(x.amount_cny)}"></div></div></div><div v-if="!analysis.top.length" class="py-12 text-center text-gray-400">暂无成本数据</div></div></div>
        </div>
      </template>

      <template v-else-if="tab==='accounts'">
        <div class="flex flex-wrap justify-between gap-3"><div class="flex gap-2"><input v-model="accountSearch" class="input w-64" placeholder="搜索账号或平台" @keyup.enter="loadAccounts"><Select v-model="accountMode" :options="accountModeOptions" class="w-40" @change="accountPage=1;loadAccounts()" /></div><button class="btn btn-primary" :disabled="!selectedAccounts.size" @click="openBatch">批量配置（{{ selectedAccounts.size }}）</button></div>
        <div class="card overflow-hidden"><div class="overflow-x-auto"><table class="table"><thead><tr><th><input type="checkbox" :checked="allAccountsSelected" @change="toggleAllAccounts"></th><th>账号</th><th>平台</th><th>成本方式</th><th>成本方案</th><th>生效时间</th><th>待核算</th><th>操作</th></tr></thead><tbody><tr v-for="x in accounts" :key="x.account_id"><td><input type="checkbox" :checked="selectedAccounts.has(x.account_id)" @change="toggleAccount(x.account_id)"></td><td>{{ x.account_name }}</td><td>{{ x.platform }}</td><td>{{ modeLabel(x.cost_mode) }}</td><td>{{ x.plan_name||'-' }}</td><td>{{ date(x.effective_from) }}</td><td>{{ x.pending_count }}</td><td><button class="text-primary-600" @click="openAccount(x)">配置</button></td></tr></tbody></table></div><Pagination :page="accountPage" :page-size="pageSize" :total="accountTotal" @update:page="accountPage=$event;loadAccounts()" @update:pageSize="pageSize=$event;accountPage=1;loadAccounts()" /></div>
      </template>

      <template v-else>
        <div class="flex flex-wrap justify-between gap-3"><div class="flex gap-2"><input v-model="planSearch" class="input w-64" placeholder="搜索成本方案" @keyup.enter="loadPlans"><Select v-model="planType" :options="planTypeOptions" class="w-40" @change="planPage=1;loadPlans()" /></div><button class="btn btn-primary" @click="openPlan()">新建成本方案</button></div>
        <div class="card overflow-hidden"><div class="overflow-x-auto"><table class="table"><thead><tr><th>方案</th><th>类型</th><th>模型/采购数量</th><th>绑定账号</th><th>生效时间</th><th>状态</th><th>操作</th></tr></thead><tbody><tr v-for="x in plans" :key="x.id"><td>{{ x.name }}</td><td>{{ x.plan_type==='metered'?'按量':'固定' }}</td><td>{{ x.plan_type==='metered'?x.model_count+' 个模型':x.purchase_quantity+' 份' }}</td><td>{{ x.account_count }}</td><td>{{ date(x.effective_from) }}</td><td>{{ x.status==='active'?'启用':'停用' }}</td><td class="space-x-3"><button class="text-primary-600" @click="editPlan(x)">编辑</button><button v-if="x.status==='active'" class="text-red-500" @click="disablePlan(x)">停用</button></td></tr></tbody></table></div><Pagination :page="planPage" :page-size="pageSize" :total="planTotal" @update:page="planPage=$event;loadPlans()" @update:pageSize="pageSize=$event;planPage=1;loadPlans()" /></div>
      </template>
    </div>

    <BaseDialog :show="accountDialog" :title="batchMode?'批量配置账号成本':'配置账号成本'" @close="accountDialog=false">
      <div class="space-y-4">
        <div><label class="input-label">成本方案</label><Select v-model="selectedAccountPlan" :options="accountPlanOptions" /></div>
        <div v-if="accountForm.cost_mode==='excluded'"><label class="input-label">排除原因</label><input v-model="accountForm.exclude_reason" class="input w-full"></div>
        <div><label class="input-label">生效时间</label><input v-model="accountForm.effective_from" type="datetime-local" class="input w-full"></div>
        <div><label class="input-label">结束时间（可选）</label><input v-model="accountForm.effective_to" type="datetime-local" class="input w-full"></div>
      </div>
      <template #footer><button v-if="!batchMode&&editingAccountConfigured" class="btn btn-danger mr-auto" @click="endAccountCost">结束当前核算</button><button class="btn btn-secondary" @click="accountDialog=false">取消</button><button class="btn btn-primary" @click="saveAccountForm">保存</button></template>
    </BaseDialog>

    <BaseDialog :show="planDialog" :title="editingPlanId?'编辑成本方案':'新建成本方案'" width="wide" @close="planDialog=false">
      <div class="space-y-4">
        <div class="grid gap-4 md:grid-cols-2"><div><label class="input-label">方案名称</label><input v-model="planForm.name" class="input w-full"></div><div><label class="input-label">类型</label><Select v-model="planForm.plan_type" :options="createPlanTypeOptions" :disabled="!!editingPlanId" /></div></div>
        <div class="grid gap-4 md:grid-cols-2"><div><label class="input-label">生效时间</label><input v-model="planForm.effective_from" type="datetime-local" class="input w-full"></div><div><label class="input-label">结束时间（可选）</label><input v-model="planForm.effective_to" type="datetime-local" class="input w-full"></div><template v-if="planForm.plan_type==='fixed'"><div><label class="input-label">固定成本分类</label><Select v-model="planForm.fixed_category" :options="fixedCategoryOptions" /></div><div><label class="input-label">付费周期</label><Select v-model="planForm.billing_cycle" :options="billingCycleOptions" /></div><div><label class="input-label">单份{{ planForm.billing_cycle==='yearly'?'年':'月' }}成本（CNY）</label><input v-model="planForm.fixed_unit_cost_cny" type="number" min="0" class="input w-full"><p v-if="planForm.billing_cycle==='yearly'" class="mt-1 text-xs text-gray-500">后台按年费 ÷ 12 折算每月成本</p></div><div><label class="input-label">采购数量</label><input v-model.number="planForm.purchase_quantity" type="number" min="1" class="input w-full"></div></template></div>
        <div v-if="planForm.plan_type==='metered'" class="space-y-2">
          <div class="flex justify-between"><b>模型价格（Token 价格为 CNY / MTok）</b><button class="btn btn-secondary btn-sm" @click="addPrice">添加模型</button></div>
          <div v-for="(p,i) in planForm.prices" :key="i" class="space-y-3 rounded border border-gray-200 p-3">
            <div class="flex gap-3"><Select v-model="p.upstream_model" :options="modelOptions" searchable creatable class="min-w-0 flex-1" placeholder="选择或输入上游模型" /><button class="text-red-500" :disabled="planForm.prices.length===1" @click="planForm.prices.splice(i,1)">删除</button></div>
            <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
              <div><label class="input-label">计价方式</label><Select v-model="p.billing_mode" :options="billingModeOptions" /></div>
              <div><label class="input-label">输入 Token</label><input v-model="p.input_price_cny" class="input w-full" type="number" min="0"></div>
              <div><label class="input-label">输出 Token</label><input v-model="p.output_price_cny" class="input w-full" type="number" min="0"></div>
              <div><label class="input-label">缓存写入 Token</label><input v-model="p.cache_write_price_cny" class="input w-full" type="number" min="0"></div>
              <div><label class="input-label">缓存读取 Token</label><input v-model="p.cache_read_price_cny" class="input w-full" type="number" min="0"></div>
              <div><label class="input-label">图片输入 Token</label><input v-model="p.image_input_price_cny" class="input w-full" type="number" min="0"></div>
              <div><label class="input-label">图片输出 Token</label><input v-model="p.image_output_price_cny" class="input w-full" type="number" min="0"></div>
              <div><label class="input-label">每次请求</label><input v-model="p.per_request_price_cny" class="input w-full" type="number" min="0"></div>
            </div>
          </div>
        </div>
      </div>
      <template #footer><button class="btn btn-secondary" @click="planDialog=false">取消</button><button class="btn btn-primary" @click="savePlan">保存</button></template>
    </BaseDialog>

    <BaseDialog :show="recalcDialog" title="历史成本补算" width="wide" @close="recalcDialog=false">
      <div class="space-y-5">
        <div><label class="input-label">补算日期范围</label><DateRangePicker v-model:start-date="recalc.start_date" v-model:end-date="recalc.end_date" /></div>
        <div><h3 class="mb-2 font-medium">补算任务</h3><div class="overflow-x-auto"><table class="table"><thead><tr><th>范围</th><th>状态</th><th>进度</th><th>创建时间</th></tr></thead><tbody><tr v-for="job in recalcJobs" :key="job.id"><td>{{ date(job.start_date) }} 至 {{ date(job.end_date) }}</td><td>{{ job.status }}</td><td>{{ job.completed_days }}/{{ job.total_days }}</td><td>{{ date(job.created_at) }}</td></tr></tbody></table><div v-if="!recalcJobs.length" class="py-6 text-center text-sm text-gray-400">暂无补算任务</div></div><Pagination :page="recalcPage" :page-size="20" :total="recalcTotal" @update:page="recalcPage=$event;loadRecalculations()" /></div>
      </div>
      <template #footer><button class="btn btn-secondary" @click="recalcDialog=false">取消</button><button class="btn btn-primary" @click="submitRecalc">开始补算</button></template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { Chart } from 'chart.js/auto'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import { useAppStore } from '@/stores'
import costManagementAPI, { type AccountCostInput, type AccountCostRow, type CostAnalysis, type CostJob, type CostOverview, type CostPlan } from '@/api/admin/costManagement'

const app=useAppStore(),tab=ref<'overview'|'accounts'|'plans'>('overview'),tabs=[{key:'overview',label:'成本总览'},{key:'accounts',label:'账号成本'},{key:'plans',label:'成本方案'}] as const
const today=new Date().toISOString().slice(0,10),month=today.slice(0,7),currentYear=Number(today.slice(0,4)),monthStart=ref(month),monthEnd=ref(month),yearStart=ref(currentYear),yearEnd=ref(currentYear),rangeKind=ref<'month'|'year'>('month'),rangePreset=ref('month'),range=reactive({start:month+'-01',end:today})
const presets=[{key:'week',label:'本周'},{key:'month',label:'本月'},{key:'lastMonth',label:'上月'},{key:'year',label:'本年'},{key:'lastYear',label:'去年'}]
const rangeKindOptions=[{value:'month',label:'按月'},{value:'year',label:'按年'}]
const monthOptions=Array.from({length:120},(_,i)=>{const d=new Date(currentYear,new Date().getMonth()-i,1),value=`${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,'0')}`;return {value,label:value}})
const yearOptions=Array.from({length:10},(_,i)=>({value:currentYear-i,label:String(currentYear-i)}))
const overview=reactive<CostOverview>({dynamic_cost_cny:'0',fixed_cost_cny:'0',total_cost_cny:'0',pending_count:0,error_count:0,eligible_count:0,calculated_count:0,coverage_complete:true,previous_coverage_complete:true,previous_total_cost_cny:'0'}),analysis=reactive<CostAnalysis>({period:'day',total_cost_cny:'0',trend:[],top:[]})
const completion=computed(()=>overview.eligible_count?Math.round(overview.calculated_count/overview.eligible_count*1000)/10:100),lastUpdated=computed(()=>overview.last_success_at?new Date(overview.last_success_at).toLocaleString():'尚未聚合')
const aggregationDelayed=computed(()=>!overview.last_success_at||Date.now()-new Date(overview.last_success_at).getTime()>10*60*1000)
const money=(v:string|number)=>'¥'+Number(v||0).toLocaleString('zh-CN',{minimumFractionDigits:2,maximumFractionDigits:2})
const comparisonText=computed(()=>{if(!overview.previous_coverage_complete)return '上一周期数据不完整';const current=Number(overview.total_cost_cny),previous=Number(overview.previous_total_cost_cny);if(!previous)return current?'新增 '+money(current):'持平';const percent=(current-previous)/previous*100;return `${percent>=0?'增加':'减少'} ${Math.abs(percent).toFixed(1)}%`})
const coverageText=computed(()=>overview.coverage_complete?'数据覆盖完整':`数据不完整 · 当前覆盖 ${date(overview.coverage_start)} 至 ${date(overview.coverage_end)}`)
const overviewCards=computed(()=>[{label:'真实总成本',value:money(overview.total_cost_cny)},{label:'动态成本',value:money(overview.dynamic_cost_cny)},{label:'固定成本',value:money(overview.fixed_cost_cny)},{label:'待核算',value:overview.pending_count+' 条'}])
const chartPeriods=[{key:'week',label:'近一周'},{key:'day',label:'按天'},{key:'month',label:'按月'},{key:'year',label:'按年'}],chartPeriod=ref('day'),chartCanvas=ref<HTMLCanvasElement>(),chart=ref<Chart>()
const chartRangeLabel=computed(()=>({week:'近 7 天',day:'近 30 天',month:'近 12 个月',year:'近 5 年'}[chartPeriod.value]))
async function loadOverview(){Object.assign(overview,await costManagementAPI.overview({start_date:range.start,end_date:range.end}))}
async function loadAnalysis(period=chartPeriod.value){chartPeriod.value=period;Object.assign(analysis,await costManagementAPI.analysis(period));await nextTick();chart.value?.destroy();if(!chartCanvas.value)return;chart.value=new Chart(chartCanvas.value,{type:'line',data:{labels:analysis.trend.map(x=>x.bucket),datasets:[{label:'总成本',data:analysis.trend.map(x=>+x.total_cost_cny),borderColor:'#7a5af8'},{label:'动态成本',data:analysis.trend.map(x=>+x.dynamic_cost_cny),borderColor:'#0866ed'},{label:'固定成本',data:analysis.trend.map(x=>+x.fixed_cost_cny),borderColor:'#ff7800'}]},options:{responsive:true,maintainAspectRatio:false}})}
function applyPreset(key:string){rangePreset.value=key;const now=new Date(),y=now.getFullYear(),m=now.getMonth();if(key==='week'){const d=(now.getDay()+6)%7,s=new Date(now);s.setDate(now.getDate()-d);range.start=s.toISOString().slice(0,10);range.end=today}else if(key==='month'){range.start=month+'-01';range.end=today}else if(key==='lastMonth'){const s=new Date(y,m-1,1),e=new Date(y,m,0);range.start=s.toISOString().slice(0,10);range.end=e.toISOString().slice(0,10)}else if(key==='year'){range.start=y+'-01-01';range.end=today}else{range.start=(y-1)+'-01-01';range.end=(y-1)+'-12-31'}loadOverview()}
function applySelectedRange(){rangePreset.value='';if(rangeKind.value==='month'){if(!monthStart.value||!monthEnd.value||monthStart.value>monthEnd.value)return;range.start=monthStart.value+'-01';const [y,m]=monthEnd.value.split('-').map(Number),end=new Date(y,m,0).toISOString().slice(0,10);range.end=end>today?today:end}else{if(yearStart.value>yearEnd.value)return;range.start=yearStart.value+'-01-01';range.end=yearEnd.value>=currentYear?today:yearEnd.value+'-12-31'}loadOverview()}
const topWidth=(v:string)=>{const max=Math.max(...analysis.top.map(x=>+x.amount_cny),1);return Math.round(+v/max*100)+'%'}
const topPercent=(v:string)=>Number(analysis.total_cost_cny)?(Number(v)/Number(analysis.total_cost_cny)*100).toFixed(1)+'%':'0%'

const pageSize=ref(20),accounts=ref<AccountCostRow[]>([]),accountTotal=ref(0),accountPage=ref(1),accountSearch=ref(''),accountMode=ref(''),selectedAccounts=ref(new Set<number>())
const accountModeOptions=[{value:'',label:'全部成本方式'},{value:'metered',label:'按使用量'},{value:'fixed',label:'固定成本'},{value:'excluded',label:'不纳入核算'}]
async function loadAccounts(){const r=await costManagementAPI.accounts({page:accountPage.value,page_size:pageSize.value,search:accountSearch.value,mode:accountMode.value});accounts.value=r.items;accountTotal.value=r.total}
const allAccountsSelected=computed(()=>accounts.value.length>0&&accounts.value.every(x=>selectedAccounts.value.has(x.account_id)))
function toggleAccount(id:number){const s=new Set(selectedAccounts.value);s.has(id)?s.delete(id):s.add(id);selectedAccounts.value=s}
function toggleAllAccounts(){const s=new Set(selectedAccounts.value);allAccountsSelected.value?accounts.value.forEach(x=>s.delete(x.account_id)):accounts.value.forEach(x=>s.add(x.account_id));selectedAccounts.value=s}
const modeLabel=(v:string)=>({metered:'按使用量',fixed:'固定成本',excluded:'不纳入核算'}[v]||'未配置'),date=(v?:string)=>v?new Date(v).toLocaleDateString():'-'
const accountDialog=ref(false),batchMode=ref(false),editingAccount=ref<number>(),editingAccountConfigured=ref(false),accountForm=reactive<any>({cost_mode:'metered',plan_id:undefined,effective_from:new Date().toISOString().slice(0,16),effective_to:'',exclude_reason:''})
const planChoices=ref<CostPlan[]>([])
// ponytail: local search over 100 plans; switch Select to remote search if deployments exceed this.
async function loadPlanChoices(){const r=await costManagementAPI.plans({page:1,page_size:100});planChoices.value=r.items}
const accountPlanOptions=computed(()=>[
  ...planChoices.value.map(x=>({value:x.id,label:`${x.name}（${x.plan_type==='metered'?'按量':'固定'}）`,disabled:x.status!=='active'})),
  {value:'excluded',label:'不纳入成本核算',disabled:false},
])
const selectedAccountPlan=computed<string|number|undefined>({
  get:()=>accountForm.cost_mode==='excluded'?'excluded':accountForm.plan_id,
  set:value=>{
    if(value==='excluded'){accountForm.cost_mode='excluded';accountForm.plan_id=undefined;return}
    const plan=planChoices.value.find(x=>x.id===value)
    accountForm.cost_mode=plan?.plan_type||'metered';accountForm.plan_id=value;accountForm.exclude_reason=''
  },
})
function openAccount(x:AccountCostRow){batchMode.value=false;editingAccount.value=x.account_id;editingAccountConfigured.value=!!x.cost_mode;Object.assign(accountForm,{cost_mode:x.cost_mode||'metered',plan_id:x.plan_id,effective_from:new Date().toISOString().slice(0,16),effective_to:'',exclude_reason:x.exclude_reason||''});accountDialog.value=true}
function openBatch(){batchMode.value=true;editingAccount.value=undefined;Object.assign(accountForm,{cost_mode:'metered',plan_id:undefined,effective_from:new Date().toISOString().slice(0,16),effective_to:'',exclude_reason:''});accountDialog.value=true}
async function saveAccountForm(){const input:AccountCostInput={...accountForm,plan_id:accountForm.cost_mode==='excluded'?undefined:accountForm.plan_id,exclude_reason:accountForm.cost_mode==='excluded'?accountForm.exclude_reason:'',effective_from:new Date(accountForm.effective_from).toISOString(),effective_to:accountForm.effective_to?new Date(accountForm.effective_to).toISOString():undefined};try{batchMode.value?await costManagementAPI.saveAccounts([...selectedAccounts.value],input):await costManagementAPI.saveAccount(editingAccount.value!,input);app.showSuccess('成本配置已保存');accountDialog.value=false;selectedAccounts.value=new Set();loadAccounts()}catch(e:any){app.showError(e.message||'保存失败')}}
async function endAccountCost(){if(!editingAccount.value||!confirm('确认结束当前账号的成本核算？历史成本不会改变。'))return;try{await costManagementAPI.endAccount(editingAccount.value);app.showSuccess('当前成本核算已结束');accountDialog.value=false;loadAccounts()}catch(e:any){app.showError(e.message||'结束失败')}}

const plans=ref<CostPlan[]>([]),planTotal=ref(0),planPage=ref(1),planSearch=ref(''),planType=ref(''),planDialog=ref(false),editingPlanId=ref<number>()
const planTypeOptions=[{value:'',label:'全部类型'},{value:'metered',label:'按量成本'},{value:'fixed',label:'固定成本'}],createPlanTypeOptions=planTypeOptions.slice(1),fixedCategoryOptions=[{value:'coding_plan',label:'Coding Plan'},{value:'self_hosted',label:'本地部署'},{value:'other',label:'其他'}],billingCycleOptions=[{value:'monthly',label:'月付'},{value:'yearly',label:'年付'}]
const billingModeOptions=[{value:'token',label:'按 Token'},{value:'request',label:'按请求'},{value:'hybrid',label:'混合计价'}]
const modelOptions=ref<Array<{value:string;label:string}>>([])
async function loadModelOptions(){const r=await costManagementAPI.modelOptions({page:1,page_size:100});modelOptions.value=r.items.map(x=>({value:x.model,label:x.model}))}
const emptyPrice=()=>({upstream_model:'',billing_mode:'token',input_price_cny:'0',output_price_cny:'0',cache_write_price_cny:'0',cache_read_price_cny:'0',image_input_price_cny:'0',image_output_price_cny:'0',per_request_price_cny:'0'})
const planForm=reactive<any>({name:'',plan_type:'metered',fixed_category:'coding_plan',effective_from:new Date().toISOString().slice(0,16),effective_to:'',billing_cycle:'monthly',fixed_unit_cost_cny:'0',purchase_quantity:1,note:'',prices:[emptyPrice()]})
async function loadPlans(){const r=await costManagementAPI.plans({page:planPage.value,page_size:pageSize.value,search:planSearch.value,type:planType.value});plans.value=r.items;planTotal.value=r.total}
function openPlan(){editingPlanId.value=undefined;Object.assign(planForm,{name:'',plan_type:'metered',fixed_category:'coding_plan',effective_from:new Date().toISOString().slice(0,16),effective_to:'',billing_cycle:'monthly',fixed_unit_cost_cny:'0',purchase_quantity:1,note:'',prices:[emptyPrice()]});planDialog.value=true}
async function editPlan(x:CostPlan){const p=await costManagementAPI.plan(x.id);editingPlanId.value=x.id;Object.assign(planForm,{...p,effective_from:new Date().toISOString().slice(0,16),effective_to:'',prices:p.prices?.length?p.prices.map(y=>({...y})):[emptyPrice()]});planDialog.value=true}
function addPrice(){planForm.prices.push(emptyPrice())}
async function savePlan(){try{if(planForm.plan_type==='metered'){const models=planForm.prices.map((x:any)=>x.upstream_model.trim());if(models.some((x:string)=>!x)||new Set(models).size!==models.length)throw new Error('上游模型不能为空且不能重复')}const input={...planForm,effective_from:new Date(planForm.effective_from).toISOString(),effective_to:planForm.effective_to?new Date(planForm.effective_to).toISOString():undefined};editingPlanId.value?await costManagementAPI.updatePlan(editingPlanId.value,input):await costManagementAPI.createPlan(input);app.showSuccess('成本方案已保存');planDialog.value=false;loadPlans()}catch(e:any){app.showError(e.message||'保存失败')}}
async function disablePlan(x:CostPlan){if(!confirm(`确认停用“${x.name}”？`))return;await costManagementAPI.disablePlan(x.id);loadPlans()}
const recalcDialog=ref(false),recalc=reactive({start_date:month+'-01',end_date:today}),recalcJobs=ref<CostJob[]>([]),recalcPage=ref(1),recalcTotal=ref(0)
async function loadRecalculations(){const r=await costManagementAPI.recalculations({page:recalcPage.value,page_size:20});recalcJobs.value=r.items;recalcTotal.value=r.total}
function openRecalc(){recalcDialog.value=true;loadRecalculations()}
async function submitRecalc(){if(!confirm(`确认补算 ${recalc.start_date} 至 ${recalc.end_date}？`))return;await costManagementAPI.createRecalculation(recalc);app.showSuccess('补算任务已创建');recalcPage.value=1;loadRecalculations()}
watch(tab,v=>{if(v==='accounts'){loadAccounts();loadPlanChoices()}else if(v==='plans')loadPlans()})
onMounted(()=>{loadOverview();loadAnalysis();loadPlans();loadModelOptions()});onBeforeUnmount(()=>chart.value?.destroy())
</script>
