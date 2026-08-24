<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="flex items-center justify-between">
      <div>
        <label class="input-label mb-0">分时段调度</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">勾选日期后统一设置调度时间段</p>
      </div>
      <button type="button" class="relative inline-flex h-6 w-11 rounded-full" :class="enabled ? 'bg-primary-600' : 'bg-gray-300 dark:bg-dark-600'" @click="toggle">
        <span class="h-5 w-5 translate-y-0.5 rounded-full bg-white shadow transition" :class="enabled ? 'translate-x-5' : 'translate-x-0.5'" />
      </button>
    </div>

    <div v-if="enabled" class="mt-4 space-y-4">
      <select v-model="timezone" class="input" @change="emitValue">
        <option v-for="item in timezones" :key="item" :value="item">时区：{{ item }}</option>
      </select>

      <div>
        <div class="mb-2 flex items-center justify-between">
          <span class="text-xs font-medium text-gray-600 dark:text-gray-300">选择日期</span>
          <button type="button" class="text-xs text-primary-600" @click="selectWeekdays">周一至周五</button>
        </div>
        <div class="grid grid-cols-7 gap-2">
          <button v-for="day in days" :key="day.value" type="button" class="rounded-md border px-2 py-2 text-sm transition" :class="selectedDays.includes(day.value) ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'border-gray-200 text-gray-500 dark:border-dark-600 dark:text-gray-400'" @click="toggleDay(day.value)">
            {{ day.label }}
          </button>
        </div>
      </div>

      <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
        <div class="mb-2 flex items-center justify-between">
          <span class="text-xs font-medium text-gray-600 dark:text-gray-300">已选日期的调度时段</span>
          <button type="button" class="text-xs text-primary-600" @click="addWindow">+ 添加时段</button>
        </div>
        <div v-for="(window, index) in editingWindows" :key="index" class="mb-2 flex items-center gap-2 last:mb-0">
          <input v-model="window[0]" type="time" class="input" />
          <span class="text-gray-400">至</span>
          <input v-model="window[1]" type="time" class="input" />
          <button v-if="editingWindows.length > 1" type="button" class="px-2 text-gray-500 hover:text-red-500" title="删除时段" @click="removeWindow(index)">×</button>
        </div>
        <p v-if="selectedDays.length === 0" class="mt-2 text-xs text-amber-600">请先选择日期</p>
        <button type="button" class="mt-3 w-full rounded-md bg-primary-600 px-3 py-2 text-sm text-white transition hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-50" :disabled="selectedDays.length === 0" @click="applyToSelected">应用到已选日期</button>
      </div>

      <div v-if="configuredDays.length" class="flex flex-wrap gap-1.5">
        <span v-for="item in configuredDays" :key="item.value" class="rounded bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">{{ item.label }} {{ item.summary }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'

type ScheduleValue = { enabled: boolean; timezone: string; weekly_windows: Record<string, string[][]> }
const props = defineProps<{ modelValue: ScheduleValue | null }>()
const emit = defineEmits<{ 'update:modelValue': [value: ScheduleValue | null] }>()
const days = [
  { value: 1, label: '周一' }, { value: 2, label: '周二' }, { value: 3, label: '周三' },
  { value: 4, label: '周四' }, { value: 5, label: '周五' }, { value: 6, label: '周六' }, { value: 7, label: '周日' }
]
const timezones = ['Asia/Shanghai', 'UTC', 'Asia/Tokyo', 'Asia/Singapore', 'America/Los_Angeles', 'America/New_York', 'Europe/London']
const enabled = ref(false)
const timezone = ref('Asia/Shanghai')
const selectedDays = ref<number[]>([1, 2, 3, 4, 5])
const editingWindows = ref<string[][]>([['09:00', '18:00']])
const dayWindows = ref<Record<number, string[][]>>(Object.fromEntries(days.map((day) => [day.value, []])))

const copyWindows = (value: string[][]) => value.map((item) => [...item])
const sameWindows = (left: string[][], right: string[][]) => JSON.stringify(left) === JSON.stringify(right)
const loadEditingWindows = () => {
  const current = selectedDays.value.map((day) => dayWindows.value[day] || [])
  editingWindows.value = current.length && current.every((item) => sameWindows(item, current[0])) && current[0].length
    ? copyWindows(current[0])
    : [['09:00', '18:00']]
}
const load = (value: ScheduleValue | null) => {
  enabled.value = value?.enabled === true
  timezone.value = value?.timezone || 'Asia/Shanghai'
  dayWindows.value = Object.fromEntries(days.map((day) => [day.value, copyWindows(value?.weekly_windows?.[String(day.value)] || [])]))
  const configured = days.map((day) => day.value).filter((day) => dayWindows.value[day].length)
  selectedDays.value = configured.length ? configured : [1, 2, 3, 4, 5]
  loadEditingWindows()
}
watch(() => props.modelValue, load, { immediate: true, deep: true })

const configuredDays = computed(() => days.filter((day) => dayWindows.value[day.value].length).map((day) => ({
  ...day,
  summary: dayWindows.value[day.value].map((item) => `${item[0]}-${item[1]}`).join('、')
})))
const emitValue = () => {
  if (!enabled.value) { emit('update:modelValue', null); return }
  const weekly_windows: Record<string, string[][]> = {}
  for (const day of days) if (dayWindows.value[day.value]?.length) weekly_windows[String(day.value)] = copyWindows(dayWindows.value[day.value])
  emit('update:modelValue', { enabled: true, timezone: timezone.value, weekly_windows })
}
const toggle = () => { enabled.value = !enabled.value; emitValue() }
const toggleDay = (day: number) => {
  selectedDays.value = selectedDays.value.includes(day) ? selectedDays.value.filter((item) => item !== day) : [...selectedDays.value, day].sort((a, b) => a - b)
  loadEditingWindows()
}
const selectWeekdays = () => { selectedDays.value = [1, 2, 3, 4, 5]; loadEditingWindows() }
const addWindow = () => editingWindows.value.push(['09:00', '18:00'])
const removeWindow = (index: number) => editingWindows.value.splice(index, 1)
const applyToSelected = () => {
  for (const day of selectedDays.value) dayWindows.value[day] = copyWindows(editingWindows.value)
  emitValue()
}
</script>
