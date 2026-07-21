<template>
  <div class="card" data-testid="nonwork-calendar-settings">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('admin.usage.nonworkCalendar') }}
          </h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.usage.nonworkCalendarDescription') }}
          </p>
        </div>
        <div class="flex gap-2">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="busy" @click="refresh">
            {{ t('common.refresh') }}
          </button>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="busy" @click="syncCalendar">
            {{ t('admin.usage.syncSelectedYear') }}
          </button>
        </div>
      </div>
      <div class="mt-3 flex flex-wrap gap-2">
        <button
          v-for="status in statuses"
          :key="status.year"
          type="button"
          class="rounded border px-2.5 py-1 text-xs transition-colors"
          :class="[
            status.confirmed
              ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-900/20 dark:text-emerald-300'
              : 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300',
            selectedYear === status.year ? 'ring-2 ring-primary-400' : ''
          ]"
          @click="selectYear(status.year)"
        >
          {{ status.year }} · {{ status.confirmed ? t('admin.usage.calendarConfirmed') : t('admin.usage.calendarPredicted') }}
          · {{ status.confirmed_days }}/{{ status.total_days || daysInYear(status.year) }}
        </button>
      </div>
    </div>

    <div class="space-y-6 p-6">
      <section>
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h3 class="text-sm font-semibold text-gray-800 dark:text-gray-200">
            {{ t('admin.usage.yearOffdays', { year: selectedYear, count: offdays.length }) }}
          </h3>
          <span v-if="loading" class="text-xs text-gray-400">{{ t('common.loading') }}</span>
        </div>
        <div class="max-h-72 overflow-y-auto rounded-lg border border-gray-200 dark:border-dark-700">
          <table class="w-full text-sm">
            <thead class="sticky top-0 bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400">
              <tr>
                <th class="px-3 py-2">{{ t('admin.usage.calendarDate') }}</th>
                <th class="px-3 py-2">{{ t('admin.usage.calendarName') }}</th>
                <th class="px-3 py-2">{{ t('admin.usage.calendarSource') }}</th>
                <th class="px-3 py-2 text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="day in offdays" :key="day.date">
                <td class="whitespace-nowrap px-3 py-2 font-medium text-gray-700 dark:text-gray-200">{{ day.date }}</td>
                <td class="px-3 py-2 text-gray-600 dark:text-gray-300">{{ day.holiday_name || offdayTypeLabel(day.day_type) }}</td>
                <td class="px-3 py-2 text-gray-500 dark:text-gray-400">
                  {{ day.manual_override ? t('admin.usage.calendarManual') : day.source }}
                </td>
                <td class="px-3 py-2 text-right">
                  <button
                    v-if="day.manual_override"
                    type="button"
                    class="text-xs text-red-600 hover:underline dark:text-red-400"
                    :disabled="busy"
                    @click="removeManualOffday(day)"
                  >
                    {{ t('admin.usage.removeOffday') }}
                  </button>
                </td>
              </tr>
              <tr v-if="!loading && offdays.length === 0">
                <td colspan="4" class="px-3 py-8 text-center text-sm text-gray-400">
                  {{ t('admin.usage.noOffdays') }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="rounded-lg bg-gray-50 p-4 dark:bg-dark-800/60">
        <h3 class="text-sm font-semibold text-gray-800 dark:text-gray-200">{{ t('admin.usage.addOffday') }}</h3>
        <div class="mt-3 flex flex-wrap items-end gap-3">
          <div>
            <label class="input-label">{{ t('admin.usage.calendarDate') }}</label>
            <input v-model="manualDate" type="date" class="input w-44" />
          </div>
          <div class="min-w-56 flex-1">
            <label class="input-label">{{ t('admin.usage.calendarName') }}</label>
            <input v-model="manualName" type="text" class="input" :placeholder="t('admin.usage.calendarNamePlaceholder')" />
          </div>
          <button type="button" class="btn btn-primary" :disabled="busy || !manualDate" @click="addOffday">
            {{ t('admin.usage.addOffday') }}
          </button>
        </div>
      </section>

      <section class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
        <h3 class="text-sm font-semibold text-gray-800 dark:text-gray-200">{{ t('admin.usage.backfillNonwork') }}</h3>
        <div class="mt-3 flex flex-wrap items-end gap-3">
          <div>
            <label class="input-label">{{ t('admin.usage.startDate') }}</label>
            <input v-model="backfillStart" type="date" class="input w-44" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.usage.endDate') }}</label>
            <input v-model="backfillEnd" type="date" class="input w-44" />
          </div>
          <button type="button" class="btn btn-secondary" :disabled="busy || !backfillStart || !backfillEnd" @click="backfill">
            {{ t('admin.usage.backfillNonwork') }}
          </button>
          <span v-if="coverage" class="pb-2 text-xs" :class="coverage.complete ? 'text-emerald-600' : 'text-amber-600'">
            {{ t('admin.usage.nonworkStatsCoverage', { aggregated: coverage.aggregated_days, total: coverage.total_days }) }}
          </span>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminUsageAPI, type NonworkCalendarOffday, type NonworkCalendarYearStatus } from '@/api/admin/usage'
