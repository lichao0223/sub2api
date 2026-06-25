<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.userApiKeys')"
    width="wide"
    @close="handleClose"
  >
    <div v-if="user" class="space-y-4">
      <div
        class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700"
      >
        <div
          class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30"
        >
          <span
            class="text-lg font-medium text-primary-700 dark:text-primary-300"
          >
            {{ user.email.charAt(0).toUpperCase() }}
          </span>
        </div>
        <div class="min-w-0 flex-1">
          <p class="truncate font-medium text-gray-900 dark:text-white">
            {{ user.email }}
          </p>
          <p class="truncate text-sm text-gray-500 dark:text-dark-400">
            {{ user.username }}
          </p>
        </div>
        <button class="btn btn-primary" @click="openCreateForm">
          <Icon name="plus" size="sm" class="mr-2" />
          {{ t('common.create') }}
        </button>
      </div>

      <form
        v-if="showForm"
        class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-600"
        @submit.prevent="submitForm"
      >
        <div class="grid gap-3 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('keys.nameLabel') }}</label>
            <input
              v-model="form.name"
              required
              class="input"
              :placeholder="t('keys.namePlaceholder')"
            />
          </div>
          <div>
            <label class="input-label">{{ t('keys.groupLabel') }}</label>
            <Select
              v-model="form.group_id"
              :options="groupOptions"
              :placeholder="t('keys.selectGroup')"
              :searchable="true"
              :search-placeholder="t('keys.searchGroup')"
            >
              <template #selected="{ option }">
                <span
                  v-if="!option || Number(option.value) === 0"
                  class="text-gray-400"
                >
                  {{ t('keys.noGroup') }}
                </span>
                <GroupBadge
                  v-else
                  :name="(option as unknown as GroupOption).label"
                  :platform="(option as unknown as GroupOption).platform"
                  :subscription-type="
                    (option as unknown as GroupOption).subscriptionType
                  "
                  :rate-multiplier="(option as unknown as GroupOption).rate"
                />
              </template>
              <template #option="{ option, selected }">
                <span
                  v-if="Number(option.value) === 0"
                  class="text-gray-500 dark:text-gray-400"
                >
                  {{ t('keys.noGroup') }}
                </span>
                <GroupOptionItem
                  v-else
                  :name="(option as unknown as GroupOption).label"
                  :platform="(option as unknown as GroupOption).platform"
                  :subscription-type="
                    (option as unknown as GroupOption).subscriptionType
                  "
                  :rate-multiplier="(option as unknown as GroupOption).rate"
                  :description="(option as unknown as GroupOption).description"
                  :selected="selected"
                />
              </template>
            </Select>
          </div>
        </div>
        <div class="grid gap-3 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('common.status') }}</label>
            <Select v-model="form.status" :options="statusOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('keys.quotaLimit') }}</label>
            <div class="relative">
              <span
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500"
                >$</span
              >
              <input
                v-model.number="form.quota"
                type="number"
                min="0"
                step="0.01"
                class="input pl-7"
                :placeholder="t('keys.quotaAmountPlaceholder')"
              />
            </div>
            <p class="input-hint">{{ t('keys.quotaAmountHint') }}</p>
          </div>
        </div>

        <div v-if="!editingKey" class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{
              t('keys.customKeyLabel')
            }}</label>
            <button
              type="button"
              @click="form.use_custom_key = !form.use_custom_key"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                form.use_custom_key
                  ? 'bg-primary-600'
                  : 'bg-gray-200 dark:bg-dark-600',
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  form.use_custom_key ? 'translate-x-4' : 'translate-x-0',
                ]"
              />
            </button>
          </div>
          <div v-if="form.use_custom_key">
            <input
              v-model="form.custom_key"
              class="input font-mono"
              :placeholder="t('keys.customKeyPlaceholder')"
              :class="{ 'border-red-500 dark:border-red-500': customKeyError }"
            />
            <p v-if="customKeyError" class="mt-1 text-sm text-red-500">
              {{ customKeyError }}
            </p>
            <p v-else class="input-hint">{{ t('keys.customKeyHint') }}</p>
          </div>
        </div>

        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{
              t('keys.ipRestriction')
            }}</label>
            <button
              type="button"
              @click="form.enable_ip_restriction = !form.enable_ip_restriction"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                form.enable_ip_restriction
                  ? 'bg-primary-600'
                  : 'bg-gray-200 dark:bg-dark-600',
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  form.enable_ip_restriction
                    ? 'translate-x-4'
                    : 'translate-x-0',
                ]"
              />
            </button>
          </div>
          <div
            v-if="form.enable_ip_restriction"
            class="grid gap-3 md:grid-cols-2"
          >
            <div>
              <label class="input-label">{{ t('keys.ipWhitelist') }}</label>
              <textarea
                v-model="form.ip_whitelist"
                rows="3"
                class="input font-mono text-sm"
                :placeholder="t('keys.ipWhitelistPlaceholder')"
              />
              <p class="input-hint">{{ t('keys.ipWhitelistHint') }}</p>
            </div>
            <div>
              <label class="input-label">{{ t('keys.ipBlacklist') }}</label>
              <textarea
                v-model="form.ip_blacklist"
                rows="3"
                class="input font-mono text-sm"
                :placeholder="t('keys.ipBlacklistPlaceholder')"
              />
              <p class="input-hint">{{ t('keys.ipBlacklistHint') }}</p>
            </div>
          </div>
        </div>

        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{
              t('keys.rateLimitSection')
            }}</label>
            <button
              type="button"
              @click="form.enable_rate_limit = !form.enable_rate_limit"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                form.enable_rate_limit
                  ? 'bg-primary-600'
                  : 'bg-gray-200 dark:bg-dark-600',
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  form.enable_rate_limit ? 'translate-x-4' : 'translate-x-0',
                ]"
              />
            </button>
          </div>
          <div v-if="form.enable_rate_limit" class="grid gap-3 md:grid-cols-3">
            <div>
              <label class="input-label">{{ t('keys.rateLimit5h') }}</label>
              <input
                v-model.number="form.rate_limit_5h"
                type="number"
                min="0"
                step="0.01"
                class="input"
                placeholder="0"
              />
            </div>
            <div>
              <label class="input-label">{{ t('keys.rateLimit1d') }}</label>
              <input
                v-model.number="form.rate_limit_1d"
                type="number"
                min="0"
                step="0.01"
                class="input"
                placeholder="0"
              />
            </div>
            <div>
              <label class="input-label">{{ t('keys.rateLimit7d') }}</label>
              <input
                v-model.number="form.rate_limit_7d"
                type="number"
                min="0"
                step="0.01"
                class="input"
                placeholder="0"
              />
            </div>
          </div>
        </div>

        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.expiration') }}</label>
            <button
              type="button"
              @click="form.enable_expiration = !form.enable_expiration"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                form.enable_expiration
                  ? 'bg-primary-600'
                  : 'bg-gray-200 dark:bg-dark-600',
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  form.enable_expiration ? 'translate-x-4' : 'translate-x-0',
                ]"
              />
            </button>
          </div>
          <div v-if="form.enable_expiration" class="space-y-3">
            <div class="flex flex-wrap gap-2">
              <button
                v-for="days in ['7', '30', '90']"
                :key="days"
                type="button"
                @click="setExpirationDays(parseInt(days))"
                :class="[
                  'rounded-lg px-3 py-1.5 text-sm transition-colors',
                  form.expiration_preset === days
                    ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600',
                ]"
              >
                {{
                  editingKey
                    ? t('keys.extendDays', { days })
                    : t('keys.expiresInDays', { days })
                }}
              </button>
              <button
                type="button"
                @click="form.expiration_preset = 'custom'"
                :class="[
                  'rounded-lg px-3 py-1.5 text-sm transition-colors',
                  form.expiration_preset === 'custom'
                    ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600',
                ]"
              >
                {{ t('keys.customDate') }}
              </button>
            </div>
            <div>
              <label class="input-label">{{ t('keys.expirationDate') }}</label>
              <input
                v-model="form.expiration_date"
                type="datetime-local"
                class="input"
              />
              <p class="input-hint">{{ t('keys.expirationDateHint') }}</p>
            </div>
            <div v-if="editingKey?.expires_at" class="text-sm">
              <span class="text-gray-500 dark:text-gray-400"
                >{{ t('keys.currentExpiration') }}:
              </span>
              <span class="font-medium text-gray-900 dark:text-white">
                {{ formatDateTime(editingKey.expires_at) }}
              </span>
            </div>
          </div>
        </div>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn btn-secondary" @click="closeForm">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" class="btn btn-primary" :disabled="submitting">
            <svg
              v-if="submitting"
              class="mr-2 h-4 w-4 animate-spin"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
              />
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              />
            </svg>
            {{ editingKey ? t('common.update') : t('common.create') }}
          </button>
        </div>
      </form>

      <div v-if="loading" class="flex justify-center py-8">
        <svg
          class="h-8 w-8 animate-spin text-primary-500"
          fill="none"
          viewBox="0 0 24 24"
        >
          <circle
            class="opacity-25"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            stroke-width="4"
          />
          <path
            class="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
          />
        </svg>
      </div>
      <div v-else-if="apiKeys.length === 0" class="py-8 text-center">
        <p class="text-sm text-gray-500">{{ t('admin.users.noApiKeys') }}</p>
      </div>
      <div v-else class="max-h-96 space-y-3 overflow-y-auto">
        <div
          v-for="key in apiKeys"
          :key="key.id"
          class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <div class="mb-1 flex flex-wrap items-center gap-2">
                <span class="font-medium text-gray-900 dark:text-white">{{
                  key.name
                }}</span>
                <span
                  :class="[
                    'badge text-xs',
                    key.status === 'active' ? 'badge-success' : 'badge-danger',
                  ]"
                >
                  {{ key.status }}
                </span>
              </div>
              <p class="truncate font-mono text-sm text-gray-500">
                {{ key.key.substring(0, 20) }}...{{
                  key.key.substring(key.key.length - 8)
                }}
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
                  <span v-else class="text-gray-400 italic">{{
                    t('admin.users.none')
                  }}</span>
                </div>
                <div>
                  {{ t('admin.users.columns.created') }}:
                  {{ formatDateTime(key.created_at) }}
                </div>
              </div>
            </div>
            <div class="flex shrink-0 flex-wrap justify-end gap-1">
              <button
                class="rounded-lg p-2 text-gray-500 hover:bg-gray-100 dark:hover:bg-dark-700"
                @click="toggleStatus(key)"
              >
                <Icon
                  :name="key.status === 'active' ? 'ban' : 'checkCircle'"
                  size="sm"
                />
              </button>
              <button
                class="rounded-lg p-2 text-gray-500 hover:bg-gray-100 dark:hover:bg-dark-700"
                @click="openEditForm(key)"
              >
                <Icon name="edit" size="sm" />
              </button>
              <button
                class="rounded-lg p-2 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20"
                @click="deleteKey(key)"
              >
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
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { formatDateTime } from '@/utils/format'
import type {
  AdminUser,
  AdminGroup,
  ApiKey,
  CreateApiKeyRequest,
  GroupPlatform,
  SubscriptionType,
  UpdateApiKeyRequest,
} from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
import Select from '@/components/common/Select.vue'
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

