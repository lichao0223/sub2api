package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	httppool "github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

const (
	legacyGLMUsageURL        = "https://open.bigmodel.cn/api/monitor/usage/quota/limit"
	legacyDeepSeekBalanceURL = "https://api.deepseek.com/user/balance"
)

type AccountBalance struct {
	Currency     string `json:"currency"`
	TotalBalance string `json:"total_balance"`
}

func isLegacyCNUsageAccount(account *Account) bool {
	if account == nil || account.Type != AccountTypeAPIKey ||
		(account.Platform != PlatformAnthropic && account.Platform != PlatformOpenAI) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(account.GetExtraString("model_provider"))) {
	case "glm", "kimi", "deepseek":
		return true
	default:
		return false
	}
}

func (s *AccountUsageService) getLegacyCNUsage(ctx context.Context, account *Account) (*UsageInfo, error) {
	provider := strings.ToLower(strings.TrimSpace(account.GetExtraString("model_provider")))
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return nil, fmt.Errorf("%s API key is empty", provider)
	}

	switch provider {
	case "glm":
		body, err := legacyCNGet(ctx, account, legacyGLMUsageURL, apiKey, false)
		if err != nil {
			return nil, err
		}
		usage, err := parseLegacyGLMUsage(body)
		if err == nil {
			s.persistLegacyGLMQuota(ctx, account, usage)
		}
		return usage, err
	case "kimi":
		body, err := legacyCNGet(ctx, account, legacyKimiUsageURL(account.GetBaseURL()), apiKey, true)
		if err != nil {
			return nil, err
		}
		return parseLegacyKimiUsage(body)
	case "deepseek":
		body, err := legacyCNGet(ctx, account, legacyDeepSeekBalanceURL, apiKey, true)
		if err != nil {
			return nil, err
		}
		var payload struct {
			BalanceInfos []AccountBalance `json:"balance_infos"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("decode deepseek balance response: %w", err)
		}
		now := time.Now()
		return &UsageInfo{Source: "active", UpdatedAt: &now, Balances: payload.BalanceInfos}, nil
	default:
		return nil, fmt.Errorf("unsupported legacy CN provider: %s", provider)
	}
}

func legacyCNGet(ctx context.Context, account *Account, url, apiKey string, bearer bool) ([]byte, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build usage request: %w", err)
	}
	if bearer {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	} else {
		req.Header.Set("Authorization", apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept-Language", "en-US,en")
	}
	req.Header.Set("Accept", "application/json")
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	client, err := httppool.GetClient(httppool.Options{ProxyURL: proxyURL, Timeout: 15 * time.Second, ResponseHeaderTimeout: 10 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("build usage client: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("usage request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("usage API returned %d", resp.StatusCode)
	}
	return body, nil
}

func legacyKimiUsageURL(baseURL string) string {
	base := strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(baseURL), "/"), "/v1")
	return base + "/v1/usages"
}

func parseLegacyGLMUsage(body []byte) (*UsageInfo, error) {
	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Limits []struct {
				Type          string  `json:"type"`
				Unit          int     `json:"unit"`
				Percentage    float64 `json:"percentage"`
				NextResetTime int64   `json:"nextResetTime"`
			} `json:"limits"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode GLM usage response: %w", err)
	}
	if payload.Code != 0 && payload.Code != http.StatusOK {
		return nil, fmt.Errorf("GLM usage returned code %d: %s", payload.Code, payload.Msg)
	}
	now := time.Now()
	usage := &UsageInfo{Source: "active", UpdatedAt: &now}
	for _, limit := range payload.Data.Limits {
		if limit.Type != "TOKENS_LIMIT" {
			continue
		}
		progress := &UsageProgress{Utilization: limit.Percentage}
		if limit.NextResetTime > 0 {
			reset := time.UnixMilli(limit.NextResetTime)
			progress.ResetsAt = &reset
			progress.RemainingSeconds = maxInt(int(time.Until(reset).Seconds()), 0)
		}
		switch limit.Unit {
		case 3:
			usage.FiveHour = progress
		case 6:
			usage.SevenDay = progress
		}
	}
	return usage, nil
}

