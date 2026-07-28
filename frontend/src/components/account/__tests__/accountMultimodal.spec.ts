import { describe, expect, it } from 'vitest'
import {
  applyMultimodalConfig,
  readMultimodalConfig
} from '../accountMultimodal'

describe('account multimodal credentials', () => {
  it('round-trips mapped upstream model rules', () => {
    const credentials: Record<string, unknown> = {}
    applyMultimodalConfig(credentials, {
      defaultMode: 'passthrough',
      defaultVisionGroupId: 0,
      defaultVisionModel: '',
      rules: [{
        model: 'qwen3.6-35b',
        mode: 'vision_to_text',
        visionGroupId: 7,
        visionModel: 'gpt-4.1-mini'
      }]
    })

    expect(credentials).toEqual({
      multimodal_model_modes: { 'qwen3.6-35b': 'vision_to_text' },
      multimodal_vision_models: { 'qwen3.6-35b': 'gpt-4.1-mini' },
      multimodal_vision_group_ids: { 'qwen3.6-35b': 7 }
    })
    expect(readMultimodalConfig(credentials).rules).toEqual([
      {
        model: 'qwen3.6-35b',
        mode: 'vision_to_text',
        visionGroupId: 7,
        visionModel: 'gpt-4.1-mini'
      }
    ])
  })
})
