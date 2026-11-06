<template>
  <BaseDialog :show="!!person" :title="`${person?.name || ''} · 个人使用详情`" width="full" placement="right" @close="$emit('close')">
    <div class="analytics-person">
      <div class="analytics-person-hero"><span class="analytics-person-avatar">{{ person?.name.slice(0,1) }}</span><div><h2>{{ person?.name }}</h2><p>{{ person?.org_names.slice(0,3).join(' / ') || '待确认归属' }}</p><p>平台用户 #{{ person?.user_id }}<span v-if="person?.person_id"> · 外部人员 {{ person.person_id }}</span></p></div></div>
      <div class="analytics-person-scope">{{ rangeLabel }} · {{ query?.org_id ? '当前部门范围' : '全部组织' }}<span v-if="detail?.coverage.provisional"> · 本月为暂估</span></div>
      <p v-if="loading" class="analytics-person-loading" role="status">正在加载个人用量与成本…</p>
      <div v-else-if="error" role="alert" class="analytics-alert error">{{ error }}<button type="button" class="analytics-text-button" @click="loadDetail">重新加载</button></div>
      <template v-else-if="detail">
        <div class="analytics-person-summary">
          <article class="analytics-panel"><span>所选月份 Token 总数</span><strong>{{ tokenText(detail.summary.tokens) }}</strong><small>含不计成本模型的完整用量</small></article>
          <article class="analytics-panel"><span>所选月份实际成本</span><strong>{{ moneyText(detail.summary.cost) }}</strong><small>按量成本＋固定成本分摊</small></article>
        </div>
        <section class="analytics-token-breakdown" aria-label="个人Token组成">
          <div v-for="item in tokenParts" :key="item.label"><span>{{ item.label }}</span><strong>{{ tokenText(item.value) }}</strong></div>
        </section>
        <h3>个人成本组成</h3>
        <section class="analytics-person-costs">
          <div><span>按量成本</span><strong>{{ moneyText(detail.summary.metered) }}</strong><small>按量模型使用产生的采购成本</small></div>
          <div><span>固定成本分摊</span><strong>{{ moneyText(detail.summary.fixed) }}</strong><small>当月用过该订阅的规范用户均摊</small></div>
          <div><span>不计成本模型</span><strong>¥0.00</strong><small>用量 {{ tokenText(detail.summary.excluded_tokens) }} · 已包含在总 Token</small></div>
        </section>

        <div class="analytics-model-heading"><h3>模型用量与成本明细</h3><Select v-model="modelMonth" class="analytics-model-period" aria-label="个人模型月份" :options="modelMonthOptions" :disabled="modelLoading" @change="loadModelMonth" /></div>
        <p class="analytics-muted">包含全部模型，按模型、采购来源和历史组织分别列出；缓存同时展示写入和读取。</p>
        <p v-if="modelError" role="alert" class="analytics-alert error">{{ modelError }}<button type="button" class="analytics-text-button" @click="loadModelMonth">重试</button></p>
        <div v-if="modelLoading" class="analytics-person-loading" role="status">正在加载所选月份的模型明细…</div>
        <template v-else-if="models">
          <div class="analytics-table-scroll">
            <table class="analytics-table analytics-model-table" aria-label="个人模型用量与成本组成">
              <thead><tr><th>模型 / 成本归属</th><th class="num">输入</th><th class="num">输出</th><th class="num">缓存</th><th class="num">Token 合计</th><th class="num">按量成本</th><th class="num">固定分摊</th><th class="num">总成本</th></tr></thead>
              <tbody><tr v-for="(row,index) in visibleModels" :key="index" :class="{ 'analytics-excluded-model':row.mode==='excluded' }">
                <td><strong>{{ row.model }}</strong><span class="analytics-subcell"><span class="analytics-pill" :class="{muted:row.mode==='excluded'}">{{ modeLabel(row.mode) }}</span> {{ row.source }}</span><small class="analytics-subcell">{{ row.org_names.slice(0,3).join(' / ') || '待确认归属' }}</small><small v-if="row.issue" class="analytics-model-issue">{{ issueLabel(row.issue) }}</small></td>
                <td class="num">{{ tokenText(row.input) }}</td><td class="num">{{ tokenText(row.output) }}</td><td class="num">{{ tokenText(sumTokens(row.cache_write,row.cache_read)) }}<small class="analytics-subcell">写 {{ tokenText(row.cache_write) }}<br>读 {{ tokenText(row.cache_read) }}</small></td><td class="num analytics-total">{{ tokenText(row.tokens) }}</td><td class="num">{{ moneyText(row.metered) }}</td><td class="num">{{ moneyText(row.fixed) }}</td><td class="num analytics-total">{{ moneyText(row.cost) }}</td>
              </tr><tr v-if="!models.models.length"><td colspan="8" class="analytics-table-empty">该期间没有模型使用记录</td></tr></tbody>
              <tfoot><tr><td>合计</td><td class="num">{{ tokenText(models.summary.input) }}</td><td class="num">{{ tokenText(models.summary.output) }}</td><td class="num">{{ tokenText(sumTokens(models.summary.cache_write,models.summary.cache_read)) }}</td><td class="num">{{ tokenText(models.summary.tokens) }}</td><td class="num">{{ moneyText(models.summary.metered) }}</td><td class="num">{{ moneyText(models.summary.fixed) }}</td><td class="num">{{ moneyText(models.summary.cost) }}</td></tr></tfoot>
            </table>
          </div>
          <details class="analytics-cost-details"><summary>查看输入、输出、缓存、图片及请求成本组成</summary>
            <div class="analytics-table-scroll"><table class="analytics-table analytics-model-table" aria-label="模型采购成本组成"><thead><tr><th>模型 / 来源</th><th class="num">输入成本</th><th class="num">输出成本</th><th class="num">缓存写入</th><th class="num">缓存读取</th><th class="num">图片成本</th><th class="num">请求成本</th><th class="num">固定分摊</th><th class="num">总成本</th></tr></thead><tbody><tr v-for="(row,index) in visibleModels" :key="index"><td>{{ row.model }}<small class="analytics-subcell">{{ row.source }}</small></td><td class="num">{{ moneyText(row.input_cost) }}</td><td class="num">{{ moneyText(row.output_cost) }}</td><td class="num">{{ moneyText(row.write_cost) }}</td><td class="num">{{ moneyText(row.read_cost) }}</td><td class="num">{{ moneyText(row.image_cost) }}</td><td class="num">{{ moneyText(row.request_cost) }}</td><td class="num">{{ moneyText(row.fixed) }}</td><td class="num">{{ moneyText(row.cost) }}</td></tr></tbody></table></div>
          </details>
          <Pagination v-if="models.models.length>modelPageSize" :page="modelPage" :page-size="modelPageSize" :total="models.models.length" @update:page="modelPage=$event" @update:pageSize="modelPageSize=$event;modelPage=1" />
        </template>
        <h3>固定成本分摊依据</h3>
        <p class="analytics-muted">实例当月实际计提成本 ÷ 当月去重使用人数；同一人使用同一订阅的多个账号只算一份。这里展示个人整月、所有部门的唯一份额，模型表只含当前部门范围。</p>
        <div class="analytics-table-scroll"><table class="analytics-table" aria-label="个人固定成本分摊依据"><thead><tr><th>月份 / 订阅实例</th><th class="num">实例已发生成本</th><th class="num">使用人数</th><th class="num">个人整月份额</th></tr></thead><tbody><tr v-for="share in detail.fixed_shares" :key="`${share.month}/${share.unit_id}`"><td>{{ share.month.slice(0,7) }} · {{ share.name }}</td><td class="num">{{ moneyText(share.total) }}</td><td class="num">{{ share.participants }}</td><td class="num analytics-total">{{ moneyText(share.amount) }}</td></tr><tr v-if="!detail.fixed_shares.length"><td colspan="4" class="analytics-table-empty">没有固定订阅使用</td></tr></tbody></table></div>

        <h3>{{ query?.year }} 年逐月明细</h3>
        <p class="analytics-muted">高亮为顶部所选月份，并使用同一发布版本；其他月份取当前最新发布结果。未来月份不展示，尚未发布的月份不视为零用量。</p>
        <div class="analytics-table-scroll"><table class="analytics-table analytics-model-table" aria-label="个人全年逐月用量与成本"><thead><tr><th>月份</th><th class="num">输入</th><th class="num">输出</th><th class="num">缓存</th><th class="num">Token 合计</th><th class="num">按量成本</th><th class="num">固定分摊</th><th class="num">总成本</th></tr></thead><tbody>
          <tr v-for="row in detail.person_months || []" :key="row.month" :class="{'analytics-selected-month': selectedMonths.has(row.month)}"><td>{{ Number(row.month.slice(5,7)) }} 月<small v-if="!row.generation" class="analytics-subcell">尚未发布</small></td><template v-if="row.generation"><td class="num">{{ tokenText(row.input) }}</td><td class="num">{{ tokenText(row.output) }}</td><td class="num">{{ tokenText(sumTokens(row.cache_write,row.cache_read)) }}</td><td class="num">{{ tokenText(row.tokens) }}</td><td class="num">{{ moneyText(row.metered) }}</td><td class="num">{{ moneyText(row.fixed) }}</td><td class="num">{{ moneyText(row.cost) }}</td></template><td v-else colspan="7" class="analytics-muted">—</td></tr>
          <tr v-if="!detail.person_months?.length"><td colspan="8" class="analytics-table-empty">暂无已发布的逐月数据</td></tr>
        </tbody><tfoot><tr><td>所选月份合计</td><td class="num">{{ tokenText(detail.summary.input) }}</td><td class="num">{{ tokenText(detail.summary.output) }}</td><td class="num">{{ tokenText(sumTokens(detail.summary.cache_write,detail.summary.cache_read)) }}</td><td class="num">{{ tokenText(detail.summary.tokens) }}</td><td class="num">{{ moneyText(detail.summary.metered) }}</td><td class="num">{{ moneyText(detail.summary.fixed) }}</td><td class="num">{{ moneyText(detail.summary.cost) }}</td></tr><tr v-if="detail.person_months?.length"><td>全年已发布合计</td><td class="num">{{ tokenText(annualTotal('input')) }}</td><td class="num">{{ tokenText(annualTotal('output')) }}</td><td class="num">{{ tokenText(sumTokens(annualTotal('cache_write'),annualTotal('cache_read'))) }}</td><td class="num">{{ tokenText(annualTotal('tokens')) }}</td><td class="num">{{ moneyText(annualTotal('metered')) }}</td><td class="num">{{ moneyText(annualTotal('fixed')) }}</td><td class="num">{{ moneyText(annualTotal('cost')) }}</td></tr></tfoot></table></div>
        <p class="analytics-muted analytics-person-footnote">Token＝输入＋输出＋缓存写入＋缓存读取。统一使用 B / M / K，汇总按原始整数计算；金额按精确小数字符串展示。价格待核算的请求不被当作已完成的零成本。</p>
      </template>
    </div>
    <template #footer><button type="button" class="btn btn-secondary" @click="$emit('close')">关闭详情</button></template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Pagination from '@/components/common/Pagination.vue'
