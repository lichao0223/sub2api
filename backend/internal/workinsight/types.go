package workinsight

import (
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"
)

type redisPayload struct {
	Segments []string `json:"segments"`
}

func encodePayload(segments []string) (string, error) {
	raw, err := json.Marshal(redisPayload{Segments: segments})
	return string(raw), err
}

func decodePayload(raw string) []string {
	var payload redisPayload
	if json.Unmarshal([]byte(raw), &payload) == nil && len(payload.Segments) > 0 {
		return payload.Segments
	}
	if text := strings.TrimSpace(raw); text != "" {
		return []string{text}
	}
	return nil
}

const (
	SettingKey                 = "ai_work_insight_config"
	ConfigInvalidationChannel  = "sub2api:ai_work_insight:config:invalidate"
	PayloadKeyPrefix           = "sub2api:ai_work_insight:payload:"
	PayloadSlotsKey            = "sub2api:ai_work_insight:payload_slots"
	PayloadSizesKey            = "sub2api:ai_work_insight:payload_sizes"
	PayloadBytesKey            = "sub2api:ai_work_insight:payload_bytes"
	DefaultPayloadTTL          = 120 * time.Minute
	DefaultMaxJobAge           = 90 * time.Minute
	DefaultIngressLimit        = 10000
	ingressWorkerCount         = 4
	maxIngressQueueBytes       = 64 << 20
	maxPayloadBytes            = 64 << 10
	maxOutstandingPayloads     = 5000
	maxOutstandingPayloadBytes = 64 << 20
)

var TaskCategories = []string{
	"代码开发", "问题排查", "测试用例", "接口文档", "需求分析", "方案设计", "数据分析", "SQL/报表",
	"运维部署", "日志分析", "文档写作", "翻译润色", "会议纪要", "客服支持", "培训学习", "其他",
}

type Config struct {
	Enabled                bool      `json:"enabled"`
	ConfigVersion          int64     `json:"config_version"`
	SampleRate             int       `json:"sample_rate"`
	SessionIdleMinutes     int       `json:"session_idle_minutes"`
	UserDailyLimit         int       `json:"user_daily_limit"`
	GlobalDailyLimit       int       `json:"global_daily_limit"`
	Timezone               string    `json:"timezone"`
	ExcludedUserIDs        []int64   `json:"excluded_user_ids"`
	ExcludedUserEmails     []string  `json:"excluded_user_emails"`
	QueueCapacity          int       `json:"queue_capacity"`
	WorkerCount            int       `json:"worker_count"`
	AnalysisIdleMinutes    int       `json:"analysis_idle_minutes"`
	AnalysisMaxWaitMinutes int       `json:"analysis_max_wait_minutes"`
	AnalysisTriggerMode    string    `json:"analysis_trigger_mode"`
	FixedIntervalMinutes   int       `json:"analysis_fixed_interval_minutes"`
	FixedTimes             []string  `json:"analysis_fixed_times"`
	MaxSamplesPerBatch     int       `json:"max_samples_per_batch"`
	ContextWindowTokens    int       `json:"context_window_tokens"`
	MaxInputTokens         int       `json:"max_input_tokens"`
	ReservedOutputTokens   int       `json:"reserved_output_tokens"`
	AnalysisTimeoutSeconds int       `json:"analysis_timeout_seconds"`
	MaxJobAgeMinutes       int       `json:"max_job_age_minutes"`
	PayloadTTLMinutes      int       `json:"payload_ttl_minutes"`
	DailyFinalizeTime      string    `json:"daily_finalize_time"`
	StoreRedactedPreview   bool      `json:"store_redacted_preview"`
	SampleRetentionDays    int       `json:"sample_retention_days"`
	InsightRetentionDays   int       `json:"insight_retention_days"`
	CleanupEnabled         bool      `json:"cleanup_enabled"`
	CleanupTime            string    `json:"cleanup_time"`
	CleanupBatchSize       int       `json:"cleanup_batch_size"`
	AnalyzerSource         string    `json:"analyzer_source"`
	AnalyzerAccountID      int64     `json:"analyzer_account_id,omitempty"`
	AnalyzerBaseURL        string    `json:"analyzer_base_url,omitempty"`
	AnalyzerModel          string    `json:"analyzer_model"`
	AnalyzerToken          string    `json:"analyzer_token,omitempty"`
	AnalyzerTokenSet       bool      `json:"analyzer_token_set"`
	UpdatedAt              time.Time `json:"updated_at"`
	UpdatedBy              int64     `json:"updated_by"`
}

