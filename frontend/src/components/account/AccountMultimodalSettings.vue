<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="mb-3">
      <label class="input-label">多模态处理</label>
      <p class="input-hint">
        按映射后的实际上游模型配置。选择视觉分组后，会复用该分组原有调度规则选择 API Key 账号并独立计费。
      </p>
    </div>

    <div class="rounded-xl border border-gray-200 bg-gray-50/60 p-4 dark:border-dark-600 dark:bg-dark-700/30">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <label class="input-label mb-0">默认规则</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">适用于未单独配置的模型</p>
        </div>
        <div class="w-full sm:w-80">
          <Select
            :model-value="modelValue.defaultMode"
            :options="modeOptions"
            @update:model-value="setDefaultMode($event as MultimodalMode)"
          />
        </div>
      </div>
      <div v-if="modelValue.defaultMode === 'vision_to_text'" class="mt-4 grid grid-cols-1 gap-3 border-t border-gray-200 pt-4 dark:border-dark-600 sm:grid-cols-2">
        <div>
          <label class="input-label">视觉分组</label>
          <Select
            :model-value="modelValue.defaultVisionGroupId"
            :options="visionGroupOptions"
            searchable
            @update:model-value="setDefaultVisionGroup(Number($event))"
          />
        </div>
        <div>
          <label class="input-label">视觉模型</label>
          <Select
            :model-value="modelValue.defaultVisionModel"
            :options="visionModelOptions(modelValue.defaultVisionGroupId, modelValue.defaultVisionModel)"
            placeholder="选择或输入视觉模型"
            searchable
            creatable
            @update:model-value="setDefaultVisionModel(String($event ?? ''))"
          />
        </div>
      </div>
    </div>

    <div v-if="modelValue.rules.length" class="mt-4 space-y-3">
      <div
        v-for="(rule, index) in modelValue.rules"
        :key="index"
        class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800"
      >
        <div class="mb-3 flex items-center justify-between">
          <div>
            <span class="text-sm font-medium text-gray-800 dark:text-gray-200">模型规则 {{ index + 1 }}</span>
            <span v-if="rule.model" class="ml-2 inline-block max-w-64 truncate rounded-md bg-gray-100 px-2 py-0.5 align-middle font-mono text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400">
              {{ rule.model }}
            </span>
          </div>
          <button
            type="button"
            class="rounded-md p-1.5 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20"
            aria-label="删除模型规则"
            title="删除"
            @click="removeRule(index)"
          >
            <Icon name="trash" size="sm" />
          </button>
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label class="input-label">实际上游模型</label>
            <Select
              :model-value="rule.model"
              :options="sourceModelOptions(rule.model)"
              placeholder="选择或输入模型"
              searchable
              creatable
              @update:model-value="updateRule(index, { model: String($event ?? '') })"
            />
          </div>
          <div>
            <label class="input-label">处理方式</label>
            <Select
              :model-value="rule.mode"
              :options="modeOptions"
              @update:model-value="updateRule(index, { mode: $event as MultimodalMode })"
            />
          </div>
          <template v-if="rule.mode === 'vision_to_text'">
            <div>
              <label class="input-label">视觉分组</label>
              <Select
                :model-value="rule.visionGroupId"
                :options="visionGroupOptions"
                searchable
                @update:model-value="setRuleVisionGroup(index, Number($event))"
              />
            </div>
            <div>
              <label class="input-label">视觉模型</label>
              <Select
                :model-value="rule.visionModel"
                :options="visionModelOptions(rule.visionGroupId, rule.visionModel)"
                placeholder="选择或输入视觉模型"
                searchable
                creatable
                @update:model-value="updateRule(index, { visionModel: String($event ?? '') })"
              />
            </div>
          </template>
        </div>
      </div>
    </div>

    <button type="button" class="btn btn-secondary mt-3 text-sm" @click="addRule">
      <Icon name="plus" size="sm" class="mr-1.5" />
      添加模型规则
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { groupsAPI } from '@/api/admin/groups'
import type {
  MultimodalConfig,
  MultimodalMode,
  MultimodalRule
} from './accountMultimodal'

