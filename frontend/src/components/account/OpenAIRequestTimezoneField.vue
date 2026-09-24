<template>
  <div class="mt-3">
    <label class="input-label">{{ t('admin.accounts.openai.requestTimezone') }}</label>
    <select v-model="value" class="input mt-1" :disabled="loading">
      <option v-for="timezone in timezones" :key="timezone" :value="timezone">{{ timezone }}</option>
    </select>
    <p class="input-hint">{{ t('admin.accounts.openai.requestTimezoneDesc') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'

const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const { t } = useI18n()
const timezones = ref<string[]>([props.modelValue || 'Asia/Singapore'])
const loading = ref(false)
const value = computed({
  get: () => props.modelValue,
  set: (next: string) => emit('update:modelValue', next)
})

onMounted(async () => {
  loading.value = true
  try {
    const result = await adminAPI.accounts.getOpenAIRequestTimezones()
    timezones.value = result.timezones
    if (!props.modelValue) emit('update:modelValue', result.default)
  } finally {
    loading.value = false
  }
})
</script>
