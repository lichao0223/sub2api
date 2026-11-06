<template>
  <section v-if="editable || notice" data-testid="analytics-task-control" class="card p-6" :aria-busy="loading || saving">
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div class="min-w-0 flex-1">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.analyticsTasks.title') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t(editable ? 'admin.settings.analyticsTasks.description' : 'admin.settings.analyticsTasks.pausedNotice') }}
        </p>
      </div>
      <Toggle
        v-if="editable"
        :model-value="status?.control.enabled ?? false"
        :disabled="!status || loading || saving || unavailable"
        :aria-label="t('admin.settings.analyticsTasks.enabled')"
        @update:model-value="save"
      />
    </div>
    <div class="mt-4 flex flex-wrap items-center gap-x-6 gap-y-2 text-sm text-gray-600 dark:text-gray-300" aria-live="polite">
      <span>{{ t('admin.settings.analyticsTasks.actualState') }}：{{ t(`admin.settings.analyticsTasks.states.${unavailable || !status ? 'unknown' : status.state}`) }}</span>
      <span v-if="status && !unavailable">{{ t('admin.settings.analyticsTasks.running') }}：{{ status.running }}</span>
      <span>{{ t('admin.settings.analyticsTasks.lastSuccess') }}：{{ status?.last_success_at ? formatDateTime(status.last_success_at) : '—' }}</span>
    </div>
    <p v-if="error" role="alert" class="mt-3 text-sm text-red-600 dark:text-red-400">{{ error }}</p>
    <div class="mt-3 flex items-center gap-4">
      <button type="button" class="btn btn-secondary btn-sm" data-testid="refresh-analytics-tasks" :disabled="loading || saving" @click="refresh">
        {{ t('admin.settings.analyticsTasks.refresh') }}
      </button>
      <a v-if="!editable" href="/admin/settings" class="text-sm text-primary-600">{{ t('admin.settings.analyticsTasks.openSettings') }}</a>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import { analyticsTasksAPI, type AnalyticsTaskStatus } from '@/api/admin/dataAnalytics'
import { formatDateTime } from '@/utils/format'

defineProps<{ editable?: boolean }>()
const emit = defineEmits<{ loaded: [status: AnalyticsTaskStatus | null] }>()
const { t } = useI18n()
const status = ref<AnalyticsTaskStatus | null>(null)
const loading = ref(false)
const saving = ref(false)
const unavailable = ref(true)
const error = ref('')
const notice = computed(() => unavailable.value || !status.value?.control.enabled || status.value.state === 'unknown')
let disposed = false
let timer: ReturnType<typeof setInterval> | undefined

async function load() {
  loading.value = true
  try {
    const result = await analyticsTasksAPI.status()
    if (!disposed) {
      status.value = result
      unavailable.value = false
      emit('loaded', result)
    }
  } catch {
    if (!disposed) {
      unavailable.value = true
      error.value = t('admin.settings.analyticsTasks.loadFailed')
      emit('loaded', null)
    }
  } finally {
    loading.value = false
  }
}

async function refresh() {
  if (loading.value || saving.value || disposed) return
  error.value = ''
  await load()
}

async function save(enabled: boolean) {
  if (!status.value || saving.value || loading.value || unavailable.value) return
  saving.value = true
  error.value = ''
  try {
    const control = await analyticsTasksAPI.setEnabled(enabled, status.value.control.version)
    if (disposed) return
    status.value = { ...status.value, control, state: enabled ? 'idle' : 'pausing' }
    await load()
  } catch (err) {
    if (disposed) return
    if ((err as { status?: number }).status === 409) {
      await load()
      if (!unavailable.value) error.value = t('admin.settings.analyticsTasks.conflict')
    } else {
      error.value = t('admin.settings.analyticsTasks.saveFailed')
      // The request may have committed before a connection failure.
      unavailable.value = true
      emit('loaded', null)
    }
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void refresh()
  timer = setInterval(() => { void refresh() }, 10_000)
})
onBeforeUnmount(() => {
  disposed = true
  if (timer) clearInterval(timer)
})
</script>