const props = defineProps<{
  modelValue: MultimodalConfig
  models?: string[]
}>()

const emit = defineEmits<{
  'update:modelValue': [value: MultimodalConfig]
}>()

const visionGroups = ref<Array<{ id: number; name: string; platform: string }>>([])
const modelsByGroup = ref<Record<number, string[]>>({})

const modeOptions: SelectOption[] = [
  { value: 'passthrough', label: '交给上游处理（图片透传）' },
  { value: 'vision_to_text', label: '视觉模型转文本' },
  { value: 'reject', label: '不支持图片（跳过此账号）' }
]

const visionGroupOptions = computed<SelectOption[]>(() => [
  { value: 0, label: '当前账号（仅 API Key）' },
  ...visionGroups.value.map((group) => ({
    value: group.id,
    label: `${group.name} · ${group.platform}`
  }))
])

const sourceModelOptions = (selected: string): SelectOption[] =>
  [...new Set([...(props.models || []), selected].map((model) => model.trim()).filter(Boolean))]
    .map((model) => ({ value: model, label: model }))

const visionModelOptions = (groupId: number, selected: string): SelectOption[] => {
  const models = groupId > 0 ? modelsByGroup.value[groupId] || [] : props.models || []
  return [...new Set([...models, selected].map((model) => model.trim()).filter(Boolean))]
    .map((model) => ({ value: model, label: model }))
}

const loadVisionModels = async (groupId: number) => {
  if (groupId <= 0 || modelsByGroup.value[groupId]) return
  try {
    modelsByGroup.value[groupId] = await groupsAPI.getModelsListCandidates(groupId)
  } catch {
    modelsByGroup.value[groupId] = []
  }
}

onMounted(async () => {
  try {
    visionGroups.value = (await groupsAPI.getAll())
      .filter((group) => ['openai', 'anthropic', 'composite'].includes(group.platform))
  } catch {
    visionGroups.value = []
  }
})

watch(
  () => [
    props.modelValue.defaultVisionGroupId,
    ...props.modelValue.rules.map((rule) => rule.visionGroupId)
  ],
  (groupIds) => groupIds.forEach((groupId) => void loadVisionModels(groupId)),
  { immediate: true }
)

const setDefaultMode = (defaultMode: MultimodalMode) => {
  emit('update:modelValue', { ...props.modelValue, defaultMode })
}

const setDefaultVisionGroup = (defaultVisionGroupId: number) => {
  emit('update:modelValue', {
    ...props.modelValue,
    defaultVisionGroupId,
    defaultVisionModel: ''
  })
}

const setDefaultVisionModel = (defaultVisionModel: string) => {
  emit('update:modelValue', { ...props.modelValue, defaultVisionModel })
}

const updateRule = (index: number, patch: Partial<MultimodalRule>) => {
  const rules = props.modelValue.rules.map((rule, i) =>
    i === index ? { ...rule, ...patch } : rule
  )
  emit('update:modelValue', { ...props.modelValue, rules })
}

const setRuleVisionGroup = (index: number, visionGroupId: number) => {
  updateRule(index, { visionGroupId, visionModel: '' })
}

const addRule = () => {
  const used = new Set(props.modelValue.rules.map((rule) => rule.model))
  const model = props.models?.find((item) => !used.has(item)) || ''
  emit('update:modelValue', {
    ...props.modelValue,
    rules: [...props.modelValue.rules, {
      model,
      mode: 'reject',
      visionGroupId: 0,
      visionModel: ''
    }]
  })
}

const removeRule = (index: number) => {
  emit('update:modelValue', {
    ...props.modelValue,
    rules: props.modelValue.rules.filter((_, i) => i !== index)
  })
}
</script>
