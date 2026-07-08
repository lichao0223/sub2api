package service

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type accountUsageCodexProbeRepo struct {
	stubOpenAIAccountRepo
	updateExtraCh chan map[string]any
	rateLimitCh   chan time.Time
}

func (r *accountUsageCodexProbeRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if r.updateExtraCh != nil {
		copied := make(map[string]any, len(updates))
		for k, v := range updates {
			copied[k] = v
		}
		r.updateExtraCh <- copied
	}
	return nil
}

func (r *accountUsageCodexProbeRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	if r.rateLimitCh != nil {
		r.rateLimitCh <- resetAt
	}
	return nil
}

func TestBuildGLMClaudeUsageResponse(t *testing.T) {
	t.Parallel()

	payload := &glmUsageResponse{}
	payload.Data.Limits = append(payload.Data.Limits,
		struct {
			Type          string  `json:"type"`
			Unit          int     `json:"unit"`
			Percentage    float64 `json:"percentage"`
			NextResetTime int64   `json:"nextResetTime"`
		}{Type: "TOKENS_LIMIT", Unit: 3, Percentage: 100, NextResetTime: 1783075612365},
		struct {
			Type          string  `json:"type"`
			Unit          int     `json:"unit"`
			Percentage    float64 `json:"percentage"`
			NextResetTime int64   `json:"nextResetTime"`
		}{Type: "TOKENS_LIMIT", Unit: 6, Percentage: 46, NextResetTime: 1783562399981},
	)

	resp := buildGLMClaudeUsageResponse(payload)
	if resp.FiveHour.Utilization != 100 {
		t.Fatalf("FiveHour.Utilization = %v, want 100", resp.FiveHour.Utilization)
	}
	if resp.SevenDay.Utilization != 46 {
		t.Fatalf("SevenDay.Utilization = %v, want 46", resp.SevenDay.Utilization)
	}
	if resp.FiveHour.ResetsAt == "" || resp.SevenDay.ResetsAt == "" {
		t.Fatalf("expected reset times, got five_hour=%q seven_day=%q", resp.FiveHour.ResetsAt, resp.SevenDay.ResetsAt)
	}
}

func TestBuildGLMCodexExtraUpdates(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	resp := &ClaudeUsageResponse{}
	resp.FiveHour.Utilization = 100
	resp.FiveHour.ResetsAt = "2026-07-06T15:00:00Z"
	resp.SevenDay.Utilization = 46
	resp.SevenDay.ResetsAt = "2026-07-13T10:00:00Z"

	updates := buildGLMCodexExtraUpdates(resp, now)
	if got := updates["codex_5h_used_percent"]; got != 100.0 {
		t.Fatalf("codex_5h_used_percent = %v, want 100", got)
	}
	if got := updates["codex_5h_window_minutes"]; got != 300 {
		t.Fatalf("codex_5h_window_minutes = %v, want 300", got)
	}
	if got := updates["codex_7d_used_percent"]; got != 46.0 {
		t.Fatalf("codex_7d_used_percent = %v, want 46", got)
	}
	if got := updates["codex_7d_window_minutes"]; got != 10080 {
		t.Fatalf("codex_7d_window_minutes = %v, want 10080", got)
	}
	if got := updates["codex_usage_updated_at"]; got != "2026-07-06T10:00:00Z" {
		t.Fatalf("codex_usage_updated_at = %v, want 2026-07-06T10:00:00Z", got)
	}
}