type storedConfig struct {
	Config
	AnalyzerTokenCiphertext string `json:"analyzer_token_ciphertext,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		ConfigVersion: 1, SampleRate: 2, SessionIdleMinutes: 5,
		UserDailyLimit: 5000, GlobalDailyLimit: 200000, Timezone: "Asia/Shanghai",
		QueueCapacity: 10000, WorkerCount: 4, AnalysisIdleMinutes: 15,
		AnalysisMaxWaitMinutes: 60, AnalysisTriggerMode: "hybrid", FixedIntervalMinutes: 30,
		MaxSamplesPerBatch: 50, ContextWindowTokens: 128000, MaxInputTokens: 64000,
		ReservedOutputTokens: 4000, AnalysisTimeoutSeconds: 60,
		MaxJobAgeMinutes: 90, PayloadTTLMinutes: 120,
		DailyFinalizeTime: "00:15", SampleRetentionDays: 30, InsightRetentionDays: 180,
		CleanupEnabled: true, CleanupTime: "03:30", CleanupBatchSize: 5000,
		AnalyzerSource: "account", AnalyzerModel: "",
	}
}

func (c *Config) normalize() {
	defaults := DefaultConfig()
	if c.AnalysisTriggerMode == "" {
		c.AnalysisTriggerMode = defaults.AnalysisTriggerMode
	}
	if c.FixedIntervalMinutes == 0 {
		c.FixedIntervalMinutes = defaults.FixedIntervalMinutes
	}
	if c.ContextWindowTokens == 0 {
		c.ContextWindowTokens = defaults.ContextWindowTokens
	}
	if c.ReservedOutputTokens == 0 {
		c.ReservedOutputTokens = defaults.ReservedOutputTokens
	}
	if c.AnalysisTimeoutSeconds == 0 {
		c.AnalysisTimeoutSeconds = defaults.AnalysisTimeoutSeconds
	}
	if c.DailyFinalizeTime == "" {
		c.DailyFinalizeTime = defaults.DailyFinalizeTime
	}
	if c.CleanupTime == "" {
		c.CleanupTime = defaults.CleanupTime
	}
	if c.CleanupBatchSize == 0 {
		c.CleanupBatchSize = defaults.CleanupBatchSize
	}
	c.Timezone = strings.TrimSpace(c.Timezone)
	c.AnalyzerSource = strings.ToLower(strings.TrimSpace(c.AnalyzerSource))
	c.AnalyzerBaseURL = strings.TrimRight(strings.TrimSpace(c.AnalyzerBaseURL), "/")
	c.AnalyzerModel = strings.TrimSpace(c.AnalyzerModel)
	c.AnalyzerToken = strings.TrimSpace(c.AnalyzerToken)
	c.AnalysisTriggerMode = strings.ToLower(strings.TrimSpace(c.AnalysisTriggerMode))
	c.DailyFinalizeTime = strings.TrimSpace(c.DailyFinalizeTime)
	c.CleanupTime = strings.TrimSpace(c.CleanupTime)
	for i := range c.FixedTimes {
		c.FixedTimes[i] = strings.TrimSpace(c.FixedTimes[i])
	}
	slices.Sort(c.FixedTimes)
	c.FixedTimes = slices.Compact(c.FixedTimes)
	slices.Sort(c.ExcludedUserIDs)
	c.ExcludedUserIDs = slices.Compact(c.ExcludedUserIDs)
	for i := range c.ExcludedUserEmails {
		c.ExcludedUserEmails[i] = strings.ToLower(strings.TrimSpace(c.ExcludedUserEmails[i]))
	}
	slices.Sort(c.ExcludedUserEmails)
	c.ExcludedUserEmails = slices.Compact(c.ExcludedUserEmails)
}

func (c Config) validate() error {
	if c.ConfigVersion < 1 || c.SampleRate < 0 || c.SampleRate > 100 || c.SessionIdleMinutes < 1 || c.SessionIdleMinutes > 120 {
		return errors.New("invalid sampling configuration")
	}
	if c.UserDailyLimit < 1 || c.UserDailyLimit > 100000 || c.GlobalDailyLimit < c.UserDailyLimit || c.GlobalDailyLimit > 500000 || c.QueueCapacity < 100 || c.QueueCapacity > DefaultIngressLimit {
		return errors.New("invalid sampling limits")
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return errors.New("invalid timezone")
	}
	if c.WorkerCount < 1 || c.WorkerCount > 32 || c.AnalysisIdleMinutes < 1 || c.AnalysisMaxWaitMinutes < c.AnalysisIdleMinutes {
		return errors.New("invalid worker configuration")
	}
	if !slices.Contains([]string{"hybrid", "fixed_interval", "fixed_time"}, c.AnalysisTriggerMode) || c.FixedIntervalMinutes < 1 {
		return errors.New("invalid analysis trigger")
	}
	if c.AnalysisTriggerMode == "fixed_time" && len(c.FixedTimes) == 0 {
		return errors.New("fixed analysis times are required")
	}
	for _, value := range append(slices.Clone(c.FixedTimes), c.DailyFinalizeTime, c.CleanupTime) {
		if _, err := time.Parse("15:04", value); err != nil {
			return errors.New("invalid scheduled time")
		}
	}
	safeInput := c.ContextWindowTokens - c.ReservedOutputTokens - 4000 - (c.ContextWindowTokens * 15 / 100)
	if c.MaxSamplesPerBatch < 1 || c.ContextWindowTokens < 8000 || c.MaxInputTokens < 1000 || c.MaxInputTokens > safeInput || c.ReservedOutputTokens < 1000 || c.AnalysisTimeoutSeconds < 1 || c.AnalysisTimeoutSeconds > 300 || c.MaxJobAgeMinutes < 1 || c.PayloadTTLMinutes < c.MaxJobAgeMinutes+30 || c.PayloadTTLMinutes > 120 {
		return errors.New("invalid analysis lifecycle")
	}
	if c.SampleRetentionDays < 1 || c.InsightRetentionDays < c.SampleRetentionDays || c.CleanupBatchSize < 100 || c.CleanupBatchSize > 5000 {
		return errors.New("invalid retention configuration")
	}
	if c.AnalyzerSource != "account" && c.AnalyzerSource != "custom" {
		return errors.New("invalid analyzer source")
	}
	if c.Enabled && strings.TrimSpace(c.AnalyzerModel) == "" {
		return errors.New("analyzer model is required")
	}
	if c.AnalyzerSource == "account" && c.Enabled && c.AnalyzerAccountID <= 0 {
		return errors.New("analyzer account is required")
	}
	if c.AnalyzerSource == "custom" {
		u, err := url.Parse(c.AnalyzerBaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("invalid analyzer base URL")
		}
		if c.Enabled && !c.AnalyzerTokenSet && c.AnalyzerToken == "" {
			return errors.New("analyzer token is required")
		}
	}
	for _, id := range c.ExcludedUserIDs {
		if id <= 0 {
			return errors.New("invalid excluded user ID")
		}
	}
	return nil
}

func (c Config) excludes(userID int64, email string) bool {
	return slices.Contains(c.ExcludedUserIDs, userID) || slices.Contains(c.ExcludedUserEmails, strings.ToLower(strings.TrimSpace(email)))
}

type Runtime struct {
	Enabled              bool  `json:"enabled"`
	QueueDepth           int   `json:"queue_depth"`
	QueueCapacity        int   `json:"queue_capacity"`
	QueueBytes           int64 `json:"queue_bytes"`
	QueueByteCapacity    int64 `json:"queue_byte_capacity"`
	Dropped              int64 `json:"dropped"`
	Processed            int64 `json:"processed"`
	Failed               int64 `json:"failed"`
	WaitingSamples       int64 `json:"waiting_samples"`
	QueuedBatches        int64 `json:"queued_batches"`
	ProcessingBatches    int64 `json:"processing_batches"`
	RetryBatches         int64 `json:"retry_batches"`
	DoneBatches          int64 `json:"done_batches"`
	FailedBatches        int64 `json:"failed_batches"`
	ActiveUsers          int64 `json:"active_users"`
	ActiveSessions       int64 `json:"active_sessions"`
	CoveredSessions      int64 `json:"covered_sessions"`
	AnalyzerInputTokens  int64 `json:"analyzer_input_tokens"`
	AnalyzerOutputTokens int64 `json:"analyzer_output_tokens"`
	AnalyzerCalls        int64 `json:"analyzer_calls"`
}

type ProbeResult struct {
	OK        bool      `json:"ok"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	LatencyMS int64     `json:"latency_ms"`
	CheckedAt time.Time `json:"checked_at"`
}

