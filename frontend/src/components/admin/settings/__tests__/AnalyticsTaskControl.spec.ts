import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AnalyticsTaskControl from '../AnalyticsTaskControl.vue'

const api = vi.hoisted(() => ({ status: vi.fn(), setEnabled: vi.fn() }))
vi.mock('@/api/admin/dataAnalytics', () => ({ analyticsTasksAPI: api }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))

function status(enabled = false, version = 1, state = 'paused') {
  return { control: { enabled, version, updated_by: 0, updated_at: null }, state, running: 0, last_success_at: null }
}

describe('AnalyticsTaskControl', () => {
  const wrappers: ReturnType<typeof mount>[] = []
  function render(editable = true) {
    const wrapper = mount(AnalyticsTaskControl, { props: { editable } })
    wrappers.push(wrapper)
    return wrapper
  }
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    api.status.mockResolvedValue(status())
    api.setEnabled.mockResolvedValue(status(true, 2).control)
  })
  afterEach(() => {
    wrappers.forEach(wrapper => wrapper.unmount())
    wrappers.length = 0
    vi.useRealTimers()
  })

  it('loads persisted control and sends the displayed version when saving', async () => {
    const wrapper = render()
    await flushPromises()
    expect(wrapper.get('[role="switch"]').attributes('aria-checked')).toBe('false')
    expect(wrapper.emitted('loaded')?.at(-1)).toEqual([status()])
    api.status.mockResolvedValue(status(true, 2, 'idle'))
    await wrapper.get('[role="switch"]').trigger('click')
    await flushPromises()
    expect(api.setEnabled).toHaveBeenCalledWith(true, 1)
    expect(wrapper.get('[role="switch"]').attributes('aria-checked')).toBe('true')
    expect(wrapper.text()).toContain('admin.settings.analyticsTasks.states.idle')
  })

  it('does not report a failed save as success and reloads on conflict', async () => {
    const wrapper = render()
    await flushPromises()
    api.setEnabled.mockRejectedValue({ status: 409 })
    api.status.mockResolvedValue(status(false, 3))
    await wrapper.get('[role="switch"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('admin.settings.analyticsTasks.conflict')
    expect(wrapper.get('[role="switch"]').attributes('aria-checked')).toBe('false')
    api.setEnabled.mockResolvedValue(status(true, 4).control)
    api.status.mockResolvedValue(status(true, 4, 'idle'))
    await wrapper.get('[role="switch"]').trigger('click')
    await flushPromises()
    expect(api.setEnabled).toHaveBeenLastCalledWith(true, 3)
  })

  it('blocks repeated clicks until the setting save finishes', async () => {
    let finish!: (value: unknown) => void
    api.setEnabled.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[role="switch"]').trigger('click')
    expect(wrapper.get('[role="switch"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[role="switch"]').trigger('click')
    expect(api.setEnabled).toHaveBeenCalledTimes(1)
    finish(status(true, 2).control)
    await flushPromises()
  })

  it('keeps the last confirmed value and requires refresh after a failed save', async () => {
    const wrapper = render()
    await flushPromises()
    api.setEnabled.mockRejectedValue(new Error('connection closed'))
    await wrapper.get('[role="switch"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="switch"]').attributes('aria-checked')).toBe('false')
    expect(wrapper.get('[role="switch"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[role="alert"]').text()).toContain('admin.settings.analyticsTasks.saveFailed')
    api.status.mockResolvedValue(status(true, 2, 'idle'))
    await wrapper.get('[data-testid="refresh-analytics-tasks"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="switch"]').attributes('aria-checked')).toBe('true')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('reports unknown status on a failed poll and disables saving until refresh succeeds', async () => {
    const wrapper = render()
    await flushPromises()
    api.status.mockRejectedValue(new Error('offline'))
    await vi.advanceTimersByTimeAsync(10_000)
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('admin.settings.analyticsTasks.loadFailed')
    expect(wrapper.get('[role="switch"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('admin.settings.analyticsTasks.states.unknown')
    expect(wrapper.emitted('loaded')?.at(-1)).toEqual([null])
    api.status.mockResolvedValue(status())
    await wrapper.get('[data-testid="refresh-analytics-tasks"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="switch"]').attributes('disabled')).toBeUndefined()
  })

  it('shows pausing until workers stop and stops polling after unmount', async () => {
    api.status.mockResolvedValue(status(false, 2, 'pausing'))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('admin.settings.analyticsTasks.states.pausing')
    wrapper.unmount()
    const count = api.status.mock.calls.length
    await vi.advanceTimersByTimeAsync(30_000)
    expect(api.status).toHaveBeenCalledTimes(count)
  })

  it('shows a read-only pause notice in cost management and hides it when enabled', async () => {
    const wrapper = render(false)
    await flushPromises()
    expect(wrapper.find('[role="switch"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.settings.analyticsTasks.pausedNotice')
    api.status.mockResolvedValue(status(true, 2, 'idle'))
    await vi.advanceTimersByTimeAsync(10_000)
    await flushPromises()
    expect(wrapper.find('[data-testid="analytics-task-control"]').exists()).toBe(false)
  })
})
