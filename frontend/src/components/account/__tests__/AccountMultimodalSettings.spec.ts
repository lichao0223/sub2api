import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'

const getAll = vi.hoisted(() => vi.fn())
const getAvailableModels = vi.hoisted(() => vi.fn())

vi.mock('@/api/admin/groups', () => ({
  groupsAPI: { getAll, getAvailableModels }
}))

import Select from '@/components/common/Select.vue'
import AccountMultimodalSettings from '../AccountMultimodalSettings.vue'

const i18n = createI18n({ legacy: false, locale: 'en', messages: { en: {} } })

describe('AccountMultimodalSettings', () => {
  it('allows composite vision groups', async () => {
    getAll.mockResolvedValue([
      { id: 1, name: 'OpenAI', platform: 'openai' },
      { id: 2, name: 'Composite', platform: 'composite' },
      { id: 3, name: 'Gemini', platform: 'gemini' }
    ])

    const wrapper = mount(AccountMultimodalSettings, {
      props: {
        modelValue: {
          defaultMode: 'vision_to_text',
          defaultVisionGroupId: 0,
          defaultVisionModel: '',
          rules: []
        }
      },
      global: { plugins: [i18n] }
    })
    await flushPromises()

    const groupSelect = wrapper.findAllComponents(Select)[1]
    expect(groupSelect.exists()).toBe(true)
    const options = groupSelect.props('options') as Array<{ value: number }>
    expect(options.map(option => option.value)).toEqual([0, 1, 2])
  })

  it('loads models exposed by the selected vision group', async () => {
    getAll.mockResolvedValue([{ id: 2, name: 'Composite', platform: 'composite' }])
    getAvailableModels.mockResolvedValue(['gpt-vision'])

    const wrapper = mount(AccountMultimodalSettings, {
      props: {
        modelValue: {
          defaultMode: 'vision_to_text',
          defaultVisionGroupId: 2,
          defaultVisionModel: '',
          rules: []
        }
      },
      global: { plugins: [i18n] }
    })
    await flushPromises()

    expect(getAvailableModels).toHaveBeenCalledWith(2)
    const modelSelect = wrapper.findAllComponents(Select)[2]
    expect(modelSelect.props('options')).toEqual([{ value: 'gpt-vision', label: 'gpt-vision' }])
  })
})
