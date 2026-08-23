export const TASK_CATEGORIES = [
  '代码开发', '问题排查', '测试用例', '接口文档', '需求分析', '方案设计', '数据分析', 'SQL/报表',
  '运维部署', '日志分析', '文档写作', '翻译润色', '会议纪要', '客服支持', '培训学习', '其他',
] as const

export interface WorkInsightConfig {
  enabled: boolean
  usage_alert_enabled: boolean
  usage_alert_input_tokens: number
  config_version: number
  sample_rate: number
  session_idle_minutes: number
  user_daily_limit: number
  global_daily_limit: number
  timezone: string
  excluded_user_ids: number[]
  excluded_user_emails: string[]
  queue_capacity: number
  worker_count: number
  analysis_idle_minutes: number
  analysis_max_wait_minutes: number
  analysis_trigger_mode: 'hybrid' | 'fixed_interval' | 'fixed_time'
  analysis_fixed_interval_minutes: number
  analysis_fixed_times: string[]
  max_samples_per_batch: number
  context_window_tokens: number
  max_input_tokens: number
  reserved_output_tokens: number
  analysis_timeout_seconds: number
  max_job_age_minutes: number
  payload_ttl_minutes: number
  daily_finalize_time: string
  store_redacted_preview: boolean
  sample_retention_days: number
  insight_retention_days: number
  cleanup_enabled: boolean
  cleanup_time: string
  cleanup_batch_size: number
  analyzer_source: 'account' | 'custom'
  analyzer_account_id?: number
  analyzer_base_url?: string
  analyzer_model: string
  analyzer_token?: string
  analyzer_token_set: boolean
  updated_at: string
  updated_by: number
}

export interface UsageAlert {
  user_id: number
  username: string
  email: string
  count: number
  first_at: string
  latest_at: string
  max_input_tokens: number
}

export interface WorkInsightRuntime {
  enabled: boolean
  queue_depth: number
  queue_capacity: number
  queue_bytes: number
  queue_byte_capacity: number
  dropped: number
  processed: number
  failed: number
  waiting_samples: number
  queued_batches: number
  processing_batches: number
  retry_batches: number
  done_batches: number
  failed_batches: number
  active_users: number
  active_sessions: number
  covered_sessions: number
  analyzer_input_tokens: number
  analyzer_output_tokens: number
  analyzer_calls: number
}

export interface SampleSummary {
  id: number
  user_id?: number
  username: string
  provider: string
  requested_model: string
  sample_reason: string
  estimated_tokens: number
  prompt_chars: number
  analyzed_chars: number
  truncated: boolean
  status: string
  error_code: string
  created_at: string
}

export interface BatchSummary {
  id: number
  user_id?: number
  username: string
  sample_count: number
  trigger_reason: string
  status: string
  attempts: number
  error_code: string
  error_detail: string
  analyzer_model: string
  analyzer_input_tokens: number
  analyzer_output_tokens: number
  created_at: string
}

export interface LogPage<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

export type UsageAlertPage = LogPage<UsageAlert>

export interface ProbeResult {
  ok: boolean
  status: string
  message: string
  latency_ms: number
  checked_at: string
}

export interface AnalyzerAccount {
  id: number
  name: string
  platform: string
  models: string[]
}

export interface DailyInsight {
  id: number
  user_id?: number
  username: string
  insight_date: string
  business_request_count: number
  business_total_tokens: number
  business_input_tokens: number
  business_output_tokens: number
  business_cache_creation_tokens: number
  business_cache_read_tokens: number
  business_error_count: number
  average_duration_ms: number
  p95_duration_ms: number
  model_usage: Record<string, number>
  sample_count: number
  failed_sample_count: number
  eligible_active_session_count: number
  covered_active_session_count: number
  task_category_stats: Record<string, number>
  explicit_projects: string[]
  explicit_modules: string[]
  change_types: string[]
  business_topics: string[]
  daily_summary: string
  last_analyzed_at?: string
  finalized_at?: string
}

export interface RepresentativeItem {
  source_sample_ids: number[]
  summary: string
  task_categories: string[]
  explicit_projects: string[]
  explicit_modules: string[]
}

export interface DailyInsightDetail {
  insight: DailyInsight
  representative_items: RepresentativeItem[]
  representative_item_count: number
  representative_items_expired: boolean
  developer_tools: DeveloperTool[]
}

export interface DeveloperTool {
  name: string
  requests: number
  last_seen: string
}

export interface UserInsightRanking {
  latest_insight_id: number
  user_id?: number
  username: string
  start_date: string
  end_date: string
  insight_days: number
  business_request_count: number
  business_total_tokens: number
  sample_count: number
  failed_sample_count: number
  eligible_active_session_count: number
  covered_active_session_count: number
  latest_summary: string
  analyzed: boolean
}

export interface UserInsightRankingPage {
  items: UserInsightRanking[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface DailyInsightFilters {
  start_date: string
  end_date: string
  user_name: string
  task_category: string
  project_name: string
}

export interface WorkInsightOverview {
  active_users: number
  insight_users: number
  active_sessions: number
  covered_sessions: number
  sample_requests: number
  business_tokens: number
  failed_samples: number
  analyzer_input_tokens: number
  analyzer_output_tokens: number
}
