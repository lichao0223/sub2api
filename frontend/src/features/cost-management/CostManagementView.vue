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
            <DateRangePicker v-model:start-date="range.start" v-model:end-date="range.end" period-mode @change="loadOverview" />
          </div>
          <div class="flex items-center gap-3"><span class="text-sm" :class="aggregationDelayed?'text-amber-600':'text-gray-500'">每 5 分钟聚合 · {{ aggregationDelayed?'数据更新延迟 · ':'' }}{{ lastUpdated }}</span><button class="btn btn-secondary btn-sm" @click="openRecalc">核算任务</button></div>
        </div>
        <div class="grid gap-4 md:grid-cols-4">
          <div v-for="card in overviewCards" :key="card.label" class="card p-5"><div class="text-sm text-gray-500">{{ card.label }}</div><div class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ card.value }}</div></div>
        </div>
        <div class="flex flex-wrap items-center justify-between gap-2 text-sm text-gray-500">
          <span>较上一等长周期：{{ comparisonText }}</span>
          <span :class="overview.coverage_complete?'text-emerald-600':'text-amber-600'">{{ coverageText }}</span>
        </div>
        <div class="card p-5">
          <div class="flex items-center justify-between"><div><b>动态成本核算</b><span class="ml-3 text-sm" :class="completion===100?'text-emerald-600':'text-amber-600'">{{ completion===100?'正常':completion+'%' }} · 待核算 {{ overview.pending_count }} 条 · 异常 {{ overview.error_count }} 条</span><div v-if="latestRecalculation" class="mt-2 text-sm" :class="latestRecalculation.status==='failed'?'text-red-600':'text-gray-500'">最近补算：{{ jobStatusLabel(latestRecalculation) }} · {{ latestRecalculation.completed_days }}/{{ latestRecalculation.total_days }} 天<span v-if="latestRecalculation.error_message"> · {{ latestRecalculation.error_message }}</span></div></div><button class="btn btn-secondary btn-sm" @click="openRecalc">历史补算</button></div>
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
        <div class="card overflow-hidden"><div class="overflow-x-auto"><table class="table"><thead><tr><th><input type="checkbox" :checked="allAccountsSelected" @change="toggleAllAccounts"></th><th>账号</th><th>平台</th><th>成本方式</th><th>成本方案</th><th>归属订阅</th><th>生效时间</th><th>待核算</th><th>操作</th></tr></thead><tbody><tr v-for="x in accounts" :key="x.account_id"><td><input type="checkbox" :checked="selectedAccounts.has(x.account_id)" @change="toggleAccount(x.account_id)"></td><td>{{ x.account_name }}</td><td>{{ x.platform }}</td><td>{{ modeLabel(x.cost_mode) }}</td><td>{{ x.plan_name||'-' }}</td><td><span v-if="x.cost_mode==='fixed'" :class="x.subscription_unit_name?'':'text-amber-600'">{{ x.subscription_unit_name||'待归组' }}</span><span v-else>-</span></td><td>{{ date(x.effective_from) }}</td><td>{{ x.pending_count }}</td><td><button class="text-primary-600" @click="openAccount(x)">配置</button></td></tr></tbody></table></div><Pagination :page="accountPage" :page-size="pageSize" :total="accountTotal" @update:page="accountPage=$event;loadAccounts()" @update:pageSize="pageSize=$event;accountPage=1;loadAccounts()" /></div>
      </template>

      <template v-else>
        <div class="flex flex-wrap justify-between gap-3"><div class="flex gap-2"><input v-model="planSearch" class="input w-64" placeholder="搜索成本方案" @keyup.enter="loadPlans"><Select v-model="planType" :options="planTypeOptions" class="w-40" @change="planPage=1;loadPlans()" /></div><button class="btn btn-primary" @click="openPlan()">新建成本方案</button></div>
        <div class="card overflow-hidden"><div class="overflow-x-auto"><table class="table"><thead><tr><th>方案</th><th>类型</th><th>模型/订阅实例</th><th>绑定账号</th><th>生效时间</th><th>状态</th><th>操作</th></tr></thead><tbody><tr v-for="x in plans" :key="x.id"><td>{{ x.name }}</td><td>{{ x.plan_type==='metered'?'按量':'固定' }}</td><td><template v-if="x.plan_type==='metered'">{{ x.model_count }} 个模型</template><template v-else>{{ x.subscription_unit_count||0 }} 个实例 <span v-if="x.unassigned_account_count" class="ml-1 text-xs text-amber-600">· {{ x.unassigned_account_count }} 个账号待归组</span></template></td><td>{{ x.account_count }}</td><td>{{ date(x.effective_from) }}</td><td>{{ x.status==='active'?'启用':'停用' }}</td><td class="space-x-3"><button class="text-primary-600" @click="editPlan(x)">编辑</button><button v-if="x.status==='active'" class="text-red-500" @click="disablePlan(x)">停用</button></td></tr></tbody></table></div><Pagination :page="planPage" :page-size="pageSize" :total="planTotal" @update:page="planPage=$event;loadPlans()" @update:pageSize="pageSize=$event;planPage=1;loadPlans()" /></div>
      </template>
    </div>

    <BaseDialog :show="accountDialog" :title="batchMode?'批量配置账号成本':'配置账号成本'" @close="accountDialog=false">
      <div class="space-y-4">
        <div><label class="input-label">成本方案</label><Select v-model="selectedAccountPlan" :options="accountPlanOptions" /></div>
        <div v-if="accountForm.cost_mode==='fixed'"><label class="input-label">归属订阅实例</label><Select v-model="accountForm.subscription_unit_id" :options="subscriptionUnitOptions" /><p class="mt-1 text-xs text-gray-500">同一份订阅建立多个平台账号时，请选择同一个实例，只计算一份成本。旧方案全部归组前仍沿用原采购数量。</p></div>
        <div v-if="accountForm.cost_mode==='fixed'&&accountForm.subscription_unit_id==='new'"><label class="input-label">新订阅实例名称</label><input v-model="accountForm.new_subscription_unit_name" class="input w-full" placeholder="例如：ChatGPT Plus #3"></div>
        <div v-if="accountForm.cost_mode==='excluded'"><label class="input-label">排除原因</label><input v-model="accountForm.exclude_reason" class="input w-full"></div>
        <div><label class="input-label">生效时间</label><input v-model="accountForm.effective_from" type="datetime-local" class="input w-full"></div>
        <div><label class="input-label">结束时间（可选）</label><input v-model="accountForm.effective_to" type="datetime-local" class="input w-full"></div>
      </div>
      <template #footer><button v-if="!batchMode&&editingAccountConfigured" class="btn btn-danger mr-auto" @click="endAccountCost">结束当前核算</button><button class="btn btn-secondary" @click="accountDialog=false">取消</button><button class="btn btn-primary" @click="saveAccountForm">保存</button></template>
    </BaseDialog>

    <BaseDialog :show="planDialog" :title="editingPlanId?'编辑成本方案':'新建成本方案'" width="wide" @close="planDialog=false">
      <div class="space-y-4">
        <div class="grid gap-4 md:grid-cols-2"><div><label class="input-label">方案名称</label><input v-model="planForm.name" class="input w-full"></div><div><label class="input-label">类型</label><Select v-model="planForm.plan_type" :options="createPlanTypeOptions" :disabled="!!editingPlanId" /></div></div>
        <div class="grid gap-4 md:grid-cols-2"><div><label class="input-label">{{ editingPlanId?'本次变更生效时间':'生效时间' }}</label><input v-model="planForm.effective_from" type="datetime-local" class="input w-full"><p v-if="editingPlanId" class="mt-1 text-xs" :class="planCreatesNewVersion?'text-primary-600':'text-amber-600'">{{ planVersionChangeHint }}</p></div><div><label class="input-label">结束时间（可选）</label><input v-model="planForm.effective_to" type="datetime-local" class="input w-full"></div><template v-if="planForm.plan_type==='fixed'"><div><label class="input-label">固定成本分类</label><Select v-model="planForm.fixed_category" :options="fixedCategoryOptions" /></div><div><label class="input-label">付费周期</label><Select v-model="planForm.billing_cycle" :options="billingCycleOptions" /></div><div><label class="input-label">单份{{ planForm.billing_cycle==='yearly'?'年':'月' }}成本（CNY）</label><input v-model="planForm.fixed_unit_cost_cny" type="number" min="0" class="input w-full"><p class="mt-1 text-xs text-gray-500">{{ planForm.billing_cycle==='yearly'?'后台按年费 ÷ 12 折算每月成本。':'订阅数量由账号成本中的订阅实例自动统计。' }}</p></div></template></div>
        <div v-if="planForm.plan_type==='metered'" class="space-y-2">
          <div class="flex justify-between"><b>模型价格（Token 价格为 CNY / MTok）</b><button class="btn btn-secondary btn-sm" @click="addPrice">添加模型</button></div>
          <div v-for="(p,i) in planForm.prices" :key="i" class="space-y-3 rounded border border-gray-200 p-3">
            <div class="flex gap-3"><Select v-model="p.upstream_model" :options="modelOptions" searchable creatable class="min-w-0 flex-1" placeholder="选择或输入上游模型" /><button class="text-red-500" :disabled="planForm.prices.length===1" @click="planForm.prices.splice(i,1)">删除</button></div>
            <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
              <div><label class="input-label">计价方式</label><Select v-model="p.billing_mode" :options="billingModeOptions" @change="changeBillingMode(p,$event)" /></div>
              <template v-if="p.billing_mode!=='request'">
                <div><label class="input-label">输入 Token</label><input v-model="p.input_price_cny" class="input w-full" type="number" min="0"></div>
                <div><label class="input-label">输出 Token</label><input v-model="p.output_price_cny" class="input w-full" type="number" min="0"></div>
                <div><label class="input-label">缓存写入 Token</label><input v-model="p.cache_write_price_cny" class="input w-full" type="number" min="0"></div>
                <div><label class="input-label">缓存读取 Token</label><input v-model="p.cache_read_price_cny" class="input w-full" type="number" min="0"></div>
                <div><label class="input-label">图片输入 Token</label><input v-model="p.image_input_price_cny" class="input w-full" type="number" min="0"></div>
                <div><label class="input-label">图片输出 Token</label><input v-model="p.image_output_price_cny" class="input w-full" type="number" min="0"></div>
              </template>
              <div v-if="p.billing_mode!=='token'"><label class="input-label">每次请求</label><input v-model="p.per_request_price_cny" class="input w-full" type="number" min="0"></div>
            </div>
          </div>
        </div>
      </div>
      <template #footer><button class="btn btn-secondary" @click="planDialog=false">取消</button><button class="btn btn-primary" @click="savePlan">保存</button></template>
    </BaseDialog>

    <BaseDialog :show="recalcDialog" title="核算任务" width="wide" @close="recalcDialog=false">
      <div class="space-y-5">
        <div><label class="input-label">新建历史补算</label><DateRangePicker v-model:start-date="recalc.start_date" v-model:end-date="recalc.end_date" :max-date="yesterday" /></div>
        <div><h3 class="mb-1 font-medium">核算任务列表 <span class="text-xs font-normal text-gray-400">运行中每 5 秒自动刷新</span></h3><p class="mb-2 text-xs text-gray-400">日期进度按补算天数统计；总览中的待核算数量按使用记录条数统计。</p><div class="overflow-x-auto"><table class="table"><thead><tr><th>类型</th><th>范围</th><th>状态</th><th>日期进度</th><th>详情</th><th>创建时间</th><th>操作</th></tr></thead><tbody><tr v-for="job in recalcJobs" :key="job.id"><td>{{ jobTypeLabel(job.kind) }}</td><td>{{ job.kind==='recalculation'?date(job.start_date)+' 至 '+date(job.end_date):'-' }}</td><td :class="job.status==='failed'?'text-red-600':''">{{ jobStatusLabel(job) }}</td><td>{{ jobProgressText(job) }}</td><td class="max-w-64 break-words" :class="job.error_message||job.status==='failed'?'text-red-600':'text-gray-400'">{{ jobDetail(job) }}</td><td class="whitespace-nowrap">{{ dateTime(job.created_at) }}</td><td><button v-if="job.kind==='recalculation'&&['queued','running'].includes(job.status)" class="text-red-600" @click="requestCancelRecalculation(job)">取消</button><span v-else class="text-gray-400">-</span></td></tr></tbody></table><div v-if="!recalcJobs.length" class="py-6 text-center text-sm text-gray-400">暂无核算任务</div></div><Pagination :page="recalcPage" :page-size="20" :total="recalcTotal" @update:page="recalcPage=$event;loadRecalculations()" /></div>
      </div>
      <template #footer><button class="btn btn-secondary" @click="recalcDialog=false">取消</button><button class="btn btn-primary" :disabled="recalcSubmitting||hasOverlappingRecalculation" @click="submitRecalc">{{ hasOverlappingRecalculation?'所选范围已有任务':'开始补算' }}</button></template>
    </BaseDialog>
    <ConfirmDialog :show="recalcConfirm" title="确认历史成本补算" :message="`确认创建 ${recalc.start_date} 至 ${recalc.end_date} 的补算任务？`" confirm-text="确认创建" :loading="recalcSubmitting" @confirm="confirmRecalc" @cancel="recalcConfirm=false" />
    <ConfirmDialog :show="!!cancelRecalculationTarget" title="取消核算任务" message="确认取消该补算任务？运行中的任务会在当前日期处理完成后停止。" confirm-text="确认取消" :loading="cancelRecalculationSubmitting" danger @confirm="confirmCancelRecalculation" @cancel="cancelRecalculationTarget=undefined" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { Chart } from 'chart.js/auto'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import { useAppStore } from '@/stores'
