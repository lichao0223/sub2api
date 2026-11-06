<template>
  <BaseDialog :show="show" title="组织与人员数据维护" width="extra-wide" @close="$emit('close')">
    <div class="space-y-5">
      <p class="text-sm text-gray-500">上传 devp_org 与 devp_user 的 UTF-8 SQL（每份最多20MiB）。仅提取组织、人员ID、姓名和状态，密码、电话、邮箱不会保存。中文姓名只在双方均唯一时自动匹配；重名需手动确认。</p>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <p v-if="!enabled" class="text-amber-600">后台统计已暂停。可预检文件；应用目录和确认归属前，请在系统设置开启统计任务。</p>
      <div class="grid gap-4 md:grid-cols-2">
        <label class="space-y-2 text-sm"><span class="font-medium">组织机构 SQL</span><input type="file" accept=".sql" class="block w-full" @change="selectFile($event, 'org')"></label>
        <label class="space-y-2 text-sm"><span class="font-medium">外部人员 SQL</span><input type="file" accept=".sql" class="block w-full" @change="selectFile($event, 'person')"></label>
      </div>
      <button class="btn btn-secondary" :disabled="busy || !orgFile || !personFile" @click="previewFiles">{{ busy ? '处理中…' : '预检与匹配' }}</button>
      <div v-if="preview" class="space-y-3 rounded-lg border border-primary-200 p-4 dark:border-dark-600">
        <div class="grid gap-3 text-sm sm:grid-cols-3"><span>组织 {{ preview.directory.orgs.length }} 个</span><span>人员 {{ preview.directory.people.length }} 人</span><span>平台匹配 {{ preview.matches }} 人</span><span>待确认 {{ preview.unmatched }} 人</span><span>重名 {{ preview.duplicate_names }} 组</span><span>缺组织 {{ preview.missing_orgs }} 人</span></div>
        <p class="text-xs text-gray-500">{{ preview.applied ? '曾导入相同目录。再次应用会同步新增的平台人员。' : '应用后由后台分批核算。' }} 首次按当前目录回溯历史；后续从指定生效时间开始，已清理源数据的期间不能改写。</p>
        <label v-if="state?.version" class="block text-sm">生效时间（北京时间）<input v-model="effective" class="input mt-1" type="datetime-local"></label>
        <button class="btn btn-primary" :disabled="busy || !enabled" @click="apply">确认应用目录</button>
      </div>
      <div v-if="state" class="space-y-3">
        <div class="flex flex-wrap items-center justify-between gap-3"><h3 class="font-semibold">当前目录与身份匹配 <span class="text-sm font-normal text-gray-500">版本 {{ state.version }} · {{ state.directory.people.length }} 人</span></h3><input v-model="search" class="input" placeholder="搜索平台姓名"></div>
        <div class="max-h-80 overflow-auto"><table class="table"><thead class="sticky top-0 bg-white dark:bg-dark-900"><tr><th>平台用户</th><th>目录人员 / 组织</th><th>匹配依据</th><th>确认归属</th></tr></thead><tbody>
          <tr v-for="i in visibleIdentities" :key="i.user_id"><td>{{ i.username }} <span class="text-gray-400">#{{ i.user_id }}</span></td><td>{{ personLabel(i.person_id) }}</td><td>{{ basisLabel(i.basis) }}</td><td><button class="text-primary-600" :disabled="!enabled || busy || !state.version" @click="mappingUser=i.user_id;mappingPerson=i.person_id">选择人员</button></td></tr>
        </tbody></table><p v-if="!visibleIdentities.length" class="p-6 text-center text-gray-400">暂无匹配记录</p></div>
        <p class="text-xs text-gray-500">显示前100条搜索结果。手动确认用于修复未归属的保留明细；已有组织归属的变更从确认时间开始生效。</p>
        <div v-if="mappingUser" class="flex flex-wrap items-end gap-3 rounded border p-3 dark:border-dark-600"><label class="min-w-72 flex-1 text-sm">为平台用户 #{{ mappingUser }} 选择目录人员<Select v-model="mappingPerson" class="mt-1" :options="personOptions" searchable /></label><button class="btn btn-primary" :disabled="busy || !mappingPerson || !enabled" @click="saveMapping">确认匹配</button><button class="btn btn-secondary" @click="mappingUser=0">取消</button></div>
        <details><summary class="cursor-pointer text-sm text-gray-500">最近导入记录</summary><div v-for="item in state.imports" :key="item.id" class="mt-2 text-xs text-gray-500">{{ new Date(item.created_at).toLocaleString() }} · {{ item.applied_at ? '已应用' : '预检' }}</div></details>
      </div>
    </div>
    <template #footer><button class="btn btn-secondary" @click="$emit('close')">关闭</button></template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import { dataAnalyticsAPI, type AnalyticsDirectoryState, type AnalyticsPreview } from '@/api/admin/dataAnalytics'
