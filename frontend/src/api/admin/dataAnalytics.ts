import { apiClient } from '../client'

export interface AnalyticsTaskControl {
  enabled: boolean
  version: number
  updated_by: number
  updated_at: string | null
}

export interface AnalyticsTaskStatus {
  control: AnalyticsTaskControl
  state: 'idle' | 'running' | 'pausing' | 'paused' | 'unknown'
  running: number
  last_success_at: string | null
  last_seen_at?: string | null
  analytics_day?: string
  analytics_stage?: string
  analytics_started_at?: string | null
  progress?: {
    directory_ready: boolean; seeded: boolean; total_days: number; completed_days: number
    backlog_days: number; failed_days: number; pending_requests: number; retry_at: string | null
    last_success_at: string | null; last_error: string
  }
}

export const analyticsTasksAPI = {
  async status(signal?: AbortSignal): Promise<AnalyticsTaskStatus> {
    return (await apiClient.get<AnalyticsTaskStatus>('/admin/settings/data-analytics', { signal, timeout: 10000 })).data
  },
  async setEnabled(enabled: boolean, version: number): Promise<AnalyticsTaskControl> {
    return (await apiClient.put<AnalyticsTaskControl>('/admin/settings/data-analytics', { enabled, version })).data
  },
}

export interface AnalyticsOrg { id: string; parent: string; name: string; path: string[]; names: string[]; level: number }
export interface AnalyticsPerson { id: string; name: string; org: string; active: boolean }
export interface AnalyticsDirectory { orgs: AnalyticsOrg[]; people: AnalyticsPerson[] }
export interface AnalyticsIdentity { user_id: number; username: string; person_id: string; basis: string }
export interface AnalyticsDirectoryState {
  version: number; effective_at: string | null; directory: AnalyticsDirectory; identities: AnalyticsIdentity[]
  imports: { id: string; created_at: string; applied_at: string | null }[]
}
export interface AnalyticsPreview {
  id: string; digest: string; directory: AnalyticsDirectory; duplicate_names: number; missing_orgs: number
  matches: number; unmatched: number; expires_at: string; applied: boolean
}
export interface AnalyticsValues {
  input: string; output: string; cache_write: string; cache_read: string; tokens: string; requests: string
  metered: string; fixed: string; cost: string; pending: string; excluded_tokens: string
  input_cost: string; output_cost: string; write_cost: string; read_cost: string; image_cost: string; request_cost: string
}
export interface AnalyticsRow extends AnalyticsValues {
  user_id: number; name: string; person_id: string; org_ids: string[]; org_names: string[]
  model: string; mode: string; source_id: number; source: string; issue: string; attribution: number
}
export interface AnalyticsDepartment extends AnalyticsValues { id: string; name: string; parent: string; level: number; users: number }
export interface AnalyticsFixedShare {
  month: string; unit_id: number; name: string; user_id: number; total: string; participants: number; amount: string; public: string
}
export interface AnalyticsPersonMonth extends AnalyticsValues { month: string; generation: number }
export interface AnalyticsReport {
  scope_org_id: string
  period: { year: number; mode: string; months: number[]; keys: string[]; grain: string }
  summary: AnalyticsValues; users: number; people: AnalyticsRow[]; people_total: number
  departments: AnalyticsDepartment[]; trend: { bucket: string; tokens: string }[]; models: AnalyticsRow[]
  fixed_shares: AnalyticsFixedShare[]; public_cost: string; versions: Record<string, number>; task_status: AnalyticsTaskStatus
  person_months?: AnalyticsPersonMonth[]
  coverage: { backlog_days: number; failed_days: number; earliest_pending: string | null; last_updated: string | null
    directory_ready: boolean; issues: { day: string; message: string }[]; raw_history_start: string | null; history_gap: boolean; last_error: string; pending_requests: number; completed_days: number; expected_days: number; complete: boolean
    history_basis: string; provisional: boolean }
}
export interface AnalyticsQuery {
  year: number; mode: string; months: string; org_id?: string; search?: string; page?: number; page_size?: number
  sort?: string; user_id?: number; versions?: string; export?: boolean
}
export const dataAnalyticsAPI = {
  async directory(): Promise<AnalyticsDirectoryState> { return (await apiClient.get('/admin/data-analytics/directory')).data },
  async preview(organizations: File, people: File): Promise<AnalyticsPreview> {
    const body = new FormData(); body.append('organizations', organizations); body.append('people', people)
    return (await apiClient.post('/admin/data-analytics/directory/preview', body, { headers: { 'Content-Type': 'multipart/form-data' } })).data
  },
  async apply(id: string, effective_at: string, version: number) {
    await apiClient.post('/admin/data-analytics/directory/apply', { id, effective_at, version })
  },
  async map(user_id: number, person_id: string, version: number) {
    await apiClient.put('/admin/data-analytics/identities', { user_id, person_id, version })
  },
  async report(params: AnalyticsQuery, signal?: AbortSignal): Promise<AnalyticsReport> {
    return (await apiClient.get('/admin/data-analytics/report', { params, signal, timeout: 25000 })).data
  },
  async rebuild(start: string, end: string) { await apiClient.post('/admin/data-analytics/rebuild', { start, end }) },
}
