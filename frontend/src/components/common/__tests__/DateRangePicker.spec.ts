import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'

import DateRangePicker from '../DateRangePicker.vue'

const messages: Record<string, string> = {
  'dates.today': 'Today',
  'dates.yesterday': 'Yesterday',
  'dates.last24Hours': 'Last 24 Hours',
  'dates.last7Days': 'Last 7 Days',
  'dates.last14Days': 'Last 14 Days',
  'dates.last30Days': 'Last 30 Days',
  'dates.thisMonth': 'This Month',
  'dates.lastMonth': 'Last Month',
  'dates.thisWeek': 'This Week',
  'dates.thisYear': 'This Year',
  'dates.lastYear': 'Last Year',
  'dates.startDate': 'Start Date',
  'dates.endDate': 'End Date',
  'dates.selectByDay': 'Select by Day',
  'dates.selectByMonth': 'Select by Month',
  'dates.selectByYear': 'Select by Year',
  'dates.startMonth': 'Start Month',
  'dates.endMonth': 'End Month',
  'dates.startYear': 'Start Year',
  'dates.endYear': 'End Year',
  'dates.apply': 'Apply',
  'dates.selectDateRange': 'Select date range'
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => messages[key] ?? key,
    locale: ref('en')
  })
}))

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

describe('DateRangePicker', () => {
  it('uses last 24 hours as the default recognized preset', () => {
    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: formatLocalDate(yesterday),
        endDate: formatLocalDate(now)
      },
      global: {
        stubs: {
          Icon: true,
          Teleport: true
        }
      }
    })

    expect(wrapper.text()).toContain('Last 24 Hours')
  })

  it('emits range updates with last24Hours preset when applied', async () => {
    const now = new Date()
    const today = formatLocalDate(now)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: today,
        endDate: today
      },
      global: {
        stubs: {
          Icon: true,
          Teleport: true
        }
      }
    })

    await wrapper.find('.date-picker-trigger').trigger('click')
    const presetButton = wrapper.findAll('.date-picker-preset').find((node) =>
      node.text().includes('Last 24 Hours')
    )
    expect(presetButton).toBeDefined()

    await presetButton!.trigger('click')
    await wrapper.find('.date-picker-apply').trigger('click')

    const nowAfterClick = new Date()
    const yesterdayAfterClick = new Date(nowAfterClick.getTime() - 24 * 60 * 60 * 1000)
    const expectedStart = formatLocalDate(yesterdayAfterClick)
    const expectedEnd = formatLocalDate(nowAfterClick)

    expect(wrapper.emitted('update:startDate')?.[0]).toEqual([expectedStart])
    expect(wrapper.emitted('update:endDate')?.[0]).toEqual([expectedEnd])
    expect(wrapper.emitted('change')?.[0]).toEqual([
      {
        startDate: expectedStart,
        endDate: expectedEnd,
        preset: 'last24Hours'
      }
    ])
  })

  it('teleports above the trigger when the dialog has no room below', async () => {
    const rect = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function () {
      if ((this as HTMLElement).classList.contains('date-picker-dropdown')) {
        return { top: 0, bottom: 280, left: 0, right: 320, width: 320, height: 280, x: 0, y: 0, toJSON: () => ({}) }
      }
      return { top: 650, bottom: 690, left: 80, right: 260, width: 180, height: 40, x: 80, y: 650, toJSON: () => ({}) }
    })
    const wrapper = mount(DateRangePicker, {
      attachTo: document.body,
      props: { startDate: '2026-07-01', endDate: '2026-07-29' },
      global: { stubs: { Icon: true } }
    })

    await wrapper.find('.date-picker-trigger').trigger('click')
    const dropdown = document.body.querySelector<HTMLElement>('.date-picker-dropdown')
    expect(dropdown).not.toBeNull()
    expect(dropdown!.style.position).toBe('fixed')
    expect(dropdown!.style.bottom).not.toBe('')

    wrapper.unmount()
    rect.mockRestore()
  })

  it('offers day, month and year selection in period mode', async () => {
    const now = new Date()
    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: formatLocalDate(new Date(now.getFullYear(), now.getMonth(), 1)),
        endDate: formatLocalDate(now),
        periodMode: true
      },
      global: { stubs: { Icon: true, Teleport: true } }
    })

    expect(wrapper.text()).toContain('This Month')
    await wrapper.find('.date-picker-trigger').trigger('click')
    expect(wrapper.text()).toContain('This Week')
    expect(wrapper.text()).toContain('This Year')
    expect(wrapper.text()).toContain('Last Year')
    expect(wrapper.text()).toContain('Start Month')
    await wrapper.findAll('.date-picker-period-tab').at(0)!.trigger('click')
    expect(wrapper.text()).toContain('Start Date')
    expect(wrapper.text()).toContain('End Date')
    expect(wrapper.findAll('input[type="date"]')).toHaveLength(2)
    await wrapper.findAll('.date-picker-period-tab').at(2)!.trigger('click')
    expect(wrapper.text()).toContain('Start Year')
    expect(wrapper.text()).toContain('End Year')
  })
})