func TestGLMCodexQuotaRateLimitResetAt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	fiveHourReset := now.Add(30 * time.Minute).Format(time.RFC3339)
	sevenDayReset := now.Add(48 * time.Hour).Format(time.RFC3339)

	tests := []struct {
		name      string
		resp      *ClaudeUsageResponse
		wantNil   bool
		wantReset time.Time
	}{
		{
			name: "5h quota reached",
			resp: func() *ClaudeUsageResponse {
				resp := &ClaudeUsageResponse{}
				resp.FiveHour.Utilization = 100
				resp.FiveHour.ResetsAt = fiveHourReset
				resp.SevenDay.Utilization = 88
				resp.SevenDay.ResetsAt = sevenDayReset
				return resp
			}(),
			wantReset: now.Add(30 * time.Minute),
		},
		{
			name: "7d quota reached",
			resp: func() *ClaudeUsageResponse {
				resp := &ClaudeUsageResponse{}
				resp.FiveHour.Utilization = 0
				resp.FiveHour.ResetsAt = fiveHourReset
				resp.SevenDay.Utilization = 100
				resp.SevenDay.ResetsAt = sevenDayReset
				return resp
			}(),
			wantReset: now.Add(48 * time.Hour),
		},
		{
			name: "both quotas reached uses later reset",
			resp: func() *ClaudeUsageResponse {
				resp := &ClaudeUsageResponse{}
				resp.FiveHour.Utilization = 100
				resp.FiveHour.ResetsAt = fiveHourReset
				resp.SevenDay.Utilization = 100
				resp.SevenDay.ResetsAt = sevenDayReset
				return resp
			}(),
			wantReset: now.Add(48 * time.Hour),
		},
		{
			name: "below quota does not rate limit",
			resp: func() *ClaudeUsageResponse {
				resp := &ClaudeUsageResponse{}
				resp.FiveHour.Utilization = 99
				resp.FiveHour.ResetsAt = fiveHourReset
				resp.SevenDay.Utilization = 88
				resp.SevenDay.ResetsAt = sevenDayReset
				return resp
			}(),
			wantNil: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := glmCodexQuotaRateLimitResetAt(tt.resp, now)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("glmCodexQuotaRateLimitResetAt() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("glmCodexQuotaRateLimitResetAt() = nil, want reset time")
				return
			}
			if !got.Equal(tt.wantReset) {
				t.Fatalf("glmCodexQuotaRateLimitResetAt() = %s, want %s", got.Format(time.RFC3339), tt.wantReset.Format(time.RFC3339))
			}
		})
	}
}

func TestAccountUsageService_PersistGLMCodexSnapshotPromotesExhaustedQuotaToRateLimit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	resetAt := now.Add(30 * time.Minute)
	repo := &accountUsageCodexProbeRepo{
		updateExtraCh: make(chan map[string]any, 1),
		rateLimitCh:   make(chan time.Time, 1),
	}
	svc := &AccountUsageService{accountRepo: repo}
	account := &Account{
		ID:       701,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"model_provider": "glm"},
	}
	resp := &ClaudeUsageResponse{}
	resp.FiveHour.Utilization = 100
	resp.FiveHour.ResetsAt = resetAt.Format(time.RFC3339)
	resp.SevenDay.Utilization = 88
	resp.SevenDay.ResetsAt = now.Add(2 * time.Hour).Format(time.RFC3339)

	svc.persistGLMCodexSnapshot(context.Background(), account, resp, now)

	select {
	case updates := <-repo.updateExtraCh:
		if got := updates["codex_5h_used_percent"]; got != 100.0 {
			t.Fatalf("codex_5h_used_percent = %v, want 100", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waiting for GLM codex snapshot update timed out")
	}

	select {
	case got := <-repo.rateLimitCh:
		if !got.Equal(resetAt) {
			t.Fatalf("rate limit reset = %s, want %s", got.Format(time.RFC3339), resetAt.Format(time.RFC3339))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waiting for GLM rate limit update timed out")
	}
	if account.RateLimitResetAt == nil || !account.RateLimitResetAt.Equal(resetAt) {
		t.Fatalf("account.RateLimitResetAt = %v, want %s", account.RateLimitResetAt, resetAt.Format(time.RFC3339))
	}
}

func TestShouldAutoPauseGLMAPIKeyAtDefault100Percent(t *testing.T) {
	t.Parallel()

	account := &Account{
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"model_provider":          "glm",
			"codex_5h_used_percent":   100.0,
			"codex_5h_reset_at":       time.Now().Add(time.Hour).Format(time.RFC3339),
			"codex_usage_updated_at":  time.Now().Format(time.RFC3339),
			"auto_pause_7d_disabled":  true,
			"auto_pause_5h_threshold": nil,
		},
	}

	paused, decision := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
	if !paused {
		t.Fatal("expected GLM API key account to pause at 100% by default")
	}
	if decision.window != "5h" {
		t.Fatalf("decision.window = %q, want 5h", decision.window)
	}
}

func TestShouldRefreshOpenAICodexSnapshot(t *testing.T) {
	t.Parallel()

	rateLimitedUntil := time.Now().Add(5 * time.Minute)
	now := time.Now()
	usage := &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 0},
		SevenDay: &UsageProgress{Utilization: 0},
	}

	if !shouldRefreshOpenAICodexSnapshot(&Account{RateLimitResetAt: &rateLimitedUntil}, usage, now) {
		t.Fatal("expected rate-limited account to force codex snapshot refresh")
	}

	if shouldRefreshOpenAICodexSnapshot(&Account{}, usage, now) {
		t.Fatal("expected complete non-rate-limited usage to skip codex snapshot refresh")
	}

	if !shouldRefreshOpenAICodexSnapshot(&Account{}, &UsageInfo{FiveHour: nil, SevenDay: &UsageProgress{}}, now) {
		t.Fatal("expected missing 5h snapshot to require refresh")
	}

	staleAt := now.Add(-(openAIProbeCacheTTL + time.Minute)).Format(time.RFC3339)
	if !shouldRefreshOpenAICodexSnapshot(&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_enabled": true,
			"codex_usage_updated_at":                       staleAt,
		},
	}, usage, now) {
		t.Fatal("expected stale ws snapshot to trigger refresh")
	}
}

