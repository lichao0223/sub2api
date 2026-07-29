<template>
  <div class="relative" ref="containerRef">
    <button
      ref="triggerRef"
      type="button"
      @click="toggle"
      :class="['date-picker-trigger', isOpen && 'date-picker-trigger-open']"
    >
      <span class="date-picker-icon">
        <Icon name="calendar" size="sm" />
      </span>
      <span class="date-picker-value">
        {{ displayValue }}
      </span>
      <span class="date-picker-chevron">
        <Icon
          name="chevronDown"
          size="sm"
          :class="['transition-transform duration-200', isOpen && 'rotate-180']"
        />
      </span>
    </button>

    <Teleport to="body">
      <Transition name="date-picker-dropdown">
        <div v-if="isOpen" ref="dropdownRef" class="date-picker-dropdown" :style="dropdownStyle">
        <!-- Quick presets -->
        <div class="date-picker-presets" :class="periodMode && 'date-picker-presets-period'">
          <button
            v-for="preset in availablePresets"
            :key="preset.value"
            @click="selectPreset(preset)"
            :class="['date-picker-preset', isPresetActive(preset) && 'date-picker-preset-active']"
          >
            {{ t(preset.labelKey) }}
          </button>
        </div>

        <div class="date-picker-divider"></div>

        <div v-if="periodMode" class="date-picker-period-tabs">
          <button :class="['date-picker-period-tab', periodGranularity==='month' && 'date-picker-period-tab-active']" @click="setPeriodGranularity('month')">{{ t('dates.selectByMonth') }}</button>
          <button :class="['date-picker-period-tab', periodGranularity==='year' && 'date-picker-period-tab-active']" @click="setPeriodGranularity('year')">{{ t('dates.selectByYear') }}</button>
        </div>

        <!-- Custom date range inputs -->
        <div class="date-picker-custom">
          <div class="date-picker-field">
            <label class="date-picker-label">{{ t(periodMode ? (periodGranularity==='month' ? 'dates.startMonth' : 'dates.startYear') : 'dates.startDate') }}</label>
            <input
              v-if="!periodMode"
              type="date"
              v-model="localStartDate"
              :max="localEndDate || tomorrow"
              class="date-picker-input"
              @change="onDateChange"
            />
            <input v-else-if="periodGranularity==='month'" v-model="startMonth" type="month" :max="endMonth || currentMonth" class="date-picker-input" @change="onDateChange" />
            <select v-else v-model="startYear" class="date-picker-input" @change="onDateChange">
              <option v-for="year in yearOptions" :key="year" :value="year" :disabled="Number(year)>Number(endYear)">{{ year }}年</option>
            </select>
          </div>
          <div class="date-picker-separator">
            <Icon name="arrowRight" size="sm" class="text-gray-400" />
          </div>
          <div class="date-picker-field">
            <label class="date-picker-label">{{ t(periodMode ? (periodGranularity==='month' ? 'dates.endMonth' : 'dates.endYear') : 'dates.endDate') }}</label>
            <input
              v-if="!periodMode"
              type="date"
              v-model="localEndDate"
              :min="localStartDate"
              :max="tomorrow"
              class="date-picker-input"
              @change="onDateChange"
            />
            <input v-else-if="periodGranularity==='month'" v-model="endMonth" type="month" :min="startMonth" :max="currentMonth" class="date-picker-input" @change="onDateChange" />
            <select v-else v-model="endYear" class="date-picker-input" @change="onDateChange">
              <option v-for="year in yearOptions" :key="year" :value="year" :disabled="Number(year)<Number(startYear)">{{ year }}年</option>
            </select>
          </div>
        </div>

        <!-- Apply button -->
        <div class="date-picker-actions">
          <button @click="apply" class="date-picker-apply">
            {{ t('dates.apply') }}
          </button>
        </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

interface DatePreset {
  labelKey: string
  value: string
  getRange: () => { start: string; end: string }
}

interface Props {
  startDate: string
  endDate: string
  periodMode?: boolean
}

interface Emits {
  (e: 'update:startDate', value: string): void
  (e: 'update:endDate', value: string): void
  (e: 'change', range: { startDate: string; endDate: string; preset: string | null }): void
}

