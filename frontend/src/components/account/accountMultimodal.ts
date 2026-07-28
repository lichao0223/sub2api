export type MultimodalMode = 'passthrough' | 'vision_to_text' | 'reject'

export interface MultimodalRule {
  model: string
  mode: MultimodalMode
  visionGroupId: number
  visionModel: string
}

export interface MultimodalConfig {
  defaultMode: MultimodalMode
  defaultVisionGroupId: number
  defaultVisionModel: string
  rules: MultimodalRule[]
}

export const defaultMultimodalConfig = (): MultimodalConfig => ({
  defaultMode: 'passthrough',
  defaultVisionGroupId: 0,
  defaultVisionModel: '',
  rules: []
})

export const readMultimodalConfig = (
  credentials?: Record<string, unknown>
): MultimodalConfig => {
  const rawDefaultMode = credentials?.multimodal_default_mode
  const defaultMode: MultimodalMode =
    rawDefaultMode === 'reject' || rawDefaultMode === 'vision_to_text'
      ? rawDefaultMode
      : 'passthrough'
  const defaultVisionModel =
    typeof credentials?.multimodal_default_vision_model === 'string'
      ? credentials.multimodal_default_vision_model
      : ''
  const defaultVisionGroupId =
    typeof credentials?.multimodal_default_vision_group_id === 'number'
      ? credentials.multimodal_default_vision_group_id
      : 0
  const rawModes = credentials?.multimodal_model_modes
  const rawVisionModels =
    credentials?.multimodal_vision_models &&
    typeof credentials.multimodal_vision_models === 'object' &&
    !Array.isArray(credentials.multimodal_vision_models)
      ? credentials.multimodal_vision_models as Record<string, unknown>
      : {}
  const rawVisionGroups =
    credentials?.multimodal_vision_group_ids &&
    typeof credentials.multimodal_vision_group_ids === 'object' &&
    !Array.isArray(credentials.multimodal_vision_group_ids)
      ? credentials.multimodal_vision_group_ids as Record<string, unknown>
      : {}
  const rules =
    rawModes && typeof rawModes === 'object' && !Array.isArray(rawModes)
      ? Object.entries(rawModes)
          .filter((entry): entry is [string, MultimodalMode] =>
            entry[1] === 'passthrough' ||
            entry[1] === 'vision_to_text' ||
            entry[1] === 'reject'
          )
          .map(([model, mode]) => ({
            model,
            mode,
            visionGroupId:
              typeof rawVisionGroups[model] === 'number' ? rawVisionGroups[model] : 0,
            visionModel:
              typeof rawVisionModels[model] === 'string' ? rawVisionModels[model] : ''
          }))
      : []
  return { defaultMode, defaultVisionGroupId, defaultVisionModel, rules }
}

export const applyMultimodalConfig = (
  credentials: Record<string, unknown>,
  config: MultimodalConfig
) => {
  if (config.defaultMode !== 'passthrough') {
    credentials.multimodal_default_mode = config.defaultMode
  } else {
    delete credentials.multimodal_default_mode
  }
  if (config.defaultMode === 'vision_to_text' && config.defaultVisionModel.trim()) {
    credentials.multimodal_default_vision_model = config.defaultVisionModel.trim()
  } else {
    delete credentials.multimodal_default_vision_model
  }
  if (config.defaultMode === 'vision_to_text' && config.defaultVisionGroupId > 0) {
    credentials.multimodal_default_vision_group_id = config.defaultVisionGroupId
  } else {
    delete credentials.multimodal_default_vision_group_id
  }

  const modes = Object.fromEntries(
    config.rules
      .map(({ model, mode }) => [model.trim(), mode])
      .filter(([model]) => model)
  )
  if (Object.keys(modes).length) {
    credentials.multimodal_model_modes = modes
  } else {
    delete credentials.multimodal_model_modes
  }

  const visionModels = Object.fromEntries(
    config.rules
      .filter(({ mode }) => mode === 'vision_to_text')
      .map(({ model, visionModel }) => [model.trim(), visionModel.trim()])
      .filter(([model, visionModel]) => model && visionModel)
  )
  if (Object.keys(visionModels).length) {
    credentials.multimodal_vision_models = visionModels
  } else {
    delete credentials.multimodal_vision_models
  }

  const visionGroups = Object.fromEntries(
    config.rules
      .filter(({ mode, visionGroupId }) => mode === 'vision_to_text' && visionGroupId > 0)
      .map(({ model, visionGroupId }) => [model.trim(), visionGroupId])
      .filter(([model]) => model)
  )
  if (Object.keys(visionGroups).length) {
    credentials.multimodal_vision_group_ids = visionGroups
  } else {
    delete credentials.multimodal_vision_group_ids
  }
}