// TestShouldRefreshOpenAICodexSnapshot_SparkShadowIgnoresWSv2 外审第9轮 P1:spark 影子用量走
// QueryUsage(/wham/usage,与 WSv2 无关),staleness 不得被 WSv2 门控,否则首刷后窗口永久冻结。
func TestShouldRefreshOpenAICodexSnapshot_SparkShadowIgnoresWSv2(t *testing.T) {
	t.Parallel()

	now := time.Now()
	usage := &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 0},
		SevenDay: &UsageProgress{Utilization: 0},
	}
	staleAt := now.Add(-(openAIProbeCacheTTL + time.Minute)).Format(time.RFC3339)
	freshAt := now.Add(-time.Minute).Format(time.RFC3339)
	parentID := int64(7001)

	// 影子无 WSv2,但首刷后窗口已存在;过期 codex_usage_updated_at 必须触发再刷新。
	shadowStale := &Account{
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
		Extra:           map[string]any{"codex_usage_updated_at": staleAt},
	}
	if !shouldRefreshOpenAICodexSnapshot(shadowStale, usage, now) {
		t.Fatal("expected stale spark shadow (no WSv2) to trigger refresh")
	}

	// 影子时间戳仍新鲜→不刷(TTL 生效)。
	shadowFresh := &Account{
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
		Extra:           map[string]any{"codex_usage_updated_at": freshAt},
	}
	if shouldRefreshOpenAICodexSnapshot(shadowFresh, usage, now) {
		t.Fatal("expected fresh spark shadow to skip refresh (TTL not elapsed)")
	}

	// 反向对照:普通账号无 WSv2 + 过期时间戳→仍不刷(WSv2 门控普通账号的 probe 刷新)。
	normalNoWS := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"codex_usage_updated_at": staleAt},
	}
	if shouldRefreshOpenAICodexSnapshot(normalNoWS, usage, now) {
		t.Fatal("expected non-WSv2 normal account to skip codex probe refresh")
	}
}

func TestExtractOpenAICodexProbeUpdatesAccepts429WithCodexHeaders(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "604800")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "18000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	updates, err := extractOpenAICodexProbeUpdates(&http.Response{StatusCode: http.StatusTooManyRequests, Header: headers})
	if err != nil {
		t.Fatalf("extractOpenAICodexProbeUpdates() error = %v", err)
	}
	if len(updates) == 0 {
		t.Fatal("expected codex probe updates from 429 headers")
	}
	if got := updates["codex_5h_used_percent"]; got != 100.0 {
		t.Fatalf("codex_5h_used_percent = %v, want 100", got)
	}
	if got := updates["codex_7d_used_percent"]; got != 100.0 {
		t.Fatalf("codex_7d_used_percent = %v, want 100", got)
	}
}

