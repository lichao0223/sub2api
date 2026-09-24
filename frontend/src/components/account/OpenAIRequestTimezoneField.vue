<template>
  <div class="mt-3">
    <label class="input-label">{{ t('admin.accounts.openai.requestTimezone') }}</label>
    <Select v-model="value" class="mt-1" :options="options" :disabled="loading" searchable />
    <p class="input-hint">{{ t('admin.accounts.openai.requestTimezoneDesc') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import Select from '@/components/common/Select.vue'

const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const { t } = useI18n()
const timezones = ref<string[]>([props.modelValue || 'Asia/Singapore'])
const loading = ref(false)
const options = computed(() => timezones.value.map((timezone) => ({ value: timezone, label: timezone })))
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
