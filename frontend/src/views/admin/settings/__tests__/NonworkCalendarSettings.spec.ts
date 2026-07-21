import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import NonworkCalendarSettings from '../NonworkCalendarSettings.vue'

const { getStatus, getDays, getStats, sync, overrideDay, backfill, showError, showSuccess } = vi.hoisted(() => ({
  getStatus: vi.fn(),
  getDays: vi.fn(),
  getStats: vi.fn(),
  sync: vi.fn(),
  overrideDay: vi.fn(),
  backfill: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin/usage', () => ({
  adminUsageAPI: {
    getNonworkCalendarStatus: getStatus,
    getNonworkCalendarDays: getDays,
    getNonworkStatsStatus: getStats,
    syncNonworkCalendar: sync,
    overrideNonworkCalendarDay: overrideDay,
    backfillNonworkUsage: backfill
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

describe('NonworkCalendarSettings', () => {
  const year = new Date().getFullYear()

  beforeEach(() => {
    vi.clearAllMocks()
    getStatus.mockResolvedValue({
      years: [{ year, total_days: 365, confirmed_days: 365, predicted_days: 0, confirmed: true }]
    })
    getDays.mockResolvedValue({
      year,
      days: [
        {
          date: `${year}-01-01`,
          day_type: 'holiday_offday',
          holiday_name: '元旦',
          source: 'holiday-cn',
          confirmed: true,
          manual_override: false
        },
        {
          date: `${year}-02-01`,
          day_type: 'manual_offday',
          holiday_name: '公司假期',
          source: 'manual',
          confirmed: true,
          manual_override: true
        }
      ]
    })
    getStats.mockResolvedValue({ coverage: { complete: true, aggregated_days: 365, total_days: 365 } })
    overrideDay.mockResolvedValue({ status: 'accepted' })
  })

  it('loads annual status, offdays and coverage', async () => {
    const wrapper = mount(NonworkCalendarSettings)
    await flushPromises()

    expect(getStatus).toHaveBeenCalledWith([year - 1, year, year + 1])
    expect(getDays).toHaveBeenCalledWith(year)
    expect(getStats).toHaveBeenCalledWith(expect.objectContaining({
      start_date: `${year}-01-01`,
      end_date: `${year}-12-31`
    }))
    expect(wrapper.text()).toContain('元旦')
    expect(wrapper.text()).toContain('公司假期')
    expect(wrapper.text()).toContain('admin.usage.removeOffday')
  })

  it('adds a manual offday and can remove it', async () => {
    const wrapper = mount(NonworkCalendarSettings)
    await flushPromises()

    await wrapper.find('input[type="date"]').setValue(`${year}-03-08`)
    await wrapper.find('input[type="text"]').setValue('妇女节')
    await wrapper.findAll('button').find((button) => button.text() === 'admin.usage.addOffday')!.trigger('click')
    await flushPromises()

    expect(overrideDay).toHaveBeenCalledWith(expect.objectContaining({
      date: `${year}-03-08`,
      is_workday: false,
      holiday_name: '妇女节'
    }))

    await wrapper.findAll('button').find((button) => button.text() === 'admin.usage.removeOffday')!.trigger('click')
    await flushPromises()

    expect(overrideDay).toHaveBeenLastCalledWith(expect.objectContaining({
      date: `${year}-02-01`,
      is_workday: true
    }))
  })
})
