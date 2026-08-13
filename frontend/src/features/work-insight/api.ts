import { apiClient } from '@/api/client'
import type {
  DailyInsightDetail,
  DailyInsightFilters,
  RepresentativeItem,
  ProbeResult,
  WorkInsightConfig,
  WorkInsightRuntime,
  AnalyzerAccount,
  WorkInsightOverview,
  SampleSummary,
  BatchSummary,
  LogPage,
  UserInsightRankingPage,
} from './types'

const basePath = '/admin/ai-work-insights'

export async function getConfig(): Promise<WorkInsightConfig> {
  const { data } = await apiClient.get<WorkInsightConfig>(`${basePath}/config`)
  return data
}

export async function updateConfig(config: WorkInsightConfig): Promise<WorkInsightConfig> {
  const { data } = await apiClient.put<WorkInsightConfig>(`${basePath}/config`, config)
  return data
}

export async function getRuntime(): Promise<WorkInsightRuntime> {
  const { data } = await apiClient.get<WorkInsightRuntime>(`${basePath}/runtime`)
  return data
}

export async function analyzeNow(): Promise<{ created_batches: number }> {
  const { data } = await apiClient.post<{ created_batches: number }>(`${basePath}/analyze-now`)
  return data
}

export async function listSamples(page = 1, pageSize = 20): Promise<LogPage<SampleSummary>> {
  const { data } = await apiClient.get<LogPage<SampleSummary>>(`${basePath}/samples`, { params: { page, page_size: pageSize } })
  return data
}

export async function listBatches(page = 1, pageSize = 20, kind: 'pending' | 'processing' | 'done' | 'errors' = 'done'): Promise<LogPage<BatchSummary>> {
  const { data } = await apiClient.get<LogPage<BatchSummary>>(`${basePath}/batches`, { params: { page, page_size: pageSize, kind } })
  return data
}

export async function clearLogs(): Promise<{ samples: number; batches: number }> {
  const { data } = await apiClient.delete<{ samples: number; batches: number }>(`${basePath}/logs`)
  return data
}

export async function retryBatch(id: number): Promise<{ batch_id: number }> {
  const { data } = await apiClient.post<{ batch_id: number }>(`${basePath}/batches/${id}/retry`)
  return data
}

export async function retryAllBatches(): Promise<{ retried: number; skipped: number }> {
  const { data } = await apiClient.post<{ retried: number; skipped: number }>(`${basePath}/batches/retry-all`)
  return data
}

export async function stopBatch(id: number): Promise<{ batch_id: number }> {
  const { data } = await apiClient.post<{ batch_id: number }>(`${basePath}/batches/${id}/stop`)
  return data
}

export async function deleteBatch(id: number): Promise<{ batch_id: number }> {
  const { data } = await apiClient.delete<{ batch_id: number }>(`${basePath}/batches/${id}`)
  return data
}

export async function deleteAllFailedBatches(): Promise<{ batches: number }> {
  const { data } = await apiClient.delete<{ batches: number }>(`${basePath}/batches/errors`)
  return data
}

export async function listAnalyzerAccounts(): Promise<AnalyzerAccount[]> {
  const { data } = await apiClient.get<AnalyzerAccount[]>(`${basePath}/analyzer-accounts`)
  return data
}

export async function probe(config: WorkInsightConfig): Promise<ProbeResult> {
  const { data } = await apiClient.post<ProbeResult>(`${basePath}/endpoint/probe`, config)
  return data
}

export async function listRanking(filters: DailyInsightFilters, page: number, pageSize: number): Promise<UserInsightRankingPage> {
  const params = Object.fromEntries(Object.entries({ ...filters, page, page_size: pageSize }).filter(([, value]) => value !== ''))
  const { data } = await apiClient.get<UserInsightRankingPage>(`${basePath}/ranking`, { params })
  return data
}

export async function getOverview(filters: DailyInsightFilters): Promise<WorkInsightOverview> {
  const params = Object.fromEntries(Object.entries(filters).filter(([, value]) => value !== ''))
  const { data } = await apiClient.get<WorkInsightOverview>(`${basePath}/overview`, { params })
  return data
}

export async function getDaily(id: number): Promise<DailyInsightDetail> {
  const { data } = await apiClient.get<DailyInsightDetail>(`${basePath}/daily/${id}`)
  return data
}

export async function listRepresentativeItems(id: number, page: number, pageSize: number): Promise<{ items: RepresentativeItem[]; total: number }> {
  const { data } = await apiClient.get<{ items: RepresentativeItem[]; total: number }>(`${basePath}/daily/${id}/representative-items`, { params: { page, page_size: pageSize } })
  return data
}

export default { getConfig, updateConfig, getRuntime, analyzeNow, listSamples, listBatches, clearLogs, retryBatch, retryAllBatches, stopBatch, deleteBatch, deleteAllFailedBatches, listAnalyzerAccounts, probe, listRanking, getOverview, getDaily, listRepresentativeItems }
