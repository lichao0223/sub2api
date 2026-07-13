import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import TokenRankingView from '../TokenRankingView.vue'

const push = vi.hoisted(() => vi.fn())
const getDashboardNonworkTokenRanking = vi.hoisted(() => vi.fn())
const authStore = vi.hoisted(() => ({ isAdmin: true }))

vi.mock('vue-router', async () => ({
  ...(await vi.importActual<typeof import('vue-router')>('vue-router')),
  useRouter: () => ({ push }),
}))

vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => authStore }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showWarning: vi.fn(), showSuccess: vi.fn(), showError: vi.fn() }),
}))
vi.mock('@/api/usage', () => ({
  usageAPI: { getDashboardNonworkTokenRanking },
}))
vi.mock('@/api/admin/usage', () => ({ default: {}, adminUsageAPI: {} }))

const mountView = () => mount(TokenRankingView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      DateRangePicker: true,
      LoadingSpinner: true,
      Pagination: true,
      Select: true,
      Icon: true,
    },
  },
})

describe('TokenRankingView user drill-down', () => {
  beforeEach(() => {
    push.mockReset()
    authStore.isAdmin = true
    getDashboardNonworkTokenRanking.mockResolvedValue({
      ranking: [{ user_id: 7, email: 'user@test.com', username: 'Test User', requests: 2, tokens: 3, actual_cost: 1 }],
      total_requests: 2,
      total_tokens: 3,
      total_actual_cost: 1,
      start_date: '2026-07-01',
      end_date: '2026-07-07',
    })
  })

  it('opens admin usage details only for administrators', async () => {
    const adminView = mountView()
    await flushPromises()

    const adminRow = adminView.find('tbody tr')
    expect(adminRow.attributes('tabindex')).toBe('0')
    await adminRow.trigger('click')
    expect(push).toHaveBeenCalledWith({
      path: '/admin/usage',
      query: { user_id: '7', start_date: '2026-07-01', end_date: '2026-07-07' },
    })
    adminView.unmount()

    push.mockReset()
    authStore.isAdmin = false
    const userView = mountView()
    await flushPromises()

    const userRow = userView.find('tbody tr')
    expect(userRow.attributes('tabindex')).toBeUndefined()
    await userRow.trigger('click')
    expect(push).not.toHaveBeenCalled()
  })
})
