import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DirectoryPanel from '../DirectoryPanel.vue'

const api = vi.hoisted(() => ({
  directory: vi.fn(), preview: vi.fn(), apply: vi.fn(), map: vi.fn(), showSuccess: vi.fn(),
}))
vi.mock('@/api/admin/dataAnalytics', () => ({ dataAnalyticsAPI: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: api.showSuccess }) }))
const state = { version: 1, effective_at: null, directory: { orgs: [], people: [] }, identities: [], imports: [] }
const stubs = { Icon: true, Select: true }
const button = (wrapper: ReturnType<typeof mount>, text: string) =>
  wrapper.findAll('button').find(item => item.text() === text)!

beforeEach(() => {
  api.directory.mockResolvedValue(state)
  api.preview.mockResolvedValue({ id: 'preview', directory: state.directory, matches: 0, unmatched: 0, duplicate_names: 0, missing_orgs: 0 })
  api.apply.mockResolvedValue(undefined)
})
afterEach(() => vi.clearAllMocks())

describe('directory maintenance panel', () => {
  it('reports a refresh failure after application without announcing an unconfirmed directory', async () => {
    const wrapper = mount(DirectoryPanel, { props: { enabled: true, initialState: state }, global: { stubs } })
    try {
      for (const label of ['上传组织机构SQL', '上传人员SQL']) {
        const input = wrapper.get(`[aria-label="${label}"]`)
        Object.defineProperty(input.element, 'files', { value: [new File(['data'], 'directory.sql')] })
        await input.trigger('change')
      }
      await button(wrapper, '预检与匹配').trigger('click')
      await flushPromises()
      api.directory.mockRejectedValue(new Error('目录刷新失败'))
      await button(wrapper, '确认应用目录').trigger('click')
      await flushPromises()
      expect(api.apply).toHaveBeenCalledOnce()
      expect(wrapper.get('[role="alert"]').text()).toContain('目录刷新失败')
      expect(api.showSuccess).not.toHaveBeenCalled()
      expect(wrapper.emitted('changed')).toBeUndefined()
      api.directory.mockResolvedValue({ ...state, version: 2 })
      await button(wrapper, '刷新目录').trigger('click')
      await flushPromises()
      expect(wrapper.emitted('loaded')?.at(-1)).toEqual([{ ...state, version: 2 }])
      expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    } finally { wrapper.unmount() }
  })
})