import { dataAnalyticsAPI, type AnalyticsQuery, type AnalyticsReport, type AnalyticsRow, type AnalyticsValues } from '@/api/admin/dataAnalytics'
import { moneyText, tokenText, sumDecimalStrings } from './format'

const props = defineProps<{ person?: AnalyticsRow; query?: AnalyticsQuery; versions: Record<string,number>; rangeLabel: string }>()
defineEmits<{ close: [] }>()
const detail = shallowRef<AnalyticsReport>(), models = shallowRef<AnalyticsReport>()
const loading = ref(false), modelLoading = ref(false), error = ref(''), modelError = ref(''), modelMonth = ref('')
const modelPage = ref(1), modelPageSize = ref(20)
const visibleModels = computed(()=>(models.value?.models || []).slice((modelPage.value-1)*modelPageSize.value,modelPage.value*modelPageSize.value))
let detailAbort: AbortController | undefined, modelAbort: AbortController | undefined
let snapshot: AnalyticsQuery | undefined
const selectedMonths = computed(()=>new Set(detail.value?.period.keys || []))
const modelMonthOptions = computed(()=>[{value:'',label:'所选月份合计'},...(detail.value?.period.months || []).map(month=>({value:String(month),label:`${month} 月模型明细`}))])
const tokenParts = computed(()=>[
  {label:'输入 Token',value:detail.value?.summary.input},{label:'输出 Token',value:detail.value?.summary.output},
  {label:'缓存写入',value:detail.value?.summary.cache_write},{label:'缓存读取',value:detail.value?.summary.cache_read},
])
const modeLabels:Record<string,string> = {fixed:'固定成本',metered:'按量成本',excluded:'不计成本'}
const issueLabels:Record<string,string> = {missing_account_config:'待配置账号成本',missing_subscription:'订阅实例不在有效期',missing_subscription_version:'缺少订阅价格版本',missing_plan_version:'缺少生效价格版本',missing_model_price:'缺少模型成本价格'}
const modeLabel = (value:string)=>modeLabels[value] || value
const issueLabel = (value:string)=>issueLabels[value] || value
const sumTokens = (...values:string[])=>values.reduce((sum,value)=>sum+BigInt(value),0n).toString()
function annualTotal(key:keyof AnalyticsValues){return sumDecimalStrings((detail.value?.person_months || []).filter(row=>row.generation>0).map(row=>row[key]))}
watch(()=>props.person,person=>{
  detailAbort?.abort();modelAbort?.abort();snapshot=undefined;detail.value=undefined;models.value=undefined;modelMonth.value='';modelPage.value=1;loading.value=false;modelLoading.value=false;error.value='';modelError.value=''
  if(person&&props.query){snapshot={...props.query,search:'',page:1,user_id:person.user_id,versions:JSON.stringify(props.versions)};void loadDetail()}
},{immediate:true})
async function loadDetail(){
  if(!snapshot)return
  detailAbort?.abort()
  const controller=new AbortController();detailAbort=controller;loading.value=true;error.value=''
  try{const data=await dataAnalyticsAPI.report(snapshot,controller.signal);if(!controller.signal.aborted){detail.value=data;models.value=data;modelMonth.value=''}}
  catch(cause){if(!controller.signal.aborted)error.value=cause instanceof Error?cause.message:'明细加载失败，请刷新报表后重试'}
  finally{if(!controller.signal.aborted)loading.value=false}
}
async function loadModelMonth(){
  modelAbort?.abort();modelError.value='';modelPage.value=1
  if(!modelMonth.value){models.value=detail.value;modelLoading.value=false;return}
  if(!snapshot||!detail.value)return
  const key=`${snapshot.year}-${modelMonth.value.padStart(2,'0')}-01`
  const generation=detail.value.versions[key]
  if(generation===undefined){modelError.value='该月份不在所选统计范围，请重新打开详情';return}
  const controller=new AbortController();modelAbort=controller;modelLoading.value=true;models.value=undefined
  try{const data=await dataAnalyticsAPI.report({...snapshot,mode:'month',months:modelMonth.value,versions:JSON.stringify({[key]:generation})},controller.signal);if(!controller.signal.aborted)models.value=data}
  catch(cause){if(!controller.signal.aborted)modelError.value=cause instanceof Error?cause.message:'月份明细加载失败，请刷新报表后重试'}
  finally{if(!controller.signal.aborted)modelLoading.value=false}
}
onBeforeUnmount(()=>{detailAbort?.abort();modelAbort?.abort()})
</script>
