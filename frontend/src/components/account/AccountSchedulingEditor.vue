<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="flex items-center justify-between">
      <div><label class="input-label mb-0">分时段调度</label><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">只在配置的时间段参与新请求调度</p></div>
      <button type="button" class="relative inline-flex h-6 w-11 rounded-full" :class="enabled ? 'bg-primary-600' : 'bg-gray-300 dark:bg-dark-600'" @click="toggle"><span class="h-5 w-5 translate-y-0.5 rounded-full bg-white shadow transition" :class="enabled ? 'translate-x-5' : 'translate-x-0.5'" /></button>
    </div>
    <div v-if="enabled" class="mt-4 space-y-3">
      <select v-model="timezone" class="input" @change="emitValue"><option v-for="item in timezones" :key="item" :value="item">{{ item }}</option></select>
      <div v-for="day in days" :key="day.value" class="grid grid-cols-[5rem_1fr_auto] items-start gap-2">
        <span class="pt-2 text-sm text-gray-600 dark:text-gray-300">{{ day.label }}</span>
        <div class="space-y-2">
          <div v-for="(window, index) in windows[day.value]" :key="index" class="flex gap-2"><input v-model="window[0]" type="time" class="input" @change="emitValue" /><input v-model="window[1]" type="time" class="input" @change="emitValue" /><button type="button" class="px-2 text-gray-500 hover:text-red-500" title="删除时段" @click="remove(day.value, index)">×</button></div>
          <button type="button" class="text-xs text-primary-600" @click="add(day.value)">+ 添加时段</button>
        </div>
        <button type="button" class="mt-1 text-xs text-gray-500" @click="copyWeekday(day.value)">复制到工作日</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
type ScheduleValue = { enabled: boolean; timezone: string; weekly_windows: Record<string, string[][]> }
const props = defineProps<{ modelValue: ScheduleValue | null }>()
const emit = defineEmits<{ 'update:modelValue': [value: ScheduleValue | null] }>()
const enabled = ref(false)
const timezone = ref('Asia/Shanghai')
const windows = ref<Record<number, string[][]>>(Object.fromEntries([1, 2, 3, 4, 5, 6, 7].map((day) => [day, []])))
const days = [
  { value: 1, label: '周一' }, { value: 2, label: '周二' }, { value: 3, label: '周三' },
  { value: 4, label: '周四' }, { value: 5, label: '周五' }, { value: 6, label: '周六' }, { value: 7, label: '周日' }
]
const timezones = ['Asia/Shanghai', 'UTC', 'Asia/Tokyo', 'Asia/Singapore', 'America/Los_Angeles', 'America/New_York', 'Europe/London']
const load = (value: ScheduleValue | null) => {
  enabled.value = value?.enabled === true
  timezone.value = value?.timezone || 'Asia/Shanghai'
  windows.value = Object.fromEntries([1, 2, 3, 4, 5, 6, 7].map((day) => [day, (value?.weekly_windows?.[String(day)] || []).map((item) => [...item])]))
}
watch(() => props.modelValue, load, { immediate: true, deep: true })
const emitValue = () => {
  if (!enabled.value) { emit('update:modelValue', null); return }
  const weekly_windows: Record<string, string[][]> = {}
  for (const day of days) if (windows.value[day.value]?.length) weekly_windows[String(day.value)] = windows.value[day.value]
  emit('update:modelValue', { enabled: true, timezone: timezone.value, weekly_windows })
}
const toggle = () => { enabled.value = !enabled.value; emitValue() }
const add = (day: number) => { windows.value[day].push(['09:00', '18:00']); emitValue() }
const remove = (day: number, index: number) => { windows.value[day].splice(index, 1); emitValue() }
const copyWeekday = (source: number) => { const sourceWindows = windows.value[source].map((item) => [...item]); for (const day of [1, 2, 3, 4, 5]) windows.value[day] = sourceWindows.map((item) => [...item]); emitValue() }
</script>