func TestAccountUsageService_PersistOpenAICodexProbeSnapshotOnlyUpdatesExtra(t *testing.T) {
	t.Parallel()

	repo := &accountUsageCodexProbeRepo{
		updateExtraCh: make(chan map[string]any, 1),
		rateLimitCh:   make(chan time.Time, 1),
	}
	svc := &AccountUsageService{accountRepo: repo}
	svc.persistOpenAICodexProbeSnapshot(321, map[string]any{
		"codex_7d_used_percent": 100.0,
		"codex_7d_reset_at":     time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339),
	})

	select {
	case updates := <-repo.updateExtraCh:
		if got := updates["codex_7d_used_percent"]; got != 100.0 {
			t.Fatalf("codex_7d_used_percent = %v, want 100", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("等待 codex 探测快照写入 extra 超时")
	}

	select {
	case got := <-repo.rateLimitCh:
		t.Fatalf("不应将探测快照写入运行时限流状态: %v", got)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestAccountUsageService_GetOpenAIUsage_DoesNotPromoteCodexExtraToRateLimit(t *testing.T) {
	t.Parallel()

	resetAt := time.Now().Add(6 * 24 * time.Hour).UTC().Truncate(time.Second)
	repo := &accountUsageCodexProbeRepo{
		rateLimitCh: make(chan time.Time, 1),
	}
	svc := &AccountUsageService{accountRepo: repo}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_5h_used_percent": 1.0,
			"codex_5h_reset_at":     time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339),
			"codex_7d_used_percent": 100.0,
			"codex_7d_reset_at":     resetAt.Format(time.RFC3339),
		},
	}

	usage, err := svc.getOpenAIUsage(context.Background(), account, false)
	if err != nil {
		t.Fatalf("getOpenAIUsage() error = %v", err)
	}
	if usage.SevenDay == nil || usage.SevenDay.Utilization != 100.0 {
		t.Fatalf("预期 7 天用量仍然可见，实际为 %#v", usage.SevenDay)
	}
	if account.RateLimitResetAt != nil {
		t.Fatalf("不应让已耗尽的 codex extra 改写运行时限流状态: %v", account.RateLimitResetAt)
	}
	select {
	case got := <-repo.rateLimitCh:
		t.Fatalf("不应将已耗尽的 codex extra 持久化为运行时限流状态: %v", got)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestBuildCodexUsageProgressFromExtra_ZerosExpiredWindow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)

	t.Run("expired 5h window zeroes utilization", func(t *testing.T) {
		extra := map[string]any{
			"codex_5h_used_percent": 42.0,
			"codex_5h_reset_at":     "2026-03-16T10:00:00Z", // 2h ago
		}
		progress := buildCodexUsageProgressFromExtra(extra, "5h", now)
		if progress == nil {
			t.Fatal("expected non-nil progress")
			return
		}
		if progress.Utilization != 0 {
			t.Fatalf("expected Utilization=0 for expired window, got %v", progress.Utilization)
		}
		if progress.RemainingSeconds != 0 {
			t.Fatalf("expected RemainingSeconds=0, got %v", progress.RemainingSeconds)
		}
	})

	t.Run("active 5h window keeps utilization", func(t *testing.T) {
		resetAt := now.Add(2 * time.Hour).Format(time.RFC3339)
		extra := map[string]any{
			"codex_5h_used_percent": 42.0,
			"codex_5h_reset_at":     resetAt,
		}
		progress := buildCodexUsageProgressFromExtra(extra, "5h", now)
		if progress == nil {
			t.Fatal("expected non-nil progress")
			return
		}
		if progress.Utilization != 42.0 {
			t.Fatalf("expected Utilization=42, got %v", progress.Utilization)
		}
	})

	t.Run("expired 7d window zeroes utilization", func(t *testing.T) {
		extra := map[string]any{
			"codex_7d_used_percent": 88.0,
			"codex_7d_reset_at":     "2026-03-15T00:00:00Z", // yesterday
		}
		progress := buildCodexUsageProgressFromExtra(extra, "7d", now)
		if progress == nil {
			t.Fatal("expected non-nil progress")
			return
		}
		if progress.Utilization != 0 {
			t.Fatalf("expected Utilization=0 for expired 7d window, got %v", progress.Utilization)
		}
	})
}
