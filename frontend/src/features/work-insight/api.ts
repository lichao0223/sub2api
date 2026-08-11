import { apiClient } from '@/api/client'
import type {
  DailyInsightDetail,
  DailyInsightFilters,
  DailyInsightPage,
  RepresentativeItem,
  ProbeResult,
  WorkInsightConfig,
  WorkInsightRuntime,
  AnalyzerAccount,
  WorkInsightOverview,
  SampleSummary,
  BatchSummary,
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

export async function listSamples(pageSize = 50): Promise<SampleSummary[]> {
  const { data } = await apiClient.get<{ items: SampleSummary[] }>(`${basePath}/samples`, { params: { page_size: pageSize } })
  return data.items
}

export async function listBatches(pageSize = 50): Promise<BatchSummary[]> {
  const { data } = await apiClient.get<{ items: BatchSummary[] }>(`${basePath}/batches`, { params: { page_size: pageSize } })
  return data.items
}

export async function listAnalyzerAccounts(): Promise<AnalyzerAccount[]> {
  const { data } = await apiClient.get<AnalyzerAccount[]>(`${basePath}/analyzer-accounts`)
  return data
}

export async function probe(config: WorkInsightConfig): Promise<ProbeResult> {
  const { data } = await apiClient.post<ProbeResult>(`${basePath}/endpoint/probe`, config)
  return data
}

export async function listDaily(filters: DailyInsightFilters, page: number, pageSize: number): Promise<DailyInsightPage> {
  const params = Object.fromEntries(Object.entries({ ...filters, page, page_size: pageSize }).filter(([, value]) => value !== ''))
  const { data } = await apiClient.get<DailyInsightPage>(`${basePath}/daily`, { params })
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

export default { getConfig, updateConfig, getRuntime, analyzeNow, listSamples, listBatches, listAnalyzerAccounts, probe, listDaily, getOverview, getDaily, listRepresentativeItems }