import type { NonworkStatsCoverage } from '@/types'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()
const currentYear = new Date().getFullYear()
const years = [currentYear - 1, currentYear, currentYear + 1]
const selectedYear = ref(currentYear)
const statuses = ref<NonworkCalendarYearStatus[]>([])
const offdays = ref<NonworkCalendarOffday[]>([])
const coverage = ref<NonworkStatsCoverage | null>(null)
const loading = ref(false)
const busy = ref(false)
const manualDate = ref('')
const manualName = ref('')
const backfillStart = ref('')
const backfillEnd = ref('')
let refreshTimer: number | undefined

const timezone = () => Intl.DateTimeFormat().resolvedOptions().timeZone
const dateForYear = (year: number, end = false) => `${year}-${end ? '12-31' : '01-01'}`
const daysInYear = (year: number) => (new Date(year, 1, 29).getMonth() === 1 ? 366 : 365)

const setBackfillRange = (year: number) => {
  backfillStart.value = dateForYear(year)
  backfillEnd.value = dateForYear(year, true)
}

const offdayTypeLabel = (dayType: string) =>
  dayType.includes('weekend') ? t('admin.usage.weekend') : t('admin.usage.offday')

const loadYear = async () => {
  const [calendar, stats] = await Promise.all([
    adminUsageAPI.getNonworkCalendarDays(selectedYear.value),
    adminUsageAPI.getNonworkStatsStatus({
      start_date: backfillStart.value,
      end_date: backfillEnd.value,
      timezone: timezone()
    })
  ])
  offdays.value = calendar.days || []
  coverage.value = stats.coverage || null
}

const refresh = async () => {
  loading.value = true
  try {
    const status = await adminUsageAPI.getNonworkCalendarStatus(years)
    statuses.value = status.years || []
    await loadYear()
  } catch {
    appStore.showError(t('admin.usage.failedToLoadCalendarStatus'))
  } finally {
    loading.value = false
  }
}

const selectYear = async (year: number) => {
  selectedYear.value = year
  setBackfillRange(year)
  loading.value = true
  try {
    await loadYear()
  } catch {
    appStore.showError(t('admin.usage.failedToLoadCalendarStatus'))
  } finally {
    loading.value = false
  }
}

const syncCalendar = async () => {
  busy.value = true
  try {
    await adminUsageAPI.syncNonworkCalendar([selectedYear.value])
    appStore.showSuccess(t('admin.usage.calendarSyncAccepted'))
    refreshTimer = window.setTimeout(refresh, 1500)
  } catch {
    appStore.showError(t('admin.usage.calendarSyncFailed'))
  } finally {
    busy.value = false
  }
}

const saveOverride = async (day: NonworkCalendarOffday | null, isWorkday: boolean) => {
  const date = day?.date || manualDate.value
  if (!date) return
  busy.value = true
  try {
    await adminUsageAPI.overrideNonworkCalendarDay({
      date,
      is_workday: isWorkday,
      holiday_name: isWorkday ? '' : manualName.value.trim(),
      timezone: timezone()
    })
    appStore.showSuccess(t('admin.usage.calendarOverrideSaved'))
    manualDate.value = ''
    manualName.value = ''
    await refresh()
  } catch {
    appStore.showError(t('admin.usage.calendarOverrideFailed'))
  } finally {
    busy.value = false
  }
}

const addOffday = () => saveOverride(null, false)
const removeManualOffday = (day: NonworkCalendarOffday) => saveOverride(day, true)

const backfill = async () => {
  busy.value = true
  try {
    await adminUsageAPI.backfillNonworkUsage({
      start_date: backfillStart.value,
      end_date: backfillEnd.value,
      timezone: timezone()
    })
    appStore.showSuccess(t('admin.usage.nonworkBackfillAccepted'))
  } catch {
    appStore.showError(t('admin.usage.nonworkBackfillFailed'))
  } finally {
    busy.value = false
  }
}

setBackfillRange(currentYear)
onMounted(refresh)
onUnmounted(() => window.clearTimeout(refreshTimer))
</script>
