<template>
  <BaseDialog :show="show" :title="t('admin.users.apiKeyManagement.title')" width="extra-wide" @close="emit('close')">
    <div class="space-y-3">
      <div class="grid items-end gap-3 md:grid-cols-[minmax(18rem,1fr)_auto]">
        <div>
          <label class="input-label" for="api-key-group-filter">{{ t('admin.users.apiKeyManagement.group') }}</label>
          <Select
            id="api-key-group-filter"
            v-model="groupId"
            :options="groupOptions"
            :placeholder="t('admin.users.apiKeyManagement.selectGroup')"
            searchable
          />
        </div>
        <div v-if="groupId && pagination.total > 0" class="flex flex-wrap items-center justify-end gap-2">
          <span class="mr-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.users.apiKeyManagement.total', { count: pagination.total }) }}
          </span>
          <button
            v-if="!allInGroupSelected"
            type="button"
            class="btn btn-primary"
            data-test="select-all-in-group"
            @click="selectAllInGroup"
          >
            <Icon name="checkCircle" size="sm" />
            {{ t('admin.users.apiKeyManagement.selectAll', { count: pagination.total }) }}
          </button>
          <button v-else type="button" class="btn btn-secondary" @click="clearSelection">
            <Icon name="x" size="sm" />
            {{ t('admin.users.apiKeyManagement.clearSelection') }}
          </button>
        </div>
      </div>

      <div v-if="!groupId" class="py-12 text-center text-sm text-gray-500">
        {{ t('admin.users.apiKeyManagement.selectGroupHint') }}
      </div>
      <template v-else>
        <div class="flex min-h-10 flex-wrap items-center justify-between gap-2 border-y border-gray-200 bg-gray-50 px-3 py-2 text-sm dark:border-dark-700 dark:bg-dark-900/40">
          <div class="flex items-center gap-2">
            <Icon :name="allInGroupSelected ? 'checkCircle' : 'infoCircle'" size="sm" :class="allInGroupSelected || selectedCount > 0 ? 'text-primary-600' : 'text-gray-400'" />
            <span :class="allInGroupSelected || selectedCount > 0 ? 'font-medium text-gray-900 dark:text-white' : 'text-gray-500 dark:text-gray-400'">
              {{ allInGroupSelected
                ? t('admin.users.apiKeyManagement.selectedAll', { count: pagination.total })
                : selectedCount > 0
                  ? t('admin.users.apiKeyManagement.selected', { count: selectedCount })
                  : t('admin.users.apiKeyManagement.noneSelected') }}
            </span>
          </div>
          <button
            v-if="selectedCount > 0 && !allInGroupSelected"
            type="button"
            class="text-sm font-medium text-gray-600 hover:text-gray-900 dark:text-gray-300 dark:hover:text-white"
            @click="clearSelection"
          >
            {{ t('admin.users.apiKeyManagement.clearSelection') }}
          </button>
        </div>

        <div class="h-[min(42vh,26rem)] min-h-64 overflow-hidden border border-gray-200 dark:border-dark-700">
          <DataTable
            :columns="columns"
            :data="keys"
            :loading="loading"
            row-key="id"
            :selectable="!allInGroupSelected"
            :selected-keys="selectedIds"
            @selection-change="setSelectedIds($event.map(Number))"
          >
            <template #cell-user="{ row }">
              <div class="min-w-0">
                <div class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ row.user?.email || '-' }}</div>
                <div class="truncate text-xs text-gray-500">{{ row.user?.username || '-' }}</div>
              </div>
            </template>
            <template #cell-key="{ row }"><span class="font-mono text-xs">{{ maskKey(row.key) }}</span></template>
            <template #cell-status="{ value }">
              <span :class="['badge text-xs', value === 'active' ? 'badge-success' : 'badge-danger']">
                {{ value === 'active' ? t('common.active') : t('common.inactive') }}
              </span>
            </template>
            <template #cell-concurrency_limit="{ row }">{{ row.current_concurrency || 0 }}/{{ row.concurrency_limit || '∞' }}</template>
            <template #cell-rate_limit_5h="{ value }">{{ formatLimit(value) }}</template>
            <template #cell-rate_limit_1d="{ value }">{{ formatLimit(value) }}</template>
            <template #cell-rate_limit_7d="{ value }">{{ formatLimit(value) }}</template>
          </DataTable>
        </div>
        <Pagination
          v-if="pagination.total > 0"
          :total="pagination.total"
          :page="pagination.page"
          :page-size="pagination.pageSize"
          @update:page="changePage"
          @update:page-size="changePageSize"
        />

        <section class="border-t border-gray-200 pt-3 dark:border-dark-700">
          <h4 class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.users.apiKeyManagement.changes') }}</h4>
          <div class="grid divide-y divide-gray-200 border-y border-gray-200 dark:divide-dark-700 dark:border-dark-700 md:grid-cols-3 md:divide-x md:divide-y-0">
            <div class="space-y-3 py-3 md:pr-4">
              <div class="flex items-center gap-3">
                <Toggle v-model="editRates" :aria-label="t('admin.users.apiKeyManagement.rateLimits')" />
                <label class="input-label mb-0">{{ t('admin.users.apiKeyManagement.rateLimits') }}</label>
              </div>
              <div v-if="editRates" class="grid grid-cols-3 gap-2">
                <label class="text-xs text-gray-500">5h<input v-model="rate5h" type="number" min="0" step="0.01" class="input mt-1" /></label>
                <label class="text-xs text-gray-500">1d<input v-model="rate1d" type="number" min="0" step="0.01" class="input mt-1" /></label>
                <label class="text-xs text-gray-500">7d<input v-model="rate7d" type="number" min="0" step="0.01" class="input mt-1" /></label>
              </div>
            </div>
            <div class="space-y-3 py-3 md:px-4">
              <div class="flex items-center gap-3">
                <Toggle v-model="editConcurrency" :aria-label="t('admin.users.apiKeyManagement.concurrency')" />
                <label class="input-label mb-0">{{ t('admin.users.apiKeyManagement.concurrency') }}</label>
              </div>
              <input v-if="editConcurrency" v-model="concurrency" type="number" min="0" step="1" class="input" />
            </div>
            <div class="space-y-3 py-3 md:pl-4">
              <div class="flex items-center gap-3">
                <Toggle v-model="editStatus" :aria-label="t('common.status')" />
                <label class="input-label mb-0">{{ t('common.status') }}</label>
              </div>
              <Select v-if="editStatus" v-model="status" :options="statusOptions" />
            </div>
          </div>
        </section>
        <p v-if="invalidValues" class="text-sm text-red-600 dark:text-red-400">{{ t('admin.users.apiKeyManagement.invalidValues') }}</p>
        <p v-if="selectedCount > 500 && !allInGroupSelected" class="text-sm text-red-600 dark:text-red-400">
          {{ t('admin.users.apiKeyManagement.selectionLimit') }}
        </p>
      </template>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="!canSubmit" @click="submit">
          {{ submitting ? t('admin.users.apiKeyManagement.applying') : t('admin.users.apiKeyManagement.apply') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { BatchUpdateApiKeysRequest } from '@/api/admin/apiKeys'
import type { AdminGroup, ApiKey } from '@/types'
import type { Column } from '@/components/common/types'
import { useAppStore } from '@/stores/app'
import { useTableSelection } from '@/composables/useTableSelection'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const appStore = useAppStore()
const groups = ref<AdminGroup[]>([])
const groupId = ref<number | null>(null)
const keys = ref<ApiKey[]>([])
const loading = ref(false)
const submitting = ref(false)
const allInGroupSelected = ref(false)
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })
const { selectedIds, selectedCount, setSelectedIds, clear } = useTableSelection<ApiKey>({ rows: keys, getId: row => row.id })

