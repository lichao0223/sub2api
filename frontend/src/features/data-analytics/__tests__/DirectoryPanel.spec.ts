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
  it.each([
    { error: { status: 400, message: '缺少字段dev_org_id' }, message: '缺少字段dev_org_id' },
    { error: { status: 413, message: 'SQL文件超过20MiB' }, message: 'SQL文件超过20MiB' },
    { error: { status: 0, message: 'Network error. Please check your connection.' }, message: 'Network error. Please check your connection.' },
    { error: new Error('SQL字符串未闭合'), message: 'SQL字符串未闭合' },
    { error: {}, message: 'SQL预检失败' },
  ])('shows the preview failure reason: $message', async ({ error, message }) => {
    api.preview.mockRejectedValueOnce(error)
    const wrapper = mount(DirectoryPanel, { props: { enabled: true, initialState: state }, global: { stubs } })
    try {
      for (const label of ['上传组织机构SQL', '上传人员SQL']) {
        const input = wrapper.get(`[aria-label="${label}"]`)
        Object.defineProperty(input.element, 'files', { value: [new File(['data'], 'directory.sql')] })
        await input.trigger('change')
      }
      await button(wrapper, '预检与匹配').trigger('click')
      await flushPromises()
      expect(wrapper.get('[role="alert"]').text()).toContain(message)
      expect(button(wrapper, '预检与匹配').attributes('disabled')).toBeUndefined()
      expect(wrapper.text()).not.toContain('确认应用目录')
      expect(api.apply).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('shows a structured API error when loading the directory fails', async () => {
    api.directory.mockRejectedValueOnce({ status: 503, message: '目录服务暂不可用' })
    const wrapper = mount(DirectoryPanel, { props: { enabled: true }, global: { stubs } })
    try {
      await flushPromises()
      expect(wrapper.get('[role="alert"]').text()).toContain('目录服务暂不可用')
    } finally { wrapper.unmount() }
  })

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
      api.directory.mockRejectedValue({ status: 503, message: '目录刷新失败' })
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

  it('keeps the preview and displays the reason when applying the directory fails', async () => {
    api.apply.mockRejectedValueOnce({ status: 409, message: '目录版本已变化，请刷新后重试' })
    const wrapper = mount(DirectoryPanel, { props: { enabled: true, initialState: state }, global: { stubs } })
    try {
      for (const label of ['上传组织机构SQL', '上传人员SQL']) {
        const input = wrapper.get(`[aria-label="${label}"]`)
        Object.defineProperty(input.element, 'files', { value: [new File(['data'], 'directory.sql')] })
        await input.trigger('change')
      }
      await button(wrapper, '预检与匹配').trigger('click')
      await flushPromises()
      await button(wrapper, '确认应用目录').trigger('click')
      await flushPromises()
      expect(wrapper.get('[role="alert"]').text()).toContain('目录版本已变化，请刷新后重试')
      expect(button(wrapper, '确认应用目录').attributes('disabled')).toBeUndefined()
      expect(api.showSuccess).not.toHaveBeenCalled()
      expect(wrapper.emitted('changed')).toBeUndefined()
    } finally { wrapper.unmount() }
  })

  it('shows the reason when saving an identity match fails', async () => {
    api.map.mockRejectedValueOnce({ status: 409, message: '统计已暂停，请先在系统设置开启后台统计任务' })
    const wrapper = mount(DirectoryPanel, {
      props: { enabled: true, initialState: { ...state, identities: [{ user_id: 7, username: '张三', person_id: 'person-1', basis: 'unique_name' }] } },
      global: { stubs },
    })
    try {
      await button(wrapper, '选择人员').trigger('click')
      await button(wrapper, '确认匹配').trigger('click')
      await flushPromises()
      expect(wrapper.get('[role="alert"]').text()).toContain('统计已暂停，请先在系统设置开启后台统计任务')
      expect(api.showSuccess).not.toHaveBeenCalled()
      expect(wrapper.emitted('changed')).toBeUndefined()
    } finally { wrapper.unmount() }
  })
})
