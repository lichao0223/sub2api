import { beforeEach, describe, expect, it, vi } from 'vitest'

const client = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn(), post: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: client }))

import api from '../api'

describe('work insight api', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    client.get.mockResolvedValue({ data: {} })
    client.put.mockResolvedValue({ data: {} })
    client.post.mockResolvedValue({ data: {} })
  })

  it('uses the isolated admin endpoints and omits empty filters', async () => {
    await api.getConfig()
    await api.getRuntime()
    await api.analyzeNow()
    await api.listSamples()
    await api.listBatches()
    await api.listAnalyzerAccounts()
    await api.probe({} as never)
    const filters = { start_date: '2026-08-01', end_date: '', user_name: '', task_category: '问题排查', project_name: '' }
    await api.listRanking(filters, 2, 20)
    await api.getOverview(filters)
    await api.getDaily(7)

    expect(client.get).toHaveBeenNthCalledWith(1, '/admin/ai-work-insights/config')
    expect(client.get).toHaveBeenNthCalledWith(2, '/admin/ai-work-insights/runtime')
    expect(client.post).toHaveBeenCalledWith('/admin/ai-work-insights/analyze-now')
    expect(client.get).toHaveBeenNthCalledWith(3, '/admin/ai-work-insights/samples', { params: { page_size: 50 } })
    expect(client.get).toHaveBeenNthCalledWith(4, '/admin/ai-work-insights/batches', { params: { page_size: 50 } })
    expect(client.get).toHaveBeenNthCalledWith(5, '/admin/ai-work-insights/analyzer-accounts')
    expect(client.post).toHaveBeenCalledWith('/admin/ai-work-insights/endpoint/probe', {})
    expect(client.get).toHaveBeenNthCalledWith(6, '/admin/ai-work-insights/ranking', { params: { start_date: '2026-08-01', task_category: '问题排查', page: 2, page_size: 20 } })
    expect(client.get).toHaveBeenNthCalledWith(7, '/admin/ai-work-insights/overview', { params: { start_date: '2026-08-01', task_category: '问题排查' } })
    expect(client.get).toHaveBeenNthCalledWith(8, '/admin/ai-work-insights/daily/7')
  })
})