const editRates = ref(false)
const editConcurrency = ref(false)
const editStatus = ref(false)
const rate5h = ref<string | number>(0)
const rate1d = ref<string | number>(0)
const rate7d = ref<string | number>(0)
const concurrency = ref<string | number>(0)
const status = ref<'active' | 'inactive'>('active')

const columns: Column[] = [
  { key: 'user', label: t('admin.users.apiKeyManagement.user'), sortable: false },
  { key: 'name', label: t('admin.users.apiKeyManagement.name'), sortable: false },
  { key: 'key', label: 'API Key', sortable: false },
  { key: 'status', label: t('common.status'), sortable: false },
  { key: 'concurrency_limit', label: t('admin.users.apiKeyManagement.concurrency'), sortable: false },
  { key: 'rate_limit_5h', label: '5h', sortable: false },
  { key: 'rate_limit_1d', label: '1d', sortable: false },
  { key: 'rate_limit_7d', label: '7d', sortable: false }
]
const groupOptions = computed(() => groups.value.map(group => ({ value: group.id, label: group.name })))
const statusOptions = computed(() => [
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') }
])
const parseNonNegative = (value: string | number, integer = false) => {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed >= 0 && (!integer || Number.isInteger(parsed)) ? parsed : null
}
const parsedRates = computed(() => [rate5h.value, rate1d.value, rate7d.value].map(value => parseNonNegative(value)))
const parsedConcurrency = computed(() => parseNonNegative(concurrency.value, true))
const invalidValues = computed(() =>
  (editRates.value && parsedRates.value.some(value => value === null)) ||
  (editConcurrency.value && parsedConcurrency.value === null)
)
const canSubmit = computed(() =>
  !!groupId.value && (allInGroupSelected.value || (selectedCount.value > 0 && selectedCount.value <= 500)) &&
  (editRates.value || editConcurrency.value || editStatus.value) && !invalidValues.value && !submitting.value
)

