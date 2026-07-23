<template>
  <BaseDialog :show="show" :title="t('admin.users.deleteUser')" width="narrow" @close="cancel">
    <div class="space-y-4">
      <p class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('admin.users.deleteConfirm', { email: user?.email || '' }) }}
      </p>

      <label class="flex cursor-pointer items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input
          v-model="migrateUsage"
          data-testid="migrate-usage-checkbox"
          type="checkbox"
          class="mt-0.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
        />
        <span>{{ t('admin.users.migrateUsageHistory') }}</span>
      </label>

      <div v-if="migrateUsage" class="relative">
        <label class="input-label">{{ t('admin.users.migrateUsageTarget') }}</label>
        <input
          v-model="keyword"
          data-testid="usage-migration-target"
          class="input"
          type="text"
          :placeholder="t('admin.users.searchMigrationTarget')"
          @input="scheduleSearch"
          @focus="showOptions = true"
        />
        <div
          v-if="showOptions && keyword.trim()"
          class="absolute z-50 mt-1 max-h-52 w-full overflow-auto rounded-md border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
        >
          <button
            v-for="option in options"
            :key="option.id"
            data-testid="usage-migration-option"
            type="button"
            class="block w-full px-3 py-2 text-left hover:bg-gray-100 dark:hover:bg-dark-700"
            @click="selectTarget(option)"
          >
            <span class="block text-sm font-medium text-gray-900 dark:text-white">
              {{ option.username || option.email }}
            </span>
            <span class="block text-xs text-gray-500 dark:text-gray-400">{{ option.email }}</span>
          </button>
          <div v-if="!searching && options.length === 0" class="px-3 py-2 text-sm text-gray-500">
            {{ t('common.noOptionsFound') }}
          </div>
        </div>
        <p class="input-hint">{{ t('admin.users.migrateUsageHint') }}</p>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="cancel">
          {{ t('common.cancel') }}
        </button>
        <button
          data-testid="confirm-delete-user"
          type="button"
          class="btn bg-red-600 text-white hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-60"
          :disabled="loading || (migrateUsage && !selectedTarget)"
          @click="confirm"
        >
          {{ t('common.delete') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { SimpleUser } from '@/api/admin/usage'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = withDefaults(defineProps<{
  show: boolean
  user: { id: number; email: string; username?: string } | null
  loading?: boolean
}>(), { loading: false })

const emit = defineEmits<{
  (event: 'confirm', targetUserID?: number): void
  (event: 'cancel'): void
}>()

const { t } = useI18n()
const migrateUsage = ref(false)
const keyword = ref('')
const options = ref<SimpleUser[]>([])
const selectedTarget = ref<SimpleUser | null>(null)
const searching = ref(false)
const showOptions = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | null = null
let searchSequence = 0

const reset = () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = null
  searchSequence += 1
  migrateUsage.value = false
  keyword.value = ''
  options.value = []
  selectedTarget.value = null
  searching.value = false
  showOptions.value = false
}

watch(() => [props.show, props.user?.id], reset)
watch(migrateUsage, enabled => {
  if (!enabled) reset()
})

const scheduleSearch = () => {
  selectedTarget.value = null
  options.value = []
  showOptions.value = true
  if (searchTimer) clearTimeout(searchTimer)
  const query = keyword.value.trim()
  if (!query) return
  const sequence = ++searchSequence
  searchTimer = setTimeout(async () => {
    searching.value = true
    try {
      const users = await adminAPI.usage.searchUsers(query)
      if (sequence === searchSequence) {
        options.value = users.filter(item =>
          item.id !== props.user?.id && !item.deleted && item.role !== 'admin' &&
          (!item.status || item.status === 'active')
        )
      }
    } catch {
      if (sequence === searchSequence) options.value = []
    } finally {
      if (sequence === searchSequence) searching.value = false
    }
  }, 300)
}

const selectTarget = (target: SimpleUser) => {
  selectedTarget.value = target
  keyword.value = target.username?.trim() || target.email
  showOptions.value = false
}

const confirm = () => {
  if (props.loading || (migrateUsage.value && !selectedTarget.value)) return
  emit('confirm', migrateUsage.value ? selectedTarget.value?.id : undefined)
}

const cancel = () => {
  if (!props.loading) emit('cancel')
}

onBeforeUnmount(() => {
  if (searchTimer) clearTimeout(searchTimer)
})
</script>
