<template>
  <BaseDialog :show="show" :title="t('admin.users.userApiKeys')" width="wide" @close="handleClose">
    <div v-if="user" class="space-y-4">
      <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
          <span class="text-lg font-medium text-primary-700 dark:text-primary-300">
            {{ user.email.charAt(0).toUpperCase() }}
          </span>
        </div>
        <div class="min-w-0 flex-1">
          <p class="truncate font-medium text-gray-900 dark:text-white">{{ user.email }}</p>
          <p class="truncate text-sm text-gray-500 dark:text-dark-400">{{ user.username }}</p>
        </div>
        <button class="btn btn-primary" @click="openCreateForm">
          <Icon name="plus" size="sm" class="mr-2" />
          {{ t('common.create') }}
        </button>
      </div>

      <form v-if="showForm" class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-600" @submit.prevent="submitForm">
        <div class="grid gap-3 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('keys.nameLabel') }}</label>
            <input v-model="form.name" required class="input" :placeholder="t('keys.namePlaceholder')" />
          </div>
          <div>
            <label class="input-label">{{ t('keys.groupLabel') }}</label>
            <select v-model="form.group_id" class="input">
              <option :value="0">{{ t('admin.users.none') }}</option>
              <option v-for="group in allGroups" :key="group.id" :value="group.id">
                {{ group.name }}
              </option>
            </select>
          </div>
        </div>
        <div class="grid gap-3 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('common.status') }}</label>
            <select v-model="form.status" class="input">
              <option value="active">{{ t('common.active') }}</option>
              <option value="inactive">{{ t('common.inactive') }}</option>
            </select>
          </div>
          <div>
            <label class="input-label">{{ t('keys.quota') }}</label>
            <input v-model.number="form.quota" type="number" min="0" step="0.01" class="input" placeholder="0" />
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('keys.customKey') }}</label>
          <input v-model="form.custom_key" :disabled="editingKey !== null" class="input font-mono" />
        </div>
        <div class="grid gap-3 md:grid-cols-3">
          <div>
            <label class="input-label">{{ t('keys.rateLimit5h') }}</label>
            <input v-model.number="form.rate_limit_5h" type="number" min="0" step="0.01" class="input" placeholder="0" />
          </div>
          <div>
            <label class="input-label">{{ t('keys.rateLimit1d') }}</label>
            <input v-model.number="form.rate_limit_1d" type="number" min="0" step="0.01" class="input" placeholder="0" />
          </div>
          <div>
            <label class="input-label">{{ t('keys.rateLimit7d') }}</label>
            <input v-model.number="form.rate_limit_7d" type="number" min="0" step="0.01" class="input" placeholder="0" />
          </div>
        </div>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn btn-secondary" @click="closeForm">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn btn-primary" :disabled="submitting">
            <svg v-if="submitting" class="mr-2 h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
            </svg>
            {{ editingKey ? t('common.update') : t('common.create') }}
          </button>
        </div>
      </form>

      <div v-if="loading" class="flex justify-center py-8">
        <svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
        </svg>
      </div>
      <div v-else-if="apiKeys.length === 0" class="py-8 text-center">
        <p class="text-sm text-gray-500">{{ t('admin.users.noApiKeys') }}</p>
      </div>
      <div v-else class="max-h-96 space-y-3 overflow-y-auto">
        <div v-for="key in apiKeys" :key="key.id" class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <div class="mb-1 flex flex-wrap items-center gap-2">
                <span class="font-medium text-gray-900 dark:text-white">{{ key.name }}</span>
                <span :class="['badge text-xs', key.status === 'active' ? 'badge-success' : 'badge-danger']">
                  {{ key.status }}
                </span>
              </div>
              <p class="truncate font-mono text-sm text-gray-500">
                {{ key.key.substring(0, 20) }}...{{ key.key.substring(key.key.length - 8) }}
              </p>
              <div class="mt-3 flex flex-wrap gap-4 text-xs text-gray-500">
                <div class="flex items-center gap-1">
                  <span>{{ t('admin.users.group') }}:</span>
                  <GroupBadge
                    v-if="key.group_id && key.group"
                    :name="key.group.name"
                    :platform="key.group.platform"
                    :subscription-type="key.group.subscription_type"
                    :rate-multiplier="key.group.rate_multiplier"
                  />
                  <span v-else class="text-gray-400 italic">{{ t('admin.users.none') }}</span>
                </div>
                <div>{{ t('admin.users.columns.created') }}: {{ formatDateTime(key.created_at) }}</div>
              </div>
            </div>
            <div class="flex shrink-0 flex-wrap justify-end gap-1">
              <button class="rounded-lg p-2 text-gray-500 hover:bg-gray-100 dark:hover:bg-dark-700" @click="toggleStatus(key)">
                <Icon :name="key.status === 'active' ? 'ban' : 'checkCircle'" size="sm" />
              </button>
              <button class="rounded-lg p-2 text-gray-500 hover:bg-gray-100 dark:hover:bg-dark-700" @click="openEditForm(key)">
                <Icon name="edit" size="sm" />
              </button>
              <button class="rounded-lg p-2 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20" @click="deleteKey(key)">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { formatDateTime } from '@/utils/format'
