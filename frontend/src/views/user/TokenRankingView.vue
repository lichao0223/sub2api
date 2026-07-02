<template>
  <AppLayout>
    <div class="flex h-[calc(100vh-8rem)] min-h-0 flex-col gap-4 overflow-hidden">
      <div class="card p-4">
        <div class="flex flex-wrap items-center gap-4">
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('dashboard.timeRange') }}:
            </span>
            <DateRangePicker
              v-model:start-date="startDate"
              v-model:end-date="endDate"
              @change="handleFilterChange"
            />
          </div>
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('tokenRanking.scope') }}:
            </span>
            <Select
              v-model="rankingScope"
              :options="rankingScopeOptions"
              class="w-32"
              @change="handleFilterChange"
            />
          </div>
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('tokenRanking.rankBy') }}:
            </span>
            <Select
              v-model="rankBy"
              :options="rankByOptions"
              class="w-56"
              @change="handleFilterChange"
            />
          </div>
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('tokenRanking.sortOrder') }}:
            </span>
            <Select
              v-model="sortOrder"
              :options="sortOrderOptions"
              class="w-28"
              @change="handleFilterChange"
            />
          </div>
          <div class="ml-auto flex items-center gap-2">
            <button class="btn btn-secondary" :disabled="loading" @click="loadRanking">
              {{ t('common.refresh') }}
            </button>
            <div ref="exportMenuRef" class="relative">
              <button
                type="button"
                class="btn btn-secondary flex items-center gap-2"
                :disabled="loading || exporting || !rankingItems.length"
                @click.stop="exportMenuOpen = !exportMenuOpen"
              >
                <Icon name="download" size="sm" />
                {{ exporting ? t('tokenRanking.exporting') : t('tokenRanking.exportRanking') }}
                <Icon name="chevronDown" size="xs" />
              </button>
              <div
                v-if="exportMenuOpen"
                class="absolute right-0 z-20 mt-2 w-36 overflow-hidden rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-700 dark:bg-dark-900"
              >
                <button
                  type="button"
                  class="block w-full px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-dark-800"
                  @click="exportRanking('xlsx')"
                >
                  {{ t('tokenRanking.exportExcel') }}
                </button>
                <button
                  type="button"
                  class="block w-full px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-dark-800"
                  @click="exportRanking('csv')"
                >
                  {{ t('tokenRanking.exportCsv') }}
                </button>
              </div>
            </div>
            <button
              v-if="authStore.isAdmin"
              type="button"
              class="btn btn-secondary"
              :disabled="importing"
              @click="openImportDialog"
            >
              {{ t('tokenRanking.importExternalData') }}
            </button>
            <button
              v-if="authStore.isAdmin"
              type="button"
              class="btn btn-secondary"
              :disabled="exportingExternal"
              @click="openExternalExportDialog"
            >
              {{ exportingExternal ? t('tokenRanking.exporting') : t('tokenRanking.exportImportData') }}
            </button>
          </div>
        </div>
      </div>

      <div v-if="loading" class="flex items-center justify-center py-16">
        <LoadingSpinner />
      </div>

      <template v-else>
        <div v-if="error" class="card p-6 text-sm text-red-600 dark:text-red-400">
          {{ t('tokenRanking.failedToLoad') }}
        </div>

        <div v-else-if="!rankingItems.length" class="card flex min-h-0 flex-1 items-center justify-center p-6 text-sm text-gray-500 dark:text-gray-400">
          {{ t('tokenRanking.noData') }}
        </div>

        <template v-else>
          <div
            v-if="statsCoverage && !statsCoverage.complete"
            class="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-200"
          >
            {{ t('tokenRanking.statsIncomplete', { aggregated: statsCoverage.aggregated_days, total: statsCoverage.total_days }) }}
            <template v-if="statsCoverageMissingSummary">
              · {{ t('tokenRanking.statsMissing', { ranges: statsCoverageMissingSummary }) }}
            </template>
          </div>

          <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-5">
            <div class="card p-4">
              <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('tokenRanking.totalTokens') }}</div>
              <div class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ formatTokens(totals.totalTokens) }}</div>
            </div>
            <div class="card p-4">
              <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('tokenRanking.totalNonworkTokens') }}</div>
              <div class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ formatTokens(totals.nonworkTokens) }}</div>
            </div>
            <div class="card p-4">
              <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('tokenRanking.totalRequests') }}</div>
              <div class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ formatNumber(totals.requests) }}</div>
            </div>
            <div class="card p-4">
              <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('tokenRanking.totalSpend') }}</div>
              <div class="mt-2 text-2xl font-bold text-emerald-600 dark:text-emerald-400">${{ formatCost(totals.actualCost) }}</div>
            </div>
            <div class="card p-4">
              <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('tokenRanking.nonworkTokenRatio') }}</div>
              <div class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ formatPercent(totals.nonworkTokenRatio) }}</div>
            </div>
          </div>

          <div class="card flex min-h-0 flex-1 flex-col overflow-hidden">
            <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700">
              <div class="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
                <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('tokenRanking.rankingList') }}
                </h2>
                <span class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
                  <Icon name="calendar" size="sm" />
                  {{ responseRange }}
                  <template v-if="calendarConfirmed === false">
                    · {{ t('tokenRanking.calendarPredicted') }}
                  </template>
                  <template v-if="lastStatsComputedAt">
                    · {{ t('tokenRanking.lastStatsComputedAt', { time: lastStatsComputedAt }) }}
                  </template>
                </span>
              </div>
            </div>
            <div ref="rankingTableScrollRef" class="token-ranking-table-scroll min-h-0 flex-1 overflow-auto">
              <table class="w-full min-w-[1120px] table-fixed text-sm">
                <colgroup>
                  <col class="w-20" />
                  <col class="w-40" />
                  <col class="w-28" />
                  <col class="w-32" />
                  <col class="w-40" />
                  <col class="w-44" />
                  <col class="w-44" />
                  <col class="w-28" />
                </colgroup>
                <thead class="sticky top-0 z-10 bg-gray-50 text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                  <tr>
                    <th class="px-4 py-3 text-left">{{ t('tokenRanking.rank') }}</th>
                    <th class="px-4 py-3 text-left">{{ t('tokenRanking.user') }}</th>
                    <th class="px-4 py-3 text-right">{{ t('tokenRanking.requests') }}</th>
                    <th class="px-4 py-3 text-right">{{ t('tokenRanking.tokens') }}</th>
                    <th class="px-4 py-3 text-right">{{ t('tokenRanking.nonworkTokens') }}</th>
                    <th class="px-4 py-3 text-right whitespace-nowrap">{{ t('tokenRanking.activeDuration') }}</th>
                    <th class="px-4 py-3 text-right whitespace-nowrap">{{ t('tokenRanking.nonworkActiveDuration') }}</th>
                    <th class="px-4 py-3 text-right">{{ t('tokenRanking.spend') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="(item, index) in paginatedRankingItems"
                    :key="`${item.user_id}-${index}`"
                    class="border-t border-gray-100 dark:border-dark-700"
                  >
                    <td class="px-4 py-3 font-semibold text-gray-500 dark:text-gray-400">#{{ paginationStart + index + 1 }}</td>
                    <td class="px-4 py-3">
                      <div class="max-w-[140px] truncate font-medium text-gray-900 dark:text-white" :title="userLabel(item)">
                        {{ userLabel(item) }}
                      </div>
                    </td>
                    <td class="px-4 py-3 text-right text-gray-700 dark:text-gray-300">{{ formatNumber(item.requests) }}</td>
                    <td class="px-4 py-3 text-right font-semibold text-gray-900 dark:text-white">{{ formatTokens(item.tokens) }}</td>
                    <td class="px-4 py-3 text-right text-gray-700 dark:text-gray-300">{{ formatTokens(item.nonwork_tokens ?? 0) }}</td>
                    <td class="px-4 py-3 text-right whitespace-nowrap text-gray-700 dark:text-gray-300">{{ formatDuration(item.active_duration_ms || 0) }}</td>
                    <td class="px-4 py-3 text-right whitespace-nowrap text-gray-700 dark:text-gray-300">{{ formatDuration(item.nonwork_active_ms || 0) }}</td>
                    <td class="px-4 py-3 text-right text-emerald-600 dark:text-emerald-400">${{ formatCost(item.actual_cost) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <Pagination
              v-if="rankingItems.length > 0"
              :page="pagination.page"
              :total="rankingItems.length"
              :page-size="pagination.page_size"
              @update:page="handlePageChange"
              @update:pageSize="handlePageSizeChange"
            />
          </div>
        </template>
      </template>
    </div>

    <div v-if="importDialogOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div class="flex max-h-[86vh] w-full max-w-5xl flex-col overflow-hidden rounded-lg bg-white shadow-xl dark:bg-dark-900">
        <div class="flex items-center justify-between border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('tokenRanking.importExternalData') }}</h3>
          <button class="btn btn-ghost" type="button" @click="closeImportDialog">{{ t('common.close') }}</button>
        </div>
        <div class="flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-5">
          <div class="flex flex-wrap items-center gap-3">
            <input ref="importFileInputRef" type="file" accept=".xlsx,.xls,.csv" class="hidden" @change="handleImportFileChange" />
            <button class="btn btn-secondary" type="button" @click="importFileInputRef?.click()">{{ t('tokenRanking.selectExcel') }}</button>
            <button class="btn btn-secondary" type="button" @click="downloadImportTemplate">{{ t('tokenRanking.downloadTemplate') }}</button>
            <span class="text-sm text-gray-500 dark:text-gray-400">{{ importFileName || t('tokenRanking.noFileSelected') }}</span>
          </div>
          <textarea
            v-model="importNote"
            class="input min-h-[72px] resize-y"
            :placeholder="t('tokenRanking.importNotePlaceholder')"
          />
          <div v-if="importPreview" class="grid grid-cols-2 gap-3 text-sm md:grid-cols-6">
            <div class="rounded border border-gray-200 p-3 dark:border-dark-700">{{ t('tokenRanking.totalRows') }}: {{ importPreview.summary.total_rows }}</div>
            <div class="rounded border border-gray-200 p-3 dark:border-dark-700">{{ t('tokenRanking.matchedRows') }}: {{ importPreview.summary.matched_rows }}</div>
            <div class="rounded border border-gray-200 p-3 dark:border-dark-700">{{ t('tokenRanking.overwriteRows') }}: {{ importPreview.summary.overwritten_rows }}</div>
            <div class="rounded border border-gray-200 p-3 dark:border-dark-700">{{ t('tokenRanking.unmatchedRows') }}: {{ importPreview.summary.unmatched_rows }}</div>
            <div class="rounded border border-gray-200 p-3 dark:border-dark-700">{{ t('tokenRanking.conflictRows') }}: {{ importPreview.summary.conflict_rows }}</div>
            <div class="rounded border border-gray-200 p-3 dark:border-dark-700">{{ t('tokenRanking.invalidRows') }}: {{ importPreview.summary.invalid_rows }}</div>
          </div>
          <div v-if="importErrors.length" class="rounded border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-200">
            <div v-for="errorItem in importErrors.slice(0, 8)" :key="errorItem">{{ errorItem }}</div>
          </div>
          <div v-if="importPreview" class="min-h-0 overflow-auto rounded border border-gray-200 dark:border-dark-700">
            <table class="w-full min-w-[900px] text-sm">
              <thead class="sticky top-0 bg-gray-50 text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                <tr>
                  <th class="px-3 py-2 text-left">{{ t('tokenRanking.excelRow') }}</th>
                  <th class="px-3 py-2 text-left">{{ t('tokenRanking.date') }}</th>
                  <th class="px-3 py-2 text-left">{{ t('tokenRanking.user') }}</th>
                  <th class="px-3 py-2 text-right">{{ t('tokenRanking.requests') }}</th>
                  <th class="px-3 py-2 text-right">{{ t('tokenRanking.tokens') }}</th>
                  <th class="px-3 py-2 text-left">{{ t('tokenRanking.importStatus') }}</th>
                  <th class="px-3 py-2 text-left">{{ t('tokenRanking.importError') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in importPreview.rows.slice(0, 100)" :key="row.row_number" class="border-t border-gray-100 dark:border-dark-700">
                  <td class="px-3 py-2">{{ row.row_number }}</td>
                  <td class="px-3 py-2">{{ row.date }}</td>
                  <td class="px-3 py-2">{{ row.username }}</td>
                  <td class="px-3 py-2 text-right">{{ formatNumber(row.requests || 0) }}</td>
                  <td class="px-3 py-2 text-right">{{ formatTokens(row.total_tokens || 0) }}</td>
                  <td class="px-3 py-2">{{ t(`tokenRanking.status_${row.status}`) }}</td>
                  <td class="px-3 py-2 text-red-600 dark:text-red-400">{{ row.errors?.map((item) => item.message).join('；') }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        <div class="flex justify-end gap-2 border-t border-gray-100 px-5 py-4 dark:border-dark-700">
          <button class="btn btn-secondary" type="button" @click="closeImportDialog">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" type="button" :disabled="!canConfirmImport || importing" @click="confirmExternalImport">
            {{ importing ? t('common.loading') : t('tokenRanking.confirmImport') }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="externalExportDialogOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div class="w-full max-w-lg rounded-lg bg-white p-5 shadow-xl dark:bg-dark-900">
        <div class="mb-4 flex items-center justify-between">
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('tokenRanking.exportImportData') }}</h3>
          <button class="btn btn-ghost" type="button" @click="externalExportDialogOpen = false">{{ t('common.close') }}</button>
        </div>
        <div class="space-y-4">
          <DateRangePicker v-model:start-date="externalExportStartDate" v-model:end-date="externalExportEndDate" />
          <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input v-model="externalExportIncludeNonwork" type="checkbox" />
            {{ t('tokenRanking.includeNonworkColumns') }}
          </label>
        </div>
        <div class="mt-5 flex justify-end gap-2">
          <button class="btn btn-secondary" type="button" @click="externalExportDialogOpen = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" type="button" :disabled="exportingExternal" @click="exportExternalImportData">{{ t('tokenRanking.exportImportData') }}</button>
        </div>
      </div>
    </div>

    <div v-if="importBatchesDialogOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div class="flex max-h-[80vh] w-full max-w-4xl flex-col overflow-hidden rounded-lg bg-white shadow-xl dark:bg-dark-900">
        <div class="flex items-center justify-between border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('tokenRanking.importRecords') }}</h3>
          <button class="btn btn-ghost" type="button" @click="importBatchesDialogOpen = false">{{ t('common.close') }}</button>
        </div>
        <div class="min-h-0 flex-1 overflow-auto p-5">
          <table class="w-full min-w-[760px] text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400">
              <tr>
                <th class="px-3 py-2 text-left">ID</th>
                <th class="px-3 py-2 text-left">{{ t('tokenRanking.fileName') }}</th>
                <th class="px-3 py-2 text-right">{{ t('tokenRanking.importedRows') }}</th>
                <th class="px-3 py-2 text-left">{{ t('tokenRanking.importedAt') }}</th>
                <th class="px-3 py-2 text-left">{{ t('tokenRanking.importStatus') }}</th>
                <th class="px-3 py-2 text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="batch in importBatches" :key="batch.id" class="border-t border-gray-100 dark:border-dark-700">
                <td class="px-3 py-2">{{ batch.id }}</td>
                <td class="px-3 py-2">{{ batch.file_name }}</td>
                <td class="px-3 py-2 text-right">{{ batch.imported_rows }}</td>
                <td class="px-3 py-2">{{ batch.imported_at ? formatDateTime(batch.imported_at) : '-' }}</td>
                <td class="px-3 py-2">{{ batch.status }}</td>
                <td class="px-3 py-2 text-right">
                  <button class="btn btn-danger btn-sm" type="button" :disabled="batch.status !== 'imported'" @click="voidImportBatch(batch.id)">
                    {{ t('tokenRanking.voidBatch') }}
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
          <div v-if="!importBatches.length" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('tokenRanking.noImportRecords') }}</div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import AppLayout from '@/components/layout/AppLayout.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { usageAPI } from '@/api/usage'
import * as adminUsageAPI from '@/api/admin/usage'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { formatDateTime } from '@/utils/format'
import type { SelectOption } from '@/components/common/Select.vue'
import type { NonworkStatsCoverage, UserTokenRankingItem } from '@/types'
import type { ExternalUsageImportBatch, ExternalUsageImportPreview, ExternalUsageImportRow } from '@/api/admin/usage'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const formatDate = (date: Date) => date.toISOString().split('T')[0]
const startDate = ref(formatDate(new Date(Date.now() - 6 * 86400000)))
const endDate = ref(formatDate(new Date()))
const loading = ref(false)
const exporting = ref(false)
const exportMenuOpen = ref(false)
const exportMenuRef = ref<HTMLElement | null>(null)
const rankingTableScrollRef = ref<HTMLElement | null>(null)
const error = ref(false)
const rankingScope = ref<'all' | 'nonwork'>('all')
const rankBy = ref<'tokens' | 'nonwork_tokens' | 'requests' | 'active_duration' | 'nonwork_active_duration' | 'actual_cost'>('tokens')
const sortOrder = ref<'asc' | 'desc'>('asc')
const rankingItems = ref<UserTokenRankingItem[]>([])
const totals = ref({ totalTokens: 0, nonworkTokens: 0, requests: 0, actualCost: 0, nonworkTokenRatio: 0 })
const pagination = ref({
  page: 1,
  page_size: getPersistedPageSize(),
})
const responseStartDate = ref('')
const responseEndDate = ref('')
const calendarConfirmed = ref<boolean | null>(null)
const statsCoverage = ref<NonworkStatsCoverage | null>(null)
const importDialogOpen = ref(false)
const importFileInputRef = ref<HTMLInputElement | null>(null)
const importFileName = ref('')
const importFileSHA256 = ref('')
const importRows = ref<ExternalUsageImportRow[]>([])
const importPreview = ref<ExternalUsageImportPreview | null>(null)
const importErrors = ref<string[]>([])
const importNote = ref('')
const importing = ref(false)
const externalExportDialogOpen = ref(false)
const externalExportStartDate = ref(startDate.value)
const externalExportEndDate = ref(endDate.value)
const externalExportIncludeNonwork = ref(true)
const exportingExternal = ref(false)
const importBatchesDialogOpen = ref(false)
const importBatches = ref<ExternalUsageImportBatch[]>([])

const rankingScopeOptions = computed<SelectOption[]>(() => [
  { value: 'all', label: t('tokenRanking.scopeAll') },
  { value: 'nonwork', label: t('tokenRanking.scopeNonwork') },
])

const rankByOptions = computed<SelectOption[]>(() => [
  { value: 'tokens', label: t('tokenRanking.rankByTokens') },
  { value: 'nonwork_tokens', label: t('tokenRanking.rankByNonworkTokens') },
  { value: 'requests', label: t('tokenRanking.rankByRequests') },
  { value: 'active_duration', label: t('tokenRanking.rankByActiveDuration') },
  { value: 'nonwork_active_duration', label: t('tokenRanking.rankByNonworkActiveDuration') },
  { value: 'actual_cost', label: t('tokenRanking.rankBySpend') },
])

const sortOrderOptions = computed<SelectOption[]>(() => [
  { value: 'asc', label: t('tokenRanking.sortAsc') },
  { value: 'desc', label: t('tokenRanking.sortDesc') },
])

const responseRange = computed(() => {
  if (!responseStartDate.value || !responseEndDate.value) return ''
  return `${responseStartDate.value} - ${responseEndDate.value}`
})

const lastStatsComputedAt = computed(() => {
  const value = statsCoverage.value?.last_computed_at
  return value ? formatDateTime(value) : ''
})

const paginationStart = computed(() => (pagination.value.page - 1) * pagination.value.page_size)

const paginatedRankingItems = computed(() => {
  const start = paginationStart.value
  return rankingItems.value.slice(start, start + pagination.value.page_size)
})

const statsCoverageMissingSummary = computed(() => {
  const ranges = statsCoverage.value?.missing_ranges || []
  return ranges.slice(0, 3).map((range) => {
    if (range.start_date === range.end_date) return range.start_date
    return `${range.start_date}~${range.end_date}`
  }).join(', ')
})

type ExportFormat = 'xlsx' | 'csv'

const importHeaders = [
  '日期',
  '用户中文名',
  '请求数',
  '总Token',
  '输入Token',
  '输出Token',
  '缓存创建Token',
  '缓存读取Token',
  '实际消费',
  '活跃时长毫秒',
  '非工作时间Token',
  '非工作时间活跃时长毫秒',
  '备注'
]

const canConfirmImport = computed(() => {
  const summary = importPreview.value?.summary
  if (!summary) return false
  return summary.matched_rows + summary.overwritten_rows > 0 &&
    summary.invalid_rows === 0 &&
    summary.unmatched_rows === 0 &&
    summary.conflict_rows === 0
})

function userLabel(item: UserTokenRankingItem): string {
  if (item.username?.trim()) return item.username.trim()
  if (item.email) return item.email
  return t('tokenRanking.userFallback', { id: item.user_id })
}

function formatNumber(value: number): string {
  return value.toLocaleString()
}

function formatTokens(value: number): string {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(2)}K`
  return value.toLocaleString()
}

function formatCost(value: number): string {
  if (value >= 1000) return `${(value / 1000).toFixed(2)}K`
  if (value >= 1) return value.toFixed(2)
  if (value >= 0.01) return value.toFixed(3)
  return value.toFixed(4)
}

function formatPercent(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0%'
  if (value >= 0.995) return `${(value * 100).toFixed(0)}%`
  return `${(value * 100).toFixed(1)}%`
}

function formatDuration(ms: number): string {
  if (ms <= 0) return '0m'
  const totalMinutes = Math.round(ms / 60000)
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  if (hours <= 0) return `${minutes}m`
  if (minutes === 0) return `${hours}h`
  return `${hours}h ${minutes}m`
}

function exportRows() {
  return rankingItems.value.map((item, index) => ({
    rank: index + 1,
    user: userLabel(item),
    email: item.email || '',
    requests: item.requests,
    tokens: item.tokens,
    nonwork_tokens: item.nonwork_tokens ?? 0,
    active_duration: formatDuration(item.active_duration_ms || 0),
    nonwork_active_duration: formatDuration(item.nonwork_active_ms || 0),
    actual_cost: item.actual_cost
  }))
}

function exportFileName(format: ExportFormat): string {
  return `token-ranking_${responseStartDate.value || startDate.value}_to_${responseEndDate.value || endDate.value}.${format === 'xlsx' ? 'xlsx' : 'csv'}`
}

async function exportRanking(format: ExportFormat) {
  exportMenuOpen.value = false
  if (exporting.value) return
  if (!rankingItems.value.length) {
    appStore.showWarning(t('usage.noDataToExport'))
    return
  }

  exporting.value = true
  try {
    const rows = exportRows()
    const header = [
      t('tokenRanking.rank'),
      t('tokenRanking.user'),
      'Email',
      t('tokenRanking.requests'),
      t('tokenRanking.tokens'),
      t('tokenRanking.nonworkTokens'),
      t('tokenRanking.activeDuration'),
      t('tokenRanking.nonworkActiveDuration'),
      t('tokenRanking.spend')
    ]
    const body = rows.map((row) => [
      row.rank,
      row.user,
      row.email,
      row.requests,
      row.tokens,
      row.nonwork_tokens,
      row.active_duration,
      row.nonwork_active_duration,
      row.actual_cost
    ])

    if (format === 'xlsx') {
      const XLSX = await import('xlsx')
      const worksheet = XLSX.utils.aoa_to_sheet([header, ...body])
      const workbook = XLSX.utils.book_new()
      XLSX.utils.book_append_sheet(workbook, worksheet, 'Token Ranking')
      const data = XLSX.write(workbook, { bookType: 'xlsx', type: 'array' })
      saveAs(new Blob([data], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }), exportFileName(format))
    } else {
      const XLSX = await import('xlsx')
      const worksheet = XLSX.utils.aoa_to_sheet([header, ...body])
      const csv = XLSX.utils.sheet_to_csv(worksheet)
      saveAs(new Blob([csv], { type: 'text/csv;charset=utf-8' }), exportFileName(format))
    }

    appStore.showSuccess(t('tokenRanking.exportSuccess'))
  } catch (err) {
    console.error('Failed to export token ranking:', err)
    appStore.showError(t('tokenRanking.exportFailed'))
  } finally {
    exporting.value = false
  }
}

function openImportDialog() {
  importDialogOpen.value = true
}

function closeImportDialog() {
  if (importing.value) return
  importDialogOpen.value = false
  importFileName.value = ''
  importFileSHA256.value = ''
  importRows.value = []
  importPreview.value = null
  importErrors.value = []
  importNote.value = ''
  if (importFileInputRef.value) {
    importFileInputRef.value.value = ''
  }
}

async function handleImportFileChange(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return
  importFileName.value = file.name
  importPreview.value = null
  importErrors.value = []
  try {
    const buffer = await file.arrayBuffer()
    importFileSHA256.value = await sha256Hex(buffer)
    const rows = await parseImportWorkbook(buffer)
    if (!rows.length) {
      importErrors.value = [t('tokenRanking.emptyExcel')]
      return
    }
    importRows.value = rows
    importPreview.value = await adminUsageAPI.previewExternalUsageImport(importPayload())
  } catch (err) {
    console.error('Failed to parse external usage import:', err)
    importErrors.value = [err instanceof Error ? err.message : t('tokenRanking.importParseFailed')]
  }
}

async function parseImportWorkbook(buffer: ArrayBuffer): Promise<ExternalUsageImportRow[]> {
  const XLSX = await import('xlsx')
  const workbook = XLSX.read(buffer, { type: 'array', cellDates: true })
  const sheet = workbook.Sheets[workbook.SheetNames[0]]
  if (!sheet) return []
  const rawRows = XLSX.utils.sheet_to_json<Record<string, unknown>>(sheet, { defval: '', raw: true })
  const headerRow = XLSX.utils.sheet_to_json<string[]>(sheet, { header: 1, defval: '' })[0] || []
  const missingHeaders = ['日期', '用户中文名', '请求数', '总Token'].filter((header) => !headerRow.includes(header))
  if (missingHeaders.length) {
    throw new Error(t('tokenRanking.missingRequiredColumns', { columns: missingHeaders.join(', ') }))
  }

  const rows: ExternalUsageImportRow[] = []
  const errors: string[] = []
  rawRows.forEach((raw, index) => {
    const rowNumber = index + 2
    const date = normalizeImportDate(raw['日期'])
    const username = String(raw['用户中文名'] ?? '').trim()
    const requestsRaw = raw['请求数']
    const totalTokensRaw = raw['总Token']
    if (!date || !username || isBlank(requestsRaw) || isBlank(totalTokensRaw)) {
      errors.push(t('tokenRanking.requiredCellMissing', { row: rowNumber }))
      return
    }
    rows.push({
      row_number: rowNumber,
      date,
      username,
      requests: parseIntegerCell(requestsRaw),
      total_tokens: parseIntegerCell(totalTokensRaw),
      input_tokens: parseIntegerCell(raw['输入Token']),
      output_tokens: parseIntegerCell(raw['输出Token']),
      cache_creation_tokens: parseIntegerCell(raw['缓存创建Token']),
      cache_read_tokens: parseIntegerCell(raw['缓存读取Token']),
      actual_cost: parseNumberCell(raw['实际消费']),
      active_duration_ms: parseDurationMs(raw['活跃时长毫秒'], raw['活跃时长秒']),
      nonwork_tokens: parseIntegerCell(raw['非工作时间Token']),
      nonwork_active_ms: parseDurationMs(raw['非工作时间活跃时长毫秒'], raw['非工作时间活跃时长秒']),
      note: String(raw['备注'] ?? '').trim()
    })
  })
  if (errors.length) {
    importErrors.value = errors
    return []
  }
  return rows
}

function isBlank(value: unknown): boolean {
  return value === undefined || value === null || String(value).trim() === ''
}

function normalizeImportDate(value: unknown): string {
  if (value instanceof Date && !Number.isNaN(value.getTime())) {
    return formatDate(value)
  }
  const raw = String(value ?? '').trim()
  if (!raw) return ''
  const normalized = raw.replace(/\//g, '-')
  const match = normalized.match(/^(\d{4})-(\d{1,2})-(\d{1,2})/)
  if (match) {
    return `${match[1]}-${match[2].padStart(2, '0')}-${match[3].padStart(2, '0')}`
  }
  const serial = Number(raw)
  if (Number.isFinite(serial) && serial > 25569) {
    return formatDate(new Date((serial - 25569) * 86400000))
  }
  return raw
}

function parseIntegerCell(value: unknown): number {
  if (isBlank(value)) return 0
  const parsed = Number(String(value).replace(/,/g, '').trim())
  if (!Number.isFinite(parsed)) return -1
  return Math.trunc(parsed)
}

function parseNumberCell(value: unknown): number {
  if (isBlank(value)) return 0
  const parsed = Number(String(value).replace(/,/g, '').trim())
  return Number.isFinite(parsed) ? parsed : -1
}

function parseDurationMs(msValue: unknown, secondsValue: unknown): number {
  if (!isBlank(msValue)) return parseIntegerCell(msValue)
  if (!isBlank(secondsValue)) return parseIntegerCell(secondsValue) * 1000
  return 0
}

async function sha256Hex(buffer: ArrayBuffer): Promise<string> {
  if (!crypto?.subtle) return ''
  const hash = await crypto.subtle.digest('SHA-256', buffer)
  return Array.from(new Uint8Array(hash)).map((byte) => byte.toString(16).padStart(2, '0')).join('')
}

function importPayload(): adminUsageAPI.ExternalUsageImportPayload {
  return {
    file_name: importFileName.value,
    file_sha256: importFileSHA256.value,
    note: importNote.value,
    rows: importRows.value
  }
}

async function confirmExternalImport() {
  if (!canConfirmImport.value || importing.value) return
  importing.value = true
  try {
    const result = await adminUsageAPI.importExternalUsage(importPayload())
    appStore.showSuccess(t('tokenRanking.importSuccess', { count: result.summary.imported_rows }))
    closeImportDialog()
    loadRanking()
  } catch (err) {
    console.error('Failed to import external usage:', err)
    appStore.showError(t('tokenRanking.importFailed'))
  } finally {
    importing.value = false
  }
}

async function downloadImportTemplate() {
  const XLSX = await import('xlsx')
  const sampleRows = [
    ['2026-06-21', '张三', 18, 123456, 60000, 60000, 2000, 1456, 1.23, 1800000, 50000, 900000, '周末导入示例'],
    ['2026-06-21', '张三', 7, 34567, 12000, 20000, 1000, 1567, 0.35, 600000, 12000, 300000, '同日同用户示例，导入时会与上一行叠加'],
    ['2026-06-22', '李四', 9, 45678, 20000, 25000, 500, 178, 0.46, 600000, 0, 0, '工作日导入示例']
  ]
  const worksheet = XLSX.utils.aoa_to_sheet([importHeaders, ...sampleRows])
  const workbook = XLSX.utils.book_new()
  XLSX.utils.book_append_sheet(workbook, worksheet, '导入模板')
  const data = XLSX.write(workbook, { bookType: 'xlsx', type: 'array' })
  saveAs(new Blob([data], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }), 'Token使用排名外部导入模板.xlsx')
}

function openExternalExportDialog() {
  externalExportStartDate.value = startDate.value
  externalExportEndDate.value = endDate.value
  externalExportDialogOpen.value = true
}

async function exportExternalImportData() {
  if (exportingExternal.value) return
  exportingExternal.value = true
  try {
    const result = await adminUsageAPI.exportExternalUsageRows({
      start_date: externalExportStartDate.value,
      end_date: externalExportEndDate.value,
      include_nonwork: externalExportIncludeNonwork.value
    })
    const XLSX = await import('xlsx')
    const body = result.rows.map((row) => [
      row.date,
      row.username,
      row.requests,
      row.total_tokens,
      row.input_tokens || 0,
      row.output_tokens || 0,
      row.cache_creation_tokens || 0,
      row.cache_read_tokens || 0,
      row.actual_cost || 0,
      row.active_duration_ms || 0,
      row.nonwork_tokens || 0,
      row.nonwork_active_ms || 0,
      row.note || ''
    ])
    const worksheet = XLSX.utils.aoa_to_sheet([importHeaders, ...body])
    const workbook = XLSX.utils.book_new()
    XLSX.utils.book_append_sheet(workbook, worksheet, '导入数据')
    const data = XLSX.write(workbook, { bookType: 'xlsx', type: 'array' })
    saveAs(new Blob([data], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }), `token-ranking-import-data_${result.start_date}_to_${result.end_date}.xlsx`)
    appStore.showSuccess(t('tokenRanking.exportSuccess'))
    externalExportDialogOpen.value = false
  } catch (err) {
    console.error('Failed to export external usage rows:', err)
    appStore.showError(t('tokenRanking.exportFailed'))
  } finally {
    exportingExternal.value = false
  }
}

async function loadImportBatches() {
  const result = await adminUsageAPI.listExternalUsageImportBatches({ page: 1, page_size: 50 })
  importBatches.value = result.items || []
}

async function voidImportBatch(id: number) {
  if (!window.confirm(t('tokenRanking.voidBatchConfirm'))) return
  try {
    await adminUsageAPI.voidExternalUsageImportBatch(id)
    appStore.showSuccess(t('tokenRanking.voidBatchSuccess'))
    await loadImportBatches()
    loadRanking()
  } catch (err) {
    console.error('Failed to void external usage import batch:', err)
    appStore.showError(t('tokenRanking.voidBatchFailed'))
  }
}

function handleDocumentClick(event: MouseEvent) {
  if (!exportMenuOpen.value) return
  const target = event.target as Node | null
  if (target && exportMenuRef.value?.contains(target)) return
  exportMenuOpen.value = false
}

async function loadRanking() {
  loading.value = true
  error.value = false
  try {
    const response = await usageAPI.getDashboardNonworkTokenRanking({
      start_date: startDate.value,
      end_date: endDate.value,
      limit: 10000,
      scope: rankingScope.value,
      rank_by: rankBy.value,
      sort_order: sortOrder.value
    })
    rankingItems.value = response.ranking || []
    clampPagination()
    totals.value = {
      totalTokens: response.total_tokens || 0,
      nonworkTokens: response.total_nonwork_tokens || 0,
      requests: response.total_requests || 0,
      actualCost: response.total_actual_cost || 0,
      nonworkTokenRatio: response.nonwork_token_ratio || 0
    }
    calendarConfirmed.value = response.calendar_confirmed ?? null
    statsCoverage.value = response.stats_coverage || null
    responseStartDate.value = response.start_date || startDate.value
    responseEndDate.value = response.end_date || endDate.value
  } catch (err) {
    console.error('Failed to load token ranking:', err)
    rankingItems.value = []
    totals.value = { totalTokens: 0, nonworkTokens: 0, requests: 0, actualCost: 0, nonworkTokenRatio: 0 }
    calendarConfirmed.value = null
    statsCoverage.value = null
    error.value = true
  } finally {
    loading.value = false
  }
}

function clampPagination() {
  const totalPages = Math.max(1, Math.ceil(rankingItems.value.length / pagination.value.page_size))
  if (pagination.value.page > totalPages) {
    pagination.value.page = totalPages
  }
}

function handleFilterChange() {
  pagination.value.page = 1
  loadRanking()
}

function handlePageChange(page: number) {
  pagination.value.page = page
  scrollRankingTableToTop()
}

function handlePageSizeChange(pageSize: number) {
  pagination.value.page_size = pageSize
  pagination.value.page = 1
  scrollRankingTableToTop()
}

function scrollRankingTableToTop() {
  rankingTableScrollRef.value?.scrollTo({ top: 0, left: 0 })
}

onMounted(() => {
  document.addEventListener('click', handleDocumentClick)
  loadRanking()
})
onUnmounted(() => {
  document.removeEventListener('click', handleDocumentClick)
})
</script>

<style scoped>
.token-ranking-table-scroll {
  scrollbar-width: thin;
  scrollbar-color: rgba(107, 114, 128, 0.7) rgba(0, 0, 0, 0.03);
  scrollbar-gutter: stable;
}

.token-ranking-table-scroll::-webkit-scrollbar {
  height: 12px;
  width: 12px;
}

.token-ranking-table-scroll::-webkit-scrollbar-track {
  background-color: rgba(0, 0, 0, 0.03);
  border-radius: 6px;
}

.token-ranking-table-scroll::-webkit-scrollbar-thumb {
  background-clip: padding-box;
  background-color: rgba(107, 114, 128, 0.75);
  border: 2px solid transparent;
  border-radius: 6px;
}

.token-ranking-table-scroll::-webkit-scrollbar-thumb:hover {
  background-color: rgba(75, 85, 99, 0.9);
}

.dark .token-ranking-table-scroll {
  scrollbar-color: rgba(156, 163, 175, 0.75) rgba(255, 255, 255, 0.05);
}

.dark .token-ranking-table-scroll::-webkit-scrollbar-track {
  background-color: rgba(255, 255, 255, 0.05);
}

.dark .token-ranking-table-scroll::-webkit-scrollbar-thumb {
  background-color: rgba(156, 163, 175, 0.75);
}

.dark .token-ranking-table-scroll::-webkit-scrollbar-thumb:hover {
  background-color: rgba(209, 213, 219, 0.9);
}
</style>