const props = withDefaults(defineProps<Props>(), { periodMode: false })
const emit = defineEmits<Emits>()

const { t, locale } = useI18n()

const isOpen = ref(false)
const containerRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const dropdownStyle = ref<Record<string, string>>({})
const localStartDate = ref(props.startDate)
const localEndDate = ref(props.endDate)
const activePreset = ref<string | null>(props.periodMode ? 'thisMonth' : 'last24Hours')
const periodGranularity = ref<'month' | 'year'>('month')

const today = computed(() => {
  // Use local timezone to avoid UTC timezone issues
  const now = new Date()
  const year = now.getFullYear()
  const month = String(now.getMonth() + 1).padStart(2, '0')
  const day = String(now.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
})

// Tomorrow's date - used for max date to handle timezone differences
// When user is in a timezone behind the server, "today" on server might be "tomorrow" locally
const tomorrow = computed(() => {
  const d = new Date()
  d.setDate(d.getDate() + 1)
  return formatDateToString(d)
})

// Helper function to format date to YYYY-MM-DD using local timezone
const formatDateToString = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const dayPresets: DatePreset[] = [
  {
    labelKey: 'dates.today',
    value: 'today',
    getRange: () => {
      const t = today.value
      return { start: t, end: t }
    }
  },
  {
    labelKey: 'dates.yesterday',
    value: 'yesterday',
    getRange: () => {
      const d = new Date()
      d.setDate(d.getDate() - 1)
      const yesterday = formatDateToString(d)
      return { start: yesterday, end: yesterday }
    }
  },
  {
    labelKey: 'dates.last24Hours',
    value: 'last24Hours',
    getRange: () => {
      const end = new Date()
      const start = new Date(end.getTime() - 24 * 60 * 60 * 1000)
      return {
        start: formatDateToString(start),
        end: formatDateToString(end)
      }
    }
  },
  {
    labelKey: 'dates.last7Days',
    value: '7days',
    getRange: () => {
      const end = today.value
      const d = new Date()
      d.setDate(d.getDate() - 6)
      const start = formatDateToString(d)
      return { start, end }
    }
  },
  {
    labelKey: 'dates.last14Days',
    value: '14days',
    getRange: () => {
      const end = today.value
      const d = new Date()
      d.setDate(d.getDate() - 13)
      const start = formatDateToString(d)
      return { start, end }
    }
  },
  {
    labelKey: 'dates.last30Days',
    value: '30days',
    getRange: () => {
      const end = today.value
      const d = new Date()
      d.setDate(d.getDate() - 29)
      const start = formatDateToString(d)
      return { start, end }
    }
  },
  {
    labelKey: 'dates.thisMonth',
    value: 'thisMonth',
    getRange: () => {
      const now = new Date()
      const start = formatDateToString(new Date(now.getFullYear(), now.getMonth(), 1))
      return { start, end: today.value }
    }
  },
  {
    labelKey: 'dates.lastMonth',
    value: 'lastMonth',
    getRange: () => {
      const now = new Date()
      const start = formatDateToString(new Date(now.getFullYear(), now.getMonth() - 1, 1))
      const end = formatDateToString(new Date(now.getFullYear(), now.getMonth(), 0))
      return { start, end }
    }
  }
]

const periodPresets: DatePreset[] = [
  {
    labelKey: 'dates.thisWeek',
    value: 'thisWeek',
    getRange: () => {
      const now = new Date()
      const start = new Date(now)
      start.setDate(now.getDate() - (now.getDay() + 6) % 7)
      return { start: formatDateToString(start), end: today.value }
    }
  },
  dayPresets[6],
  dayPresets[7],
  {
    labelKey: 'dates.thisYear',
    value: 'thisYear',
    getRange: () => ({ start: `${new Date().getFullYear()}-01-01`, end: today.value })
  },
  {
    labelKey: 'dates.lastYear',
    value: 'lastYear',
    getRange: () => {
      const year = new Date().getFullYear() - 1
      return { start: `${year}-01-01`, end: `${year}-12-31` }
    }
  }
]
const availablePresets = computed(() => props.periodMode ? periodPresets : dayPresets)
const currentMonth = computed(() => today.value.slice(0, 7))
const yearOptions = computed(() => Array.from({ length: 10 }, (_, index) => String(new Date().getFullYear() - index)))
const startMonth = computed({
  get: () => localStartDate.value.slice(0, 7),
  set: (value: string) => { localStartDate.value = `${value}-01` }
})
const endMonth = computed({
  get: () => localEndDate.value.slice(0, 7),
  set: (value: string) => {
    const [year, month] = value.split('-').map(Number)
    const end = formatDateToString(new Date(year, month, 0))
    localEndDate.value = end > today.value ? today.value : end
  }
})
const startYear = computed({
  get: () => localStartDate.value.slice(0, 4),
  set: (value: string) => { localStartDate.value = `${value}-01-01` }
})
const endYear = computed({
  get: () => localEndDate.value.slice(0, 4),
  set: (value: string) => {
    const end = `${value}-12-31`
    localEndDate.value = end > today.value ? today.value : end
  }
})

const displayValue = computed(() => {
  if (activePreset.value) {
    const preset = availablePresets.value.find((p) => p.value === activePreset.value)
    if (preset) return t(preset.labelKey)
  }

  if (localStartDate.value && localEndDate.value) {
    if (localStartDate.value === localEndDate.value) {
      return formatDate(localStartDate.value)
    }
    return `${formatDate(localStartDate.value)} - ${formatDate(localEndDate.value)}`
  }

  return t('dates.selectDateRange')
})

const formatDate = (dateStr: string): string => {
  const date = new Date(dateStr + 'T00:00:00')
  const dateLocale = locale.value === 'zh' ? 'zh-CN' : 'en-US'
  return date.toLocaleDateString(dateLocale, { month: 'short', day: 'numeric' })
}

const isPresetActive = (preset: DatePreset): boolean => {
  return activePreset.value === preset.value
}

const selectPreset = (preset: DatePreset) => {
  const range = preset.getRange()
  localStartDate.value = range.start
  localEndDate.value = range.end
  activePreset.value = preset.value
}

const onDateChange = () => {
  // Check if current dates match any preset
  activePreset.value = null
  for (const preset of availablePresets.value) {
    const range = preset.getRange()
    if (range.start === localStartDate.value && range.end === localEndDate.value) {
      activePreset.value = preset.value
      break
    }
  }
}

const setPeriodGranularity = (value: 'month' | 'year') => {
  periodGranularity.value = value
  activePreset.value = null
}

const updateDropdownPosition = () => {
  if (!isOpen.value || !triggerRef.value || !dropdownRef.value) return
  const padding = 8
  const trigger = triggerRef.value.getBoundingClientRect()
  const dropdown = dropdownRef.value.getBoundingClientRect()
  const width = Math.min(Math.max(props.periodMode ? 520 : 320, trigger.width), window.innerWidth - padding * 2)
  const left = Math.min(Math.max(padding, trigger.left), window.innerWidth - width - padding)
  const style: Record<string, string> = {
    position: 'fixed',
    left: `${left}px`,
    width: `${width}px`,
    maxHeight: `calc(100vh - ${padding * 2}px)`,
    zIndex: '100000020'
  }
  const spaceBelow = window.innerHeight - trigger.bottom - padding
  const spaceAbove = trigger.top - padding
  if (dropdown.height > spaceBelow && spaceAbove > spaceBelow) {
    style.bottom = `${window.innerHeight - trigger.top + padding}px`
  } else {
    style.top = `${trigger.bottom + padding}px`
  }
  dropdownStyle.value = style
}

const toggle = async () => {
  isOpen.value = !isOpen.value
  if (isOpen.value) {
    await nextTick()
    updateDropdownPosition()
  }
}

const apply = () => {
  emit('update:startDate', localStartDate.value)
  emit('update:endDate', localEndDate.value)
  emit('change', {
    startDate: localStartDate.value,
    endDate: localEndDate.value,
    preset: activePreset.value
  })
  isOpen.value = false
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as Node
  if (containerRef.value && !containerRef.value.contains(target) && !dropdownRef.value?.contains(target)) {
    isOpen.value = false
  }
}

const handleEscape = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && isOpen.value) {
    isOpen.value = false
  }
}