import costManagementAPI, { type AccountCostInput, type AccountCostRow, type CostAnalysis, type CostJob, type CostOverview, type CostPlan, type CostSubscriptionUnit } from '@/api/admin/costManagement'

const app=useAppStore(),tab=ref<'overview'|'accounts'|'plans'>('overview'),tabs=[{key:'overview',label:'成本总览'},{key:'accounts',label:'账号成本'},{key:'plans',label:'成本方案'}] as const
const today=new Date().toISOString().slice(0,10),month=today.slice(0,7),yesterday=new Date(Date.now()-86400000).toISOString().slice(0,10),range=reactive({start:month+'-01',end:today})
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
const topWidth=(v:string)=>{const max=Math.max(...analysis.top.map(x=>+x.amount_cny),1);return Math.round(+v/max*100)+'%'}
const topPercent=(v:string)=>Number(analysis.total_cost_cny)?(Number(v)/Number(analysis.total_cost_cny)*100).toFixed(1)+'%':'0%'

const pageSize=ref(20),accounts=ref<AccountCostRow[]>([]),accountTotal=ref(0),accountPage=ref(1),accountSearch=ref(''),accountMode=ref(''),selectedAccounts=ref(new Set<number>())
const accountModeOptions=[{value:'',label:'全部成本方式'},{value:'metered',label:'按使用量'},{value:'fixed',label:'固定成本'},{value:'excluded',label:'不纳入核算'}]
async function loadAccounts(){const r=await costManagementAPI.accounts({page:accountPage.value,page_size:pageSize.value,search:accountSearch.value,mode:accountMode.value});accounts.value=r.items;accountTotal.value=r.total}
const allAccountsSelected=computed(()=>accounts.value.length>0&&accounts.value.every(x=>selectedAccounts.value.has(x.account_id)))
function toggleAccount(id:number){const s=new Set(selectedAccounts.value);s.has(id)?s.delete(id):s.add(id);selectedAccounts.value=s}
function toggleAllAccounts(){const s=new Set(selectedAccounts.value);allAccountsSelected.value?accounts.value.forEach(x=>s.delete(x.account_id)):accounts.value.forEach(x=>s.add(x.account_id));selectedAccounts.value=s}
const modeLabel=(v:string)=>({metered:'按使用量',fixed:'固定成本',excluded:'不纳入核算'}[v]||'未配置'),date=(v?:string)=>v?new Date(v).toLocaleDateString():'-'
const dateTime=(v?:string)=>v?new Date(v).toLocaleString('zh-CN',{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:false}):'-'
const accountDialog=ref(false),batchMode=ref(false),editingAccount=ref<number>(),editingAccountConfigured=ref(false),accountForm=reactive<any>({cost_mode:'metered',plan_id:undefined,subscription_unit_id:undefined,new_subscription_unit_name:'',effective_from:new Date().toISOString().slice(0,16),effective_to:'',exclude_reason:''})
const planChoices=ref<CostPlan[]>([])
const subscriptionUnits=ref<CostSubscriptionUnit[]>([])
// ponytail: local search over 100 plans; switch Select to remote search if deployments exceed this.
async function loadPlanChoices(){const r=await costManagementAPI.plans({page:1,page_size:100});planChoices.value=r.items}
const accountPlanOptions=computed(()=>[
  ...planChoices.value.map(x=>({value:x.id,label:`${x.name}（${x.plan_type==='metered'?'按量':'固定'}）`,disabled:x.status!=='active'})),
  {value:'excluded',label:'不纳入成本核算',disabled:false},
])
const subscriptionUnitOptions=computed(()=>[
  ...subscriptionUnits.value.map(x=>({value:x.id,label:`${x.name}（${x.account_count} 个账号）`})),
  {value:'new',label:'新建订阅实例'},
])
async function loadSubscriptionUnits(planID=accountForm.plan_id){subscriptionUnits.value=planID?(await costManagementAPI.subscriptionUnits(planID)):[]}
const selectedAccountPlan=computed<string|number|undefined>({
  get:()=>accountForm.cost_mode==='excluded'?'excluded':accountForm.plan_id,
  set:value=>{
    if(value==='excluded'){accountForm.cost_mode='excluded';accountForm.plan_id=accountForm.subscription_unit_id=undefined;accountForm.new_subscription_unit_name='';return}
    const planID=Number(value),plan=planChoices.value.find(x=>x.id===planID)
    accountForm.cost_mode=plan?.plan_type||'metered';accountForm.plan_id=planID;accountForm.subscription_unit_id=undefined;accountForm.new_subscription_unit_name='';accountForm.exclude_reason=''
    plan?.plan_type==='fixed'?loadSubscriptionUnits(planID):subscriptionUnits.value=[]
  },
})
function openAccount(x:AccountCostRow){batchMode.value=false;editingAccount.value=x.account_id;editingAccountConfigured.value=!!x.cost_mode;Object.assign(accountForm,{cost_mode:x.cost_mode||'metered',plan_id:x.plan_id,subscription_unit_id:x.subscription_unit_id,new_subscription_unit_name:'',effective_from:new Date().toISOString().slice(0,16),effective_to:'',exclude_reason:x.exclude_reason||''});if(x.cost_mode==='fixed'&&x.plan_id)loadSubscriptionUnits(x.plan_id);accountDialog.value=true}
function openBatch(){batchMode.value=true;editingAccount.value=undefined;subscriptionUnits.value=[];Object.assign(accountForm,{cost_mode:'metered',plan_id:undefined,subscription_unit_id:undefined,new_subscription_unit_name:'',effective_from:new Date().toISOString().slice(0,16),effective_to:'',exclude_reason:''});accountDialog.value=true}
async function saveAccountForm(){const fixed=accountForm.cost_mode==='fixed',creatingUnit=fixed&&accountForm.subscription_unit_id==='new';const input:AccountCostInput={...accountForm,plan_id:accountForm.cost_mode==='excluded'?undefined:accountForm.plan_id,subscription_unit_id:fixed&&!creatingUnit?accountForm.subscription_unit_id:undefined,new_subscription_unit_name:creatingUnit?accountForm.new_subscription_unit_name:'',exclude_reason:accountForm.cost_mode==='excluded'?accountForm.exclude_reason:'',effective_from:new Date(accountForm.effective_from).toISOString(),effective_to:accountForm.effective_to?new Date(accountForm.effective_to).toISOString():undefined};try{batchMode.value?await costManagementAPI.saveAccounts([...selectedAccounts.value],input):await costManagementAPI.saveAccount(editingAccount.value!,input);app.showSuccess('成本配置已保存');accountDialog.value=false;selectedAccounts.value=new Set();loadAccounts()}catch(e:any){app.showError(e.message||'保存失败')}}
async function endAccountCost(){if(!editingAccount.value||!confirm('确认结束当前账号的成本核算？历史成本不会改变。'))return;try{await costManagementAPI.endAccount(editingAccount.value);app.showSuccess('当前成本核算已结束');accountDialog.value=false;loadAccounts()}catch(e:any){app.showError(e.message||'结束失败')}}