interface GroupOption extends Record<string, unknown> {
  value: number
  label: string
  description: string | null
  rate: number
  subscriptionType: SubscriptionType
  platform: GroupPlatform
}

const emptyForm = () => ({
  name: '',
  group_id: 0,
  status: 'active' as 'active' | 'inactive',
  use_custom_key: false,
  custom_key: '',
  enable_ip_restriction: false,
  ip_whitelist: '',
  ip_blacklist: '',
  quota: 0,
  enable_rate_limit: false,
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  enable_expiration: false,
  expiration_preset: '30' as '7' | '30' | '90' | 'custom',
  expiration_date: '',
})

const form = ref(emptyForm())

const groupOptions = computed<GroupOption[]>(() => [
  {
    value: 0,
    label: t('keys.noGroup'),
    description: null,
    rate: 1,
    subscriptionType: 'standard',
    platform: 'anthropic',
  },
  ...allGroups.value.map((group) => ({
    value: group.id,
    label: group.name,
    description: group.description,
    rate: group.rate_multiplier,
    subscriptionType: group.subscription_type,
    platform: group.platform,
  })),
])

const statusOptions = computed(() => [
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') },
])

const customKeyError = computed(() => {
  if (!form.value.use_custom_key || !form.value.custom_key) return ''
  if (form.value.custom_key.length < 16) return t('keys.customKeyTooShort')
  if (!/^[a-zA-Z0-9_-]+$/.test(form.value.custom_key))
    return t('keys.customKeyInvalidChars')
  return ''
})

