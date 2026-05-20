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

          <div class="card p-4">
            <div class="mb-4 flex items-center justify-between">
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('tokenRanking.topThree') }}
              </h2>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ responseRange }}
              </span>
            </div>
            <div class="grid grid-cols-1 gap-4 md:grid-cols-3 md:items-end">
              <div
                v-for="entry in podiumItems"
                :key="entry.rank"
                class="flex flex-col justify-between rounded-lg border p-4"
                :class="[rankCardClass(entry.rank), entry.orderClass]"
              >
                <div class="flex items-center justify-between">
                  <div class="flex h-10 w-10 items-center justify-center rounded-full bg-white text-base font-bold shadow-sm dark:bg-dark-800">
                    #{{ entry.rank }}
                  </div>
                  <div class="text-xs font-semibold">{{ t('tokenRanking.tokens') }}</div>
                </div>
                <div class="mt-5">
                  <div class="truncate text-base font-semibold text-gray-900 dark:text-white" :title="userLabel(entry.item)">
                    {{ userLabel(entry.item) }}
                  </div>
                </div>
                <div class="mt-5 grid grid-cols-3 gap-2 text-center">
                  <div>
                    <div class="text-lg font-bold text-gray-900 dark:text-white">{{ formatTokens(entry.item.tokens) }}</div>
                    <div class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('tokenRanking.tokens') }}</div>
                  </div>
                  <div>
                    <div class="text-lg font-bold text-gray-900 dark:text-white">{{ formatNumber(entry.item.requests) }}</div>
                    <div class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('tokenRanking.requests') }}</div>
                  </div>
                  <div>
                    <div class="text-lg font-bold text-gray-900 dark:text-white">${{ formatCost(entry.item.actual_cost) }}</div>
                    <div class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('tokenRanking.spend') }}</div>
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
  topThree.value[1] ? { rank: 2, item: topThree.value[1], orderClass: 'md:order-1 md:min-h-44' } : null,
  topThree.value[0] ? { rank: 1, item: topThree.value[0], orderClass: 'md:order-2 md:min-h-56' } : null,
  topThree.value[2] ? { rank: 3, item: topThree.value[2], orderClass: 'md:order-3 md:min-h-40' } : null
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
    return 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900/60 dark:bg-amber-900/20 dark:text-amber-300'
  }
  if (rank === 2) {
    return 'border-slate-200 bg-slate-50 text-slate-700 dark:border-slate-700 dark:bg-slate-800/50 dark:text-slate-200'
  }
  return 'border-orange-200 bg-orange-50 text-orange-700 dark:border-orange-900/60 dark:bg-orange-900/20 dark:text-orange-300'
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
