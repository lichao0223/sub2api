<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('tokenRanking.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('tokenRanking.description') }}
          </p>
        </div>
        <button class="btn btn-secondary self-start md:self-auto" :disabled="loading" @click="loadRanking">
          {{ t('common.refresh') }}
        </button>
      </div>

      <div class="card p-4">
        <div class="flex flex-wrap items-center gap-4">
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('dashboard.timeRange') }}:
            </span>
            <DateRangePicker
              v-model:start-date="startDate"
              v-model:end-date="endDate"
              @change="loadRanking"
            />
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

        <div v-else-if="!rankingItems.length" class="card flex min-h-64 items-center justify-center p-6 text-sm text-gray-500 dark:text-gray-400">
          {{ t('tokenRanking.noData') }}
        </div>

        <template v-else>
          <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
            <div class="card p-4">
              <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('tokenRanking.totalTokens') }}</div>
              <div class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ formatTokens(totals.tokens) }}</div>
            </div>
            <div class="card p-4">
              <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('tokenRanking.totalRequests') }}</div>
              <div class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ formatNumber(totals.requests) }}</div>
            </div>
            <div class="card p-4">
              <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('tokenRanking.totalSpend') }}</div>
              <div class="mt-2 text-2xl font-bold text-emerald-600 dark:text-emerald-400">${{ formatCost(totals.actualCost) }}</div>
            </div>
          </div>

          <div class="rounded-2xl bg-white p-6 shadow-lg shadow-slate-200/70 dark:bg-dark-900 dark:shadow-black/20 md:p-8">
            <div class="flex flex-col gap-4 border-b border-slate-200 pb-6 dark:border-dark-700 md:flex-row md:items-center md:justify-between">
              <div class="flex items-center gap-4">
                <div class="flex h-14 w-14 items-center justify-center rounded-full bg-blue-50 text-2xl shadow-inner dark:bg-blue-950/40">
                  🏆
                </div>
                <h2 class="text-3xl font-bold text-slate-950 dark:text-white">
                  {{ t('tokenRanking.topThree') }}
                </h2>
              </div>
              <span class="flex items-center gap-2 text-base font-medium text-slate-500 dark:text-slate-400">
                <Icon name="calendar" size="md" />
                {{ responseRange }}
              </span>
            </div>
            <div class="mt-8 grid grid-cols-1 gap-6 md:grid-cols-3 md:items-end">
              <div
                v-for="entry in podiumItems"
                :key="entry.rank"
                class="relative flex flex-col overflow-hidden rounded-2xl border p-5 shadow-lg xl:p-6"
                :class="[rankCardClass(entry.rank), entry.orderClass]"
              >
                <div class="absolute right-6 top-6 text-5xl opacity-20" aria-hidden="true">
                  {{ rankWatermark(entry.rank) }}
                </div>
                <div class="flex justify-center">
                  <div class="flex h-12 w-12 items-center justify-center rounded-full text-xl font-bold shadow-md xl:h-14 xl:w-14 xl:text-2xl" :class="rankBadgeClass(entry.rank)">
                    #{{ entry.rank }}
                  </div>
                </div>
                <div v-if="entry.rank === 1" class="mt-2 flex justify-center text-6xl leading-none xl:text-7xl" aria-hidden="true">
                  🏆
                </div>
                <div class="flex flex-1 items-center justify-center py-5 xl:py-6">
                  <div class="max-w-full truncate text-center text-3xl font-bold text-slate-950 dark:text-white xl:text-4xl" :title="userLabel(entry.item)">
                    {{ userLabel(entry.item) }}
                  </div>
                </div>
                <div class="mb-5 h-0.5 rounded-full xl:mb-6" :class="rankDividerClass(entry.rank)"></div>
                <div class="grid grid-cols-3 divide-x text-center" :class="rankDivideClass(entry.rank)">
                  <div class="min-w-0 px-1.5 xl:px-2">
                    <div class="mb-2.5 flex min-w-0 items-center justify-center gap-1.5 text-xs font-medium" :class="rankMetricLabelClass(entry.rank)">
                      <Icon name="database" size="sm" />
                      <span class="truncate">{{ t('tokenRanking.tokens') }}</span>
                    </div>
                    <div class="whitespace-nowrap text-xl font-bold text-slate-950 dark:text-white xl:text-2xl">{{ formatTokens(entry.item.tokens) }}</div>
                  </div>
                  <div class="min-w-0 px-1.5 xl:px-2">
                    <div class="mb-2.5 flex min-w-0 items-center justify-center gap-1.5 text-xs font-medium" :class="rankMetricLabelClass(entry.rank)">
                      <Icon name="chartBar" size="sm" />
                      <span class="truncate">{{ t('tokenRanking.requests') }}</span>
                    </div>
                    <div class="whitespace-nowrap text-xl font-bold text-slate-950 dark:text-white xl:text-2xl">{{ formatNumber(entry.item.requests) }}</div>
                  </div>
                  <div class="min-w-0 px-1.5 xl:px-2">
                    <div class="mb-2.5 flex min-w-0 items-center justify-center gap-1.5 text-xs font-medium" :class="rankMetricLabelClass(entry.rank)">
                      <Icon name="creditCard" size="sm" />
                      <span class="truncate">{{ t('tokenRanking.spend') }}</span>
                    </div>
                    <div class="whitespace-nowrap text-xl font-bold text-slate-950 dark:text-white xl:text-2xl">${{ formatCost(entry.item.actual_cost) }}</div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div class="card overflow-hidden">
            <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700">
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('tokenRanking.rankingList') }}
              </h2>
            </div>
            <div class="overflow-x-auto">
              <table class="w-full text-sm">
                <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                  <tr>
                    <th class="px-4 py-3 text-left">{{ t('tokenRanking.rank') }}</th>
                    <th class="px-4 py-3 text-left">{{ t('tokenRanking.user') }}</th>
                    <th class="px-4 py-3 text-right">{{ t('tokenRanking.requests') }}</th>
                    <th class="px-4 py-3 text-right">{{ t('tokenRanking.tokens') }}</th>
                    <th class="px-4 py-3 text-right">{{ t('tokenRanking.spend') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="(item, index) in rankingItems"
                    :key="`${item.user_id}-${index}`"
                    class="border-t border-gray-100 dark:border-dark-700"
                  >
                    <td class="px-4 py-3 font-semibold text-gray-500 dark:text-gray-400">#{{ index + 1 }}</td>
                    <td class="px-4 py-3">
                      <div class="max-w-[260px] truncate font-medium text-gray-900 dark:text-white" :title="userLabel(item)">
                        {{ userLabel(item) }}
                      </div>
                    </td>
                    <td class="px-4 py-3 text-right text-gray-700 dark:text-gray-300">{{ formatNumber(item.requests) }}</td>
                    <td class="px-4 py-3 text-right font-semibold text-gray-900 dark:text-white">{{ formatTokens(item.tokens) }}</td>
                    <td class="px-4 py-3 text-right text-emerald-600 dark:text-emerald-400">${{ formatCost(item.actual_cost) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </template>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import { usageAPI } from '@/api/usage'
import type { UserTokenRankingItem } from '@/types'

const { t } = useI18n()

const formatDate = (date: Date) => date.toISOString().split('T')[0]
const startDate = ref(formatDate(new Date(Date.now() - 6 * 86400000)))
const endDate = ref(formatDate(new Date()))
const loading = ref(false)
const error = ref(false)
const rankingItems = ref<UserTokenRankingItem[]>([])
const totals = ref({ tokens: 0, requests: 0, actualCost: 0 })
const responseStartDate = ref('')
const responseEndDate = ref('')

const topThree = computed(() => rankingItems.value.slice(0, 3))
const podiumItems = computed(() => [
  topThree.value[1] ? { rank: 2, item: topThree.value[1], orderClass: 'md:order-1 md:min-h-[245px] xl:min-h-[260px]' } : null,
  topThree.value[0] ? { rank: 1, item: topThree.value[0], orderClass: 'md:order-2 md:min-h-[305px] xl:min-h-[330px]' } : null,
  topThree.value[2] ? { rank: 3, item: topThree.value[2], orderClass: 'md:order-3 md:min-h-[245px] xl:min-h-[260px]' } : null
].filter((entry): entry is { rank: number; item: UserTokenRankingItem; orderClass: string } => entry !== null))
const responseRange = computed(() => {
  if (!responseStartDate.value || !responseEndDate.value) return ''
  return `${responseStartDate.value} - ${responseEndDate.value}`
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

function rankCardClass(rank: number): string {
  if (rank === 1) {
    return 'border-amber-300 bg-gradient-to-b from-amber-50 via-white to-amber-50/70 shadow-amber-200/70 dark:border-amber-700 dark:from-amber-950/30 dark:via-dark-900 dark:to-amber-950/20'
  }
  if (rank === 2) {
    return 'border-slate-300 bg-gradient-to-b from-slate-50 via-white to-blue-50/40 shadow-slate-200/70 dark:border-slate-600 dark:from-slate-900/60 dark:via-dark-900 dark:to-blue-950/20'
  }
  return 'border-orange-300 bg-gradient-to-b from-orange-50 via-white to-orange-50/60 shadow-orange-200/60 dark:border-orange-800 dark:from-orange-950/30 dark:via-dark-900 dark:to-orange-950/20'
}

function rankBadgeClass(rank: number): string {
  if (rank === 1) return 'bg-amber-200 text-amber-950 shadow-amber-200/80'
  if (rank === 2) return 'bg-slate-200 text-slate-900 shadow-slate-200/80'
  return 'bg-orange-200 text-orange-950 shadow-orange-200/80'
}

function rankDividerClass(rank: number): string {
  if (rank === 1) return 'bg-amber-300'
  if (rank === 2) return 'bg-slate-200'
  return 'bg-orange-200'
}

function rankDivideClass(rank: number): string {
  if (rank === 1) return 'divide-amber-200'
  if (rank === 2) return 'divide-slate-200'
  return 'divide-orange-200'
}

function rankMetricLabelClass(rank: number): string {
  if (rank === 1) return 'text-amber-700 dark:text-amber-300'
  if (rank === 2) return 'text-slate-600 dark:text-slate-300'
  return 'text-orange-700 dark:text-orange-300'
}

function rankWatermark(rank: number): string {
  if (rank === 1) return '★'
  return '◎'
}

async function loadRanking() {
  loading.value = true
  error.value = false
  try {
    const response = await usageAPI.getDashboardTokenRanking({
      start_date: startDate.value,
      end_date: endDate.value,
      limit: 50
    })
    rankingItems.value = response.ranking || []
    totals.value = {
      tokens: response.total_tokens || 0,
      requests: response.total_requests || 0,
      actualCost: response.total_actual_cost || 0
    }
    responseStartDate.value = response.start_date || startDate.value
    responseEndDate.value = response.end_date || endDate.value
  } catch (err) {
    console.error('Failed to load token ranking:', err)
    rankingItems.value = []
    totals.value = { tokens: 0, requests: 0, actualCost: 0 }
    error.value = true
  } finally {
    loading.value = false
  }
}

onMounted(loadRanking)
</script>