const parseIPList = (text: string): string[] =>
  text
    .split('\n')
    .map((ip) => ip.trim())
    .filter(Boolean)

const formatDateTimeLocal = (isoDate: string): string => {
  const date = new Date(isoDate)
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

const setExpirationDays = (days: number) => {
  form.value.expiration_preset = days.toString() as '7' | '30' | '90'
  const expDate = new Date()
  expDate.setDate(expDate.getDate() + days)
  form.value.expiration_date = formatDateTimeLocal(expDate.toISOString())
}

watch(
  () => props.show,
  (v) => {
    if (v && props.user) {
      load()
      loadGroups()
    } else {
      closeForm()
    }
  },
)

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
  const hasIPRestriction =
    key.ip_whitelist?.length > 0 || key.ip_blacklist?.length > 0
  const hasRateLimit =
    key.rate_limit_5h > 0 || key.rate_limit_1d > 0 || key.rate_limit_7d > 0
  const hasExpiration = !!key.expires_at
  editingKey.value = key
  form.value = {
    name: key.name,
    group_id: key.group_id || 0,
    status: key.status === 'active' ? 'active' : 'inactive',
    use_custom_key: false,
    custom_key: '',
    enable_ip_restriction: hasIPRestriction,
    ip_whitelist: (key.ip_whitelist || []).join('\n'),
    ip_blacklist: (key.ip_blacklist || []).join('\n'),
    quota: key.quota || 0,
    enable_rate_limit: hasRateLimit,
    rate_limit_5h: key.rate_limit_5h || 0,
    rate_limit_1d: key.rate_limit_1d || 0,
    rate_limit_7d: key.rate_limit_7d || 0,
    enable_expiration: hasExpiration,
    expiration_preset: 'custom',
    expiration_date: key.expires_at ? formatDateTimeLocal(key.expires_at) : '',
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
  if (!editingKey.value && form.value.use_custom_key) {
    if (!form.value.custom_key) {
      appStore.showError(t('keys.customKeyRequired'))
      return
    }
    if (customKeyError.value) {
      appStore.showError(customKeyError.value)
      return
    }
  }

  submitting.value = true
  const groupId = Number(form.value.group_id)
  const ipWhitelist = form.value.enable_ip_restriction
    ? parseIPList(form.value.ip_whitelist)
    : []
  const ipBlacklist = form.value.enable_ip_restriction
    ? parseIPList(form.value.ip_blacklist)
    : []
  const rateLimits = form.value.enable_rate_limit
    ? {
        rate_limit_5h: Number(form.value.rate_limit_5h) || 0,
        rate_limit_1d: Number(form.value.rate_limit_1d) || 0,
        rate_limit_7d: Number(form.value.rate_limit_7d) || 0,
      }
    : { rate_limit_5h: 0, rate_limit_1d: 0, rate_limit_7d: 0 }
  try {
    if (editingKey.value) {
      let expiresAt = ''
      if (form.value.enable_expiration && form.value.expiration_date) {
        expiresAt = new Date(form.value.expiration_date).toISOString()
      }
      const updates: UpdateApiKeyRequest = {
        name: form.value.name,
        group_id: groupId > 0 ? groupId : null,
        status: form.value.status,
        ip_whitelist: ipWhitelist,
        ip_blacklist: ipBlacklist,
        quota: Number(form.value.quota) || 0,
        expires_at: expiresAt,
        ...rateLimits,
      }
      await adminAPI.apiKeys.update(editingKey.value.id, updates)
      appStore.showSuccess(t('keys.keyUpdatedSuccess'))
    } else {
      let expiresInDays: number | undefined
      if (form.value.enable_expiration && form.value.expiration_date) {
        const expDate = new Date(form.value.expiration_date)
        const now = new Date()
        const diffDays = Math.ceil(
          (expDate.getTime() - now.getTime()) / (1000 * 60 * 60 * 24),
        )
        expiresInDays = diffDays > 0 ? diffDays : 1
      }
      const payload: CreateApiKeyRequest = {
        name: form.value.name,
        group_id: groupId > 0 ? groupId : 0,
        ip_whitelist: ipWhitelist,
        ip_blacklist: ipBlacklist,
        quota: Number(form.value.quota) || 0,
        expires_in_days: expiresInDays,
        ...rateLimits,
      }
      if (form.value.use_custom_key && form.value.custom_key.trim()) {
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
    await adminAPI.apiKeys.update(key.id, {
      status: key.status === 'active' ? 'inactive' : 'active',
    })
    await load()
  } catch (error: any) {
    appStore.showError(error?.message || t('keys.failedToUpdateStatus'))
  }
}

const deleteKey = async (key: ApiKey) => {
  if (!window.confirm(t('keys.deleteConfirmMessage', { name: key.name })))
    return
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
