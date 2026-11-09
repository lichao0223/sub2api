package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type glmQuotaRecoveryRepo struct {
	accountUsageCodexProbeRepo
	clearedAt time.Time
}

func (r *glmQuotaRecoveryRepo) ClearRateLimitIfObserved(_ context.Context, _ int64, _ time.Time, resetAt time.Time) (bool, error) {
	r.clearedAt = resetAt
	return true, nil
}

func TestLegacyCNUsageParsersRestoreWindowsAndBalance(t *testing.T) {
	glm, err := parseLegacyGLMUsage([]byte(`{"code":0,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"percentage":25},{"type":"TOKENS_LIMIT","unit":6,"percentage":75}]}}`))
	require.NoError(t, err)
	require.Equal(t, 25.0, glm.FiveHour.Utilization)
	require.Equal(t, 75.0, glm.SevenDay.Utilization)

	kimi, err := parseLegacyKimiUsage([]byte(`{"usage":{"limit":100,"remaining":20},"limits":[{"window":{"duration":5,"timeUnit":"HOUR"},"detail":{"limit":50,"remaining":40}}]}`))
	require.NoError(t, err)
	require.Equal(t, 20.0, kimi.FiveHour.Utilization)
	require.Equal(t, 80.0, kimi.SevenDay.Utilization)

	require.True(t, isLegacyCNUsageAccount(&Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: map[string]any{"model_provider": "deepseek"}}))
}

func TestLegacyGLMQuotaPersistsRateLimitAtExhaustion(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	fiveHourReset := now.Add(time.Hour)
	sevenDayReset := now.Add(7 * 24 * time.Hour)
	repo := &accountUsageCodexProbeRepo{rateLimitCh: make(chan time.Time, 1)}
	service := &AccountUsageService{accountRepo: repo}
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"model_provider": "glm"}}

	service.persistLegacyGLMQuota(t.Context(), account, &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 100, ResetsAt: &fiveHourReset},
		SevenDay: &UsageProgress{Utilization: 100, ResetsAt: &sevenDayReset},
	})

	select {
	case got := <-repo.rateLimitCh:
		require.Equal(t, sevenDayReset, got)
	case <-time.After(time.Second):
		t.Fatal("expected exhausted GLM quota to rate limit account")
	}
}

func TestLegacyGLMQuotaRecoversOnlyObservedExhaustion(t *testing.T) {
	now := time.Now().UTC()
	resetAt := now.Add(time.Hour)
	for _, matched := range []bool{true, false} {
		repo := &glmQuotaRecoveryRepo{}
		svc := &AccountUsageService{accountRepo: repo}
		observed := resetAt
		if !matched {
			observed = observed.Add(time.Minute)
		}
		account := &Account{ID: 7, RateLimitedAt: &now, RateLimitResetAt: &observed, Extra: map[string]any{
			"codex_5h_used_percent": 100, "codex_5h_reset_at": resetAt.Format(time.RFC3339),
		}}
		svc.persistLegacyGLMQuota(t.Context(), account, &UsageInfo{FiveHour: &UsageProgress{}, SevenDay: &UsageProgress{}})
		if matched {
			require.Equal(t, resetAt, repo.clearedAt)
			require.Nil(t, account.RateLimitResetAt)
		} else {
			require.True(t, repo.clearedAt.IsZero())
			require.NotNil(t, account.RateLimitResetAt)
		}
	}
}
