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
                {{ exporting ? t('tokenRanking.exporting') : t('tokenRanking.export') }}
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
            <div ref="rankingTableScrollRef" class="min-h-0 flex-1 overflow-auto">
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
import { useAppStore } from '@/stores/app'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { formatDateTime } from '@/utils/format'
import type { SelectOption } from '@/components/common/Select.vue'
import type { NonworkStatsCoverage, UserTokenRankingItem } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

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