type AnalyzerAccount struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Platform string   `json:"platform"`
	Models   []string `json:"models"`
}

type RepresentativeItem struct {
	SourceSampleIDs  []int64  `json:"source_sample_ids"`
	Summary          string   `json:"summary"`
	TaskCategories   []string `json:"task_categories"`
	ExplicitProjects []string `json:"explicit_projects"`
	ExplicitModules  []string `json:"explicit_modules"`
}

type BatchResult struct {
	WorkSummary         string               `json:"work_summary"`
	TaskCategories      []string             `json:"task_categories"`
	ExplicitProjects    []string             `json:"explicit_projects"`
	ExplicitModules     []string             `json:"explicit_modules"`
	ChangeTypes         []string             `json:"change_types"`
	BusinessTopics      []string             `json:"business_topics"`
	RepresentativeItems []RepresentativeItem `json:"representative_items"`
	EvidenceLevel       string               `json:"evidence_level"`
}

type Sample struct {
	ID              int64
	UserID          int64
	Username        string
	LocalDate       time.Time
	SessionID       string
	EstimatedTokens int
	CreatedAt       time.Time
}

type Batch struct {
	ID                   int64
	UserID               int64
	Username             string
	LocalDate            time.Time
	SessionID            string
	FirstSampleID        int64
	LastSampleID         int64
	SampleCount          int
	EstimatedInputTokens int
	TriggerReason        string
	Attempts             int
	ClaimVersion         int64
	BaseSummaryVersion   int64
	CreatedAt            time.Time
}

type BatchSample struct {
	ID              int64
	EstimatedTokens int
	CreatedAt       time.Time
}