// Sync local state with props
watch(
  () => props.startDate,
  (val) => {
    localStartDate.value = val
    onDateChange()
  }
)

watch(
  () => props.endDate,
  (val) => {
    localEndDate.value = val
    onDateChange()
  }
)

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleEscape)
  window.addEventListener('resize', updateDropdownPosition)
  window.addEventListener('scroll', updateDropdownPosition, true)
  // Initialize active preset detection
  onDateChange()
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleEscape)
  window.removeEventListener('resize', updateDropdownPosition)
  window.removeEventListener('scroll', updateDropdownPosition, true)
})
</script>

<style scoped>
.date-picker-trigger {
  @apply flex items-center gap-2;
  @apply rounded-lg px-3 py-2 text-sm;
  @apply bg-white dark:bg-dark-800;
  @apply border border-gray-200 dark:border-dark-600;
  @apply text-gray-700 dark:text-gray-300;
  @apply transition-all duration-200;
  @apply focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/30;
  @apply hover:border-gray-300 dark:hover:border-dark-500;
  @apply cursor-pointer;
}

.date-picker-trigger-open {
  @apply border-primary-500 ring-2 ring-primary-500/30;
}

.date-picker-icon {
  @apply text-gray-400 dark:text-dark-400;
}

.date-picker-value {
  @apply font-medium;
}

