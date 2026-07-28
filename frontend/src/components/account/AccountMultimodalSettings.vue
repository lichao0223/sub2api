<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="mb-3">
      <label class="input-label">多模态处理</label>
      <p class="input-hint">
        按映射后的实际上游模型配置。视觉转文本仅支持 API Key 账号，使用同账号视觉模型并独立计费。
      </p>
    </div>

    <div>
      <label class="input-label">未单独配置的模型</label>
      <select
        :value="modelValue.defaultMode"
        class="input"
        @change="setDefaultMode(($event.target as HTMLSelectElement).value as MultimodalMode)"
      >
        <option value="passthrough">交给上游处理（图片透传）</option>
        <option v-if="allowVisionToText" value="vision_to_text">视觉模型转文本</option>
        <option value="reject">不支持图片（跳过此账号）</option>
      </select>
    </div>
    <div v-if="modelValue.defaultMode === 'vision_to_text'" class="mt-2">
      <label class="input-label">默认视觉模型</label>
      <input
        :value="modelValue.defaultVisionModel"
        :list="datalistId"
        class="input font-mono"
        placeholder="例如 gpt-4.1-mini"
        required
        @input="setDefaultVisionModel(($event.target as HTMLInputElement).value)"
      />
    </div>

    <div v-if="modelValue.rules.length" class="mt-3 space-y-2">
      <div
        v-for="(rule, index) in modelValue.rules"
        :key="index"
        class="flex flex-wrap items-center gap-2"
      >
        <input
          :value="rule.model"
          :list="datalistId"
          class="input flex-1 font-mono"
          placeholder="实际上游模型"
          @input="updateRule(index, { model: ($event.target as HTMLInputElement).value })"
        />
        <select
          :value="rule.mode"
          class="input w-48"
          @change="updateRule(index, { mode: ($event.target as HTMLSelectElement).value as MultimodalMode })"
        >
          <option value="passthrough">交给上游处理</option>
          <option v-if="allowVisionToText" value="vision_to_text">视觉模型转文本</option>
          <option value="reject">不支持图片</option>
        </select>
        <input
          v-if="rule.mode === 'vision_to_text'"
          :value="rule.visionModel"
          :list="datalistId"
          class="input flex-1 font-mono"
          placeholder="视觉模型（必填）"
          required
          @input="updateRule(index, { visionModel: ($event.target as HTMLInputElement).value })"
        />
        <button type="button" class="text-red-500 hover:text-red-700" @click="removeRule(index)">
          删除
        </button>
      </div>
    </div>

    <datalist :id="datalistId">
      <option v-for="model in models" :key="model" :value="model" />
    </datalist>
    <button type="button" class="btn btn-secondary mt-3 text-sm" @click="addRule">
      添加模型规则
    </button>
  </div>
</template>

<script setup lang="ts">
import type {
  MultimodalConfig,
  MultimodalMode,
  MultimodalRule
} from './accountMultimodal'

const props = defineProps<{
  modelValue: MultimodalConfig
  models?: string[]
  allowVisionToText?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: MultimodalConfig]
}>()

const datalistId = `multimodal-models-${Math.random().toString(36).slice(2)}`

const setDefaultMode = (defaultMode: MultimodalMode) => {
  emit('update:modelValue', { ...props.modelValue, defaultMode })
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

const addRule = () => {
  const used = new Set(props.modelValue.rules.map((rule) => rule.model))
  const model = props.models?.find((item) => !used.has(item)) || ''
  emit('update:modelValue', {
    ...props.modelValue,
    rules: [...props.modelValue.rules, { model, mode: 'reject', visionModel: '' }]
  })
}

const removeRule = (index: number) => {
  emit('update:modelValue', {
    ...props.modelValue,
    rules: props.modelValue.rules.filter((_, i) => i !== index)
  })
}
</script>