import type { AdminUser, AdminGroup, ApiKey, CreateApiKeyRequest, UpdateApiKeyRequest } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ show: boolean; user: AdminUser | null }>()
const emit = defineEmits(['close'])
const { t } = useI18n()
const appStore = useAppStore()

const apiKeys = ref<ApiKey[]>([])
const allGroups = ref<AdminGroup[]>([])
const loading = ref(false)
const submitting = ref(false)
const showForm = ref(false)
const editingKey = ref<ApiKey | null>(null)

const emptyForm = () => ({
  name: '',
  group_id: 0,
  status: 'active' as 'active' | 'inactive',
  custom_key: '',
  quota: 0,
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0
})

const form = ref(emptyForm())

watch(() => props.show, (v) => {
  if (v && props.user) {
    load()
    loadGroups()
  } else {
    closeForm()
  }
})

const load = async () => {
  if (!props.user) return
  loading.value = true
  try {
    const res = await adminAPI.users.getUserApiKeys(props.user.id)
    apiKeys.value = res.items || []
  } catch (error: any) {
    appStore.showError(error?.message || t('keys.failedToLoad'))
  } finally {
    loading.value = false
  }
}

const loadGroups = async () => {
  try {
    allGroups.value = await adminAPI.groups.getAll()
  } catch (error) {
    console.error('Failed to load groups:', error)
  }
}

const openCreateForm = () => {
  editingKey.value = null
  form.value = emptyForm()
  showForm.value = true
}

const openEditForm = (key: ApiKey) => {
  editingKey.value = key
  form.value = {
    name: key.name,
    group_id: key.group_id || 0,
    status: key.status === 'active' ? 'active' : 'inactive',
    custom_key: '',
    quota: key.quota || 0,
    rate_limit_5h: key.rate_limit_5h || 0,
    rate_limit_1d: key.rate_limit_1d || 0,
    rate_limit_7d: key.rate_limit_7d || 0
  }
  showForm.value = true
}

const closeForm = () => {
  showForm.value = false
  editingKey.value = null
  form.value = emptyForm()
}

const submitForm = async () => {
  if (!props.user) return
  submitting.value = true
  const groupId = Number(form.value.group_id)
  try {
    if (editingKey.value) {
      const updates: UpdateApiKeyRequest = {
        name: form.value.name,
        group_id: groupId > 0 ? groupId : null,
        status: form.value.status,
        quota: Number(form.value.quota) || 0,
        rate_limit_5h: Number(form.value.rate_limit_5h) || 0,
        rate_limit_1d: Number(form.value.rate_limit_1d) || 0,
        rate_limit_7d: Number(form.value.rate_limit_7d) || 0
      }
      await adminAPI.apiKeys.update(editingKey.value.id, updates)
      appStore.showSuccess(t('keys.keyUpdatedSuccess'))
    } else {
      const payload: CreateApiKeyRequest = {
        name: form.value.name,
        group_id: groupId > 0 ? groupId : 0,
        quota: Number(form.value.quota) || 0,
        rate_limit_5h: Number(form.value.rate_limit_5h) || 0,
        rate_limit_1d: Number(form.value.rate_limit_1d) || 0,
        rate_limit_7d: Number(form.value.rate_limit_7d) || 0
      }
      if (form.value.custom_key.trim()) {
        payload.custom_key = form.value.custom_key.trim()
      }
      await adminAPI.apiKeys.createForUser(props.user.id, payload)
      appStore.showSuccess(t('keys.keyCreatedSuccess'))
    }
    closeForm()
    await load()
  } catch (error: any) {
    appStore.showError(error?.message || t('keys.failedToSave'))
  } finally {
    submitting.value = false
  }
}

const toggleStatus = async (key: ApiKey) => {
  try {
    await adminAPI.apiKeys.update(key.id, { status: key.status === 'active' ? 'inactive' : 'active' })
    await load()
  } catch (error: any) {
    appStore.showError(error?.message || t('keys.failedToUpdateStatus'))
  }
}

const deleteKey = async (key: ApiKey) => {
  if (!window.confirm(t('keys.deleteConfirmMessage', { name: key.name }))) return
  try {
    await adminAPI.apiKeys.delete(key.id)
    appStore.showSuccess(t('keys.keyDeletedSuccess'))
    await load()
  } catch (error: any) {
    appStore.showError(error?.message || t('keys.failedToDelete'))
  }
}

const handleClose = () => {
  closeForm()
  emit('close')
}
</script>