.date-picker-chevron {
  @apply text-gray-400 dark:text-dark-400;
}

.date-picker-dropdown {
  @apply fixed;
  @apply bg-white dark:bg-dark-800;
  @apply rounded-xl;
  @apply border border-gray-200 dark:border-dark-700;
  @apply shadow-lg shadow-black/10 dark:shadow-black/30;
  @apply overflow-x-hidden overflow-y-auto;
}

.date-picker-presets {
  @apply grid grid-cols-2 gap-1 p-2;
}

.date-picker-presets-period {
  @apply grid-cols-3;
}

.date-picker-preset {
  @apply rounded-md px-3 py-1.5 text-xs font-medium;
  @apply text-gray-600 dark:text-gray-400;
  @apply hover:bg-gray-100 dark:hover:bg-dark-700;
  @apply transition-colors duration-150;
}

.date-picker-preset-active {
  @apply bg-primary-100 dark:bg-primary-900/30;
  @apply text-primary-700 dark:text-primary-300;
}

.date-picker-divider {
  @apply border-t border-gray-100 dark:border-dark-700;
}

.date-picker-period-tabs {
  @apply grid grid-cols-2 gap-2 p-3 pb-0;
}

.date-picker-period-tab {
  @apply rounded-lg border border-gray-200 px-3 py-2 text-sm font-medium text-gray-600 dark:border-dark-600 dark:text-gray-300;
}

.date-picker-period-tab-active {
  @apply border-primary-300 bg-primary-50 text-primary-600 dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-300;
}

.date-picker-custom {
  @apply flex items-end gap-2 p-3;
}

.date-picker-field {
  @apply flex-1;
}

.date-picker-label {
  @apply mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400;
}

.date-picker-input {
  @apply w-full rounded-md px-2 py-1.5 text-sm;
  @apply bg-gray-50 dark:bg-dark-700;
  @apply border border-gray-200 dark:border-dark-600;
  @apply text-gray-900 dark:text-gray-100;
  @apply focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/30;
}

.date-picker-input::-webkit-calendar-picker-indicator {
  @apply cursor-pointer opacity-60 hover:opacity-100;
  filter: invert(0.5);
}

.dark .date-picker-input::-webkit-calendar-picker-indicator {
  filter: invert(0.7);
}

.date-picker-separator {
  @apply flex items-center justify-center pb-1;
}

.date-picker-actions {
  @apply flex justify-end p-2 pt-0;
}

.date-picker-apply {
  @apply rounded-lg px-4 py-1.5 text-sm font-medium;
  @apply bg-primary-600 text-white;
  @apply hover:bg-primary-700;
  @apply transition-colors duration-150;
}

/* Dropdown animation */
.date-picker-dropdown-enter-active,
.date-picker-dropdown-leave-active {
  transition: all 0.2s ease;
}

.date-picker-dropdown-enter-from,
.date-picker-dropdown-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