const plans=ref<CostPlan[]>([]),planTotal=ref(0),planPage=ref(1),planSearch=ref(''),planType=ref(''),planDialog=ref(false),editingPlanId=ref<number>(),editingPlanEffectiveFrom=ref('')
const planTypeOptions=[{value:'',label:'全部类型'},{value:'metered',label:'按量成本'},{value:'fixed',label:'固定成本'}],createPlanTypeOptions=planTypeOptions.slice(1),fixedCategoryOptions=[{value:'coding_plan',label:'订阅制'},{value:'self_hosted',label:'本地部署'},{value:'other',label:'其他'}],billingCycleOptions=[{value:'monthly',label:'月付'},{value:'yearly',label:'年付'}]
const billingModeOptions=[{value:'token',label:'按 Token'},{value:'request',label:'按请求'},{value:'hybrid',label:'混合计价'}]
const modelOptions=ref<Array<{value:string;label:string}>>([])
async function loadModelOptions(){const r=await costManagementAPI.modelOptions({page:1,page_size:100});modelOptions.value=r.items.map(x=>({value:x.model,label:x.model}))}
const emptyPrice=()=>({upstream_model:'',billing_mode:'token',input_price_cny:'0',output_price_cny:'0',cache_write_price_cny:'0',cache_read_price_cny:'0',image_input_price_cny:'0',image_output_price_cny:'0',per_request_price_cny:'0'})
const dateTimeLocal=(value:string|Date)=>{const dateValue=value instanceof Date?value:new Date(value);return new Date(dateValue.getTime()-dateValue.getTimezoneOffset()*60000).toISOString().slice(0,16)}
function changeBillingMode(price:any,mode:unknown){price.billing_mode=mode;if(mode==='request'){price.input_price_cny=price.output_price_cny=price.cache_write_price_cny=price.cache_read_price_cny=price.image_input_price_cny=price.image_output_price_cny='0'}else if(mode==='token')price.per_request_price_cny='0'}
const planForm=reactive<any>({name:'',plan_type:'metered',fixed_category:'coding_plan',effective_from:dateTimeLocal(new Date()),effective_to:'',billing_cycle:'monthly',fixed_unit_cost_cny:'0',purchase_quantity:1,note:'',prices:[emptyPrice()]})
const planCreatesNewVersion=computed(()=>!!editingPlanId.value&&planForm.effective_from>editingPlanEffectiveFrom.value)
const planVersionChangeHint=computed(()=>planCreatesNewVersion.value?'将创建新版本，只影响此时间之后的成本':'将修正当前版本，并重新核算其生效时间之后的历史成本')
async function loadPlans(){const r=await costManagementAPI.plans({page:planPage.value,page_size:pageSize.value,search:planSearch.value,type:planType.value});plans.value=r.items;planTotal.value=r.total}
function openPlan(){editingPlanId.value=undefined;editingPlanEffectiveFrom.value='';Object.assign(planForm,{name:'',plan_type:'metered',fixed_category:'coding_plan',effective_from:dateTimeLocal(new Date()),effective_to:'',billing_cycle:'monthly',fixed_unit_cost_cny:'0',purchase_quantity:1,note:'',prices:[emptyPrice()]});planDialog.value=true}
async function editPlan(x:CostPlan){const p=await costManagementAPI.plan(x.id);editingPlanId.value=x.id;editingPlanEffectiveFrom.value=dateTimeLocal(p.effective_from);Object.assign(planForm,{...p,effective_from:editingPlanEffectiveFrom.value,effective_to:p.effective_to?dateTimeLocal(p.effective_to):'',prices:p.prices?.length?p.prices.map(y=>({...y})):[emptyPrice()]});planDialog.value=true}
function addPrice(){planForm.prices.push(emptyPrice())}
const priceCostFields=['input_price_cny','output_price_cny','cache_write_price_cny','cache_read_price_cny','image_input_price_cny','image_output_price_cny','per_request_price_cny']
function stringifyCostFields(value:any,fields:string[]){const copy={...value};fields.forEach(field=>copy[field]=String(copy[field]??0));return copy}
async function savePlan(){try{if(planForm.plan_type==='metered'){const models=planForm.prices.map((x:any)=>x.upstream_model.trim());if(models.some((x:string)=>!x)||new Set(models).size!==models.length)throw new Error('上游模型不能为空且不能重复')}const input={...stringifyCostFields(planForm,['fixed_unit_cost_cny','monthly_unit_cost_cny']),prices:planForm.prices.map((price:any)=>stringifyCostFields(price,priceCostFields)),effective_from:new Date(planForm.effective_from).toISOString(),effective_to:planForm.effective_to?new Date(planForm.effective_to).toISOString():undefined};editingPlanId.value?await costManagementAPI.updatePlan(editingPlanId.value,input):await costManagementAPI.createPlan(input);app.showSuccess('成本方案已保存');planDialog.value=false;loadPlans()}catch(e:any){app.showError(e.message||'保存失败')}}
async function disablePlan(x:CostPlan){if(!confirm(`确认停用“${x.name}”？`))return;await costManagementAPI.disablePlan(x.id);loadPlans()}
const recalcDialog=ref(false),recalcConfirm=ref(false),recalcSubmitting=ref(false),recalc=reactive({start_date:yesterday.slice(0,7)+'-01',end_date:yesterday}),recalcJobs=ref<CostJob[]>([]),recalcPage=ref(1),recalcTotal=ref(0)
const cancelRecalculationTarget=ref<CostJob>(),cancelRecalculationSubmitting=ref(false)
const latestRecalculation=computed(()=>recalcJobs.value.find(job=>job.kind==='recalculation'&&['queued','running'].includes(job.status))||recalcJobs.value.find(job=>job.kind==='recalculation'))
const hasOverlappingRecalculation=computed(()=>recalcJobs.value.some(job=>['queued','running'].includes(job.status)&&!!job.start_date&&!!job.end_date&&job.start_date.slice(0,10)<=recalc.end_date&&job.end_date.slice(0,10)>=recalc.start_date))
const jobTypeLabel=(kind:CostJob['kind'])=>kind==='incremental'?'增量核算':'历史补算'
const jobStatusLabel=(job:CostJob)=>job.status==='running'&&job.kind==='incremental'?'核算中':({queued:'排队中',running:'补算中',succeeded:'已完成',failed:'失败',cancelled:'已取消'}[job.status]||'未知状态')
const jobProgress=(job:CostJob)=>job.total_days?Math.min(100,Math.round(job.completed_days/job.total_days*100)):0
const jobProgressText=(job:CostJob)=>job.kind==='incremental'?'-':`${job.completed_days}/${job.total_days} 天（${jobProgress(job)}%）`
const jobDetail=(job:CostJob)=>job.error_message||(job.kind==='incremental'?({running:'正在聚合新使用记录',succeeded:'增量核算正常',failed:'未记录错误详情（旧任务）'}[job.status]||'-'):({queued:'等待调度（每 5 分钟执行一次）',running:'处理中（每批最多 7 天）',succeeded:'补算完成',failed:'未记录错误详情（旧任务）',cancelled:'任务已取消'}[job.status]||'-'))
let recalculationRefreshTimer:number|undefined
async function loadRecalculations(){const r=await costManagementAPI.recalculations({page:recalcPage.value,page_size:20});recalcJobs.value=r.items;recalcTotal.value=r.total;if(!recalculationRefreshTimer&&r.items.some(job=>['queued','running'].includes(job.status)))recalculationRefreshTimer=window.setTimeout(()=>{recalculationRefreshTimer=undefined;loadOverview();loadRecalculations()},5000)}
function openRecalc(){recalcDialog.value=true;loadRecalculations()}
function submitRecalc(){if(hasOverlappingRecalculation.value){app.showError('所选日期范围已有补算任务排队或运行中');return}recalcConfirm.value=true}
async function confirmRecalc(){if(recalcSubmitting.value)return;recalcSubmitting.value=true;try{await costManagementAPI.createRecalculation(recalc);app.showSuccess('补算任务已加入队列');recalcConfirm.value=false;recalcDialog.value=false;recalcPage.value=1;await loadRecalculations()}catch(e:any){app.showError(e.message||'创建补算任务失败')}finally{recalcSubmitting.value=false}}
function requestCancelRecalculation(job:CostJob){cancelRecalculationTarget.value=job}
async function confirmCancelRecalculation(){if(!cancelRecalculationTarget.value||cancelRecalculationSubmitting.value)return;cancelRecalculationSubmitting.value=true;try{await costManagementAPI.cancelRecalculation(cancelRecalculationTarget.value.id);app.showSuccess('核算任务已取消');cancelRecalculationTarget.value=undefined;await loadRecalculations()}catch(e:any){app.showError(e.message||'取消核算任务失败')}finally{cancelRecalculationSubmitting.value=false}}
watch(tab,v=>{if(v==='accounts'){loadAccounts();loadPlanChoices()}else if(v==='plans')loadPlans()})
onMounted(()=>{loadOverview();loadAnalysis();loadPlans();loadModelOptions();loadRecalculations()});onBeforeUnmount(()=>{chart.value?.destroy();if(recalculationRefreshTimer)window.clearTimeout(recalculationRefreshTimer)})
</script>