func (s *AccountUsageService) persistLegacyGLMQuota(ctx context.Context, account *Account, usage *UsageInfo) {
	if s == nil || s.accountRepo == nil || account == nil || usage == nil {
		return
	}
	now := time.Now()
	updates := map[string]any{"codex_usage_updated_at": now.UTC().Format(time.RFC3339)}
	for _, window := range []struct {
		name    string
		minutes int
		value   *UsageProgress
	}{
		{"5h", 5 * 60, usage.FiveHour},
		{"7d", 7 * 24 * 60, usage.SevenDay},
	} {
		if window.value == nil {
			continue
		}
		updates["codex_"+window.name+"_used_percent"] = window.value.Utilization
		updates["codex_"+window.name+"_window_minutes"] = window.minutes
		if window.value.ResetsAt != nil {
			updates["codex_"+window.name+"_reset_at"] = window.value.ResetsAt.UTC().Format(time.RFC3339)
		}
	}
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("legacy_glm_quota_snapshot_persist_failed", "account_id", account.ID, "error", err)
	}
	if resetAt := legacyGLMRateLimitReset(usage, now); resetAt != nil {
		if err := s.accountRepo.SetRateLimited(ctx, account.ID, *resetAt); err != nil {
			slog.Warn("legacy_glm_rate_limit_persist_failed", "account_id", account.ID, "error", err)
			return
		}
		account.RateLimitedAt = &now
		account.RateLimitResetAt = resetAt
	}
}

func legacyGLMRateLimitReset(usage *UsageInfo, now time.Time) *time.Time {
	var result *time.Time
	for _, progress := range []*UsageProgress{usage.FiveHour, usage.SevenDay} {
		if progress == nil || progress.Utilization < 100 || progress.ResetsAt == nil || !progress.ResetsAt.After(now) {
			continue
		}
		if result == nil || progress.ResetsAt.After(*result) {
			reset := *progress.ResetsAt
			result = &reset
		}
	}
	return result
}

func parseLegacyKimiUsage(body []byte) (*UsageInfo, error) {
	var payload struct {
		Usage  legacyKimiUsageDetail `json:"usage"`
		Limits []struct {
			Window struct {
				Duration int    `json:"duration"`
				TimeUnit string `json:"timeUnit"`
			} `json:"window"`
			Detail legacyKimiUsageDetail `json:"detail"`
		} `json:"limits"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode Kimi usage response: %w", err)
	}
	now := time.Now()
	usage := &UsageInfo{Source: "active", UpdatedAt: &now, SevenDay: legacyKimiProgress(payload.Usage, now)}
	for _, limit := range payload.Limits {
		if (limit.Window.Duration == 300 && strings.Contains(strings.ToUpper(limit.Window.TimeUnit), "MINUTE")) ||
			(limit.Window.Duration == 5 && strings.Contains(strings.ToUpper(limit.Window.TimeUnit), "HOUR")) {
			usage.FiveHour = legacyKimiProgress(limit.Detail, now)
			break
		}
	}
	return usage, nil
}

type legacyKimiUsageDetail struct {
	Limit     json.Number `json:"limit"`
	Remaining json.Number `json:"remaining"`
	ResetTime string      `json:"resetTime"`
}

func legacyKimiProgress(detail legacyKimiUsageDetail, now time.Time) *UsageProgress {
	limit, err := detail.Limit.Int64()
	if err != nil || limit <= 0 {
		return nil
	}
	remaining, err := detail.Remaining.Int64()
	if err != nil {
		return nil
	}
	used := limit - remaining
	if used < 0 {
		used = 0
	}
	progress := &UsageProgress{Utilization: math.Min(100, float64(used)/float64(limit)*100), UsedRequests: used, LimitRequests: limit}
	if reset, err := time.Parse(time.RFC3339, detail.ResetTime); err == nil {
		progress.ResetsAt = &reset
		progress.RemainingSeconds = maxInt(int(time.Until(reset).Seconds()), 0)
	}
	return progress
}