import { useAppStore } from '@/stores/app'
const props = defineProps<{ show: boolean; enabled: boolean }>()
const emit = defineEmits<{ close: []; changed: [] }>()
const app = useAppStore()
const state = ref<AnalyticsDirectoryState>(), preview = ref<AnalyticsPreview>(), orgFile = ref<File>(), personFile = ref<File>()
const busy = ref(false), error = ref(''), search = ref(''), effective = ref(''), mappingUser = ref(0), mappingPerson = ref('')
const orgNames = computed(() => new Map<string,string>((state.value?.directory.orgs || []).map(o => [o.id, o.names.join(' / ')])))
const peopleByID = computed(() => new Map((state.value?.directory.people || []).map(p => [p.id,p])))
const personLabel = (id: string) => { const p = peopleByID.value.get(id); return p ? `${p.name} · ${orgNames.value.get(p.org) || '缺少组织'} · ${p.id}` : '待确认归属' }
const personOptions = computed(() => (state.value?.directory.people || []).map(p => ({ value: p.id, label: personLabel(p.id) })))
const visibleIdentities = computed(() => (state.value?.identities || []).filter(i => i.username.includes(search.value)).sort((a,b) => Number(!!a.person_id)-Number(!!b.person_id)).slice(0,100))
const basisLabels: Record<string,string> = { manual: '人工确认', unique_name: '唯一姓名', external_id: '外部稳定ID', conflict: '绑定冲突' }
const basisLabel = (basis: string) => basisLabels[basis] || '待确认'
async function load() { try { state.value = await dataAnalyticsAPI.directory() } catch(e) { error.value = e instanceof Error ? e.message : '读取目录失败' } }
watch(() => props.show, show => { if (show) { error.value=''; effective.value=new Date(Date.now()+8*3600000).toISOString().slice(0,16); void load() } })
function selectFile(event: Event, kind: string) {
  const file = (event.target as HTMLInputElement).files?.[0]; preview.value=undefined; if(kind==='org')orgFile.value=undefined;else personFile.value=undefined
  if (file && (file.size > 20*1024*1024 || !file.name.toLowerCase().endsWith('.sql'))) { error.value='请选择20MiB以内的.sql文件'; return }
  if (kind==='org') orgFile.value=file; else personFile.value=file
}
async function previewFiles() {
  if (!orgFile.value || !personFile.value) return
  busy.value=true; error.value=''
  try { preview.value=await dataAnalyticsAPI.preview(orgFile.value,personFile.value) }
  catch(e) { error.value=e instanceof Error ? e.message : 'SQL预检失败' } finally { busy.value=false }
}
async function apply() {
  if (!preview.value || !state.value) return
  busy.value=true; error.value=''
  try { await dataAnalyticsAPI.apply(preview.value.id,new Date(`${effective.value}:00+08:00`).toISOString(),state.value.version); preview.value=undefined; await load(); emit('changed'); app.showSuccess('目录已应用，正在分批核算') }
  catch(e) { error.value=e instanceof Error ? e.message : '应用失败'; await load() } finally { busy.value=false }
}
async function saveMapping() {
  if (!state.value) return
  busy.value=true; error.value=''
  try { await dataAnalyticsAPI.map(mappingUser.value,mappingPerson.value,state.value.version); mappingUser.value=0; await load(); emit('changed'); app.showSuccess('身份归属已保存') }
  catch(e) { error.value=e instanceof Error ? e.message : '匹配失败'; await load() } finally { busy.value=false }
}
</script>