const maskKey = (key: string) => key.length > 16 ? `${key.slice(0, 8)}…${key.slice(-6)}` : key
const formatLimit = (value: number) => value > 0 ? `$${value}` : '∞'
const clearSelection = () => { allInGroupSelected.value = false; clear() }
const selectAllInGroup = () => { clear(); allInGroupSelected.value = true }

const loadKeys = async () => {
  if (!groupId.value) return
  loading.value = true
  try {
    const result = await adminAPI.groups.getGroupApiKeys(groupId.value, pagination.page, pagination.pageSize)
    keys.value = result.items
    pagination.total = result.total
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.apiKeyManagement.loadFailed'))
  } finally {
    loading.value = false
  }
}
const changePage = (page: number) => { pagination.page = page; loadKeys() }
const changePageSize = (pageSize: number) => { pagination.pageSize = pageSize; pagination.page = 1; loadKeys() }

watch(() => props.show, async show => {
  if (!show) return
  try {
    if (groups.value.length === 0) groups.value = await adminAPI.groups.getAllIncludingInactive()
    if (groupId.value) await loadKeys()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.apiKeyManagement.loadFailed'))
  }
})
watch(groupId, () => { pagination.page = 1; clearSelection(); loadKeys() })

const submit = async () => {
  if (!canSubmit.value || !groupId.value) return
  const count = allInGroupSelected.value ? pagination.total : selectedCount.value
  if (!window.confirm(t('admin.users.apiKeyManagement.confirm', { count }))) return
  const request: BatchUpdateApiKeysRequest = {
    group_id: groupId.value,
    api_key_ids: allInGroupSelected.value ? undefined : [...selectedIds.value],
    all: allInGroupSelected.value
  }
  if (editRates.value) [request.rate_limit_5h, request.rate_limit_1d, request.rate_limit_7d] = parsedRates.value as [number, number, number]
  if (editConcurrency.value) request.concurrency_limit = parsedConcurrency.value!
  if (editStatus.value) request.status = status.value
  submitting.value = true
  try {
    const result = await adminAPI.apiKeys.batchUpdate(request)
    appStore.showSuccess(t('admin.users.apiKeyManagement.success', { count: result.affected }))
    clearSelection()
    await loadKeys()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.apiKeyManagement.failed'))
  } finally {
    submitting.value = false
  }
}
</script>
