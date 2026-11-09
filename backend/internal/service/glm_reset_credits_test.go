//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type glmResetCreditsRepo struct {
	AccountRepository
	account *Account
	updates map[string]any
	err     error
}

func (r *glmResetCreditsRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func (r *glmResetCreditsRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = updates
	return r.err
}

func TestRefreshGLMResetCredits(t *testing.T) {
	const cards = `{"code":200,"success":true,"data":{"customerId":123,"fiveHourResets":[{"recordId":1,"available":true,"expireTime":"2099-12-22 23:59:59"},{"available":false,"expireTime":"2099-12-22 23:59:59"}],"weekResets":[{"recordId":2,"available":true,"expireTime":"2099-12-23 23:59:59"},{"available":true,"expireTime":"2020-01-01 00:00:00"}]}}`
	for _, platform := range []string{PlatformOpenAI, PlatformAnthropic} {
		t.Run(platform, func(t *testing.T) {
			repo := &glmResetCreditsRepo{account: &Account{
				ID: 9, Platform: platform, Type: AccountTypeAPIKey,
				Extra:       map[string]any{"model_provider": "glm"},
				Credentials: map[string]any{"api_key": "test-only-key", "base_url": "https://untrusted.example"},
			}}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(cards))}}
			svc := NewCNProviderQuotaService(repo, nil, upstream, nil)
			result, err := svc.RefreshGLMResetCredits(t.Context(), 9)
			require.NoError(t, err)
			require.Equal(t, 2, result.ResetCredits.AvailableCount)
			require.Equal(t, "2099-12-22T23:59:59+08:00", result.ResetCredits.Credits[0].ExpiresAt.Format("2006-01-02T15:04:05Z07:00"))
			require.True(t, result.CachePersisted)
			require.Equal(t, result.ResetCredits, repo.updates["glm_reset_credit_snapshot"])
			require.Equal(t, glmResetCreditsURL, upstream.lastReq.URL.String())
			require.Equal(t, http.MethodGet, upstream.lastReq.Method)
			require.Equal(t, "test-only-key", upstream.lastReq.Header.Get("Authorization"))
			require.Len(t, upstream.requests, 1)
		})
	}
	for _, tc := range []struct {
		name    string
		body    string
		status  int
		persist error
		valid   bool
	}{
		{"no cards", `{"code":200,"success":true,"data":{"fiveHourResets":[],"weekResets":[]}}`, 200, nil, true},
		{"save failed", cards, 200, errors.New("database unavailable"), true},
		{"unauthorized", cards, 401, nil, false},
		{"business failure", `{"code":401,"success":false}`, 200, nil, false},
		{"missing card lists", `{"code":200,"success":true,"data":{}}`, 200, nil, false},
		{"bad JSON", `{`, 200, nil, false},
		{"bad expiration", `{"code":200,"success":true,"data":{"fiveHourResets":[],"weekResets":[{"available":true,"expireTime":"invalid"}]}}`, 200, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &glmResetCreditsRepo{account: &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Extra: map[string]any{"model_provider": "glm"}, Credentials: map[string]any{"api_key": "test-only-key"}}, err: tc.persist}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}}
			result, err := NewCNProviderQuotaService(repo, nil, upstream, nil).RefreshGLMResetCredits(t.Context(), 1)
			if !tc.valid {
				require.Error(t, err)
				require.Nil(t, repo.updates)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.persist == nil, result.CachePersisted)
		})
	}
}

func TestRefreshGLMResetCreditsRejectsUnsupportedAccountsAndBlockedHost(t *testing.T) {
	for _, account := range []*Account{
		{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"model_provider": "glm"}},
		{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"model_provider": "deepseek"}},
		{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: map[string]any{"model_provider": "glm"}},
	} {
		upstream := &recordingHTTPUpstream{}
		_, err := NewCNProviderQuotaService(&glmResetCreditsRepo{account: account}, nil, upstream, nil).RefreshGLMResetCredits(t.Context(), 1)
		require.Error(t, err)
		require.Zero(t, upstream.calls)
	}
	upstream := &recordingHTTPUpstream{}
	repo := &glmResetCreditsRepo{account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Extra: map[string]any{"model_provider": "glm"}, Credentials: map[string]any{"api_key": "test-only-key"}}}
	_, err := NewCNProviderQuotaService(repo, nil, upstream, cnProbeAllowlistConfig("api.example.com")).RefreshGLMResetCredits(t.Context(), 1)
	require.ErrorContains(t, err, "GLM_RESET_CREDITS_URL_REJECTED")
	require.Zero(t, upstream.calls)
}

func TestUseGLMResetCredit(t *testing.T) {
	const cards = `{"code":200,"success":true,"data":{"fiveHourResets":[],"weekResets":[{"recordId":9,"grantType":"GLOBAL","available":true,"expireTime":"2099-12-22 23:59:59"},{"recordId":10,"grantType":"GLOBAL","available":true,"expireTime":"2099-12-23 23:59:59"}]}}`
	for _, tc := range []struct {
		name, grant, useResponse, readBack string
		wantRequests                       int
		wantError                          string
	}{
		{"spent card removed even if list lags", "GLOBAL", `{"code":200,"success":true}`, cards, 3, ""},
		{"refresh failure after success", "GLOBAL", `{"code":200,"success":true}`, `{`, 3, ""},
		{"upstream rejected the reset", "GLOBAL", `{"code":400,"msg":"Credit cannot be used","success":false}`, "", 2, "Credit cannot be used"},
		{"stale grant is never sent", "OTHER", "", "", 1, "GLM_RESET_CREDIT_UNAVAILABLE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &glmResetCreditsRepo{account: &Account{
				ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
				Extra: map[string]any{"model_provider": "glm"}, Credentials: map[string]any{"api_key": "fake-glm-key"},
			}}
			upstream := &httpUpstreamRecorder{}
			for _, body := range []string{cards, tc.useResponse, tc.readBack} {
				upstream.responses = append(upstream.responses, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))})
			}
			result, err := NewCNProviderQuotaService(repo, nil, upstream, nil).UseGLMResetCredit(t.Context(), 1, 9, "WEEK", tc.grant, "operation-1")
			require.Len(t, upstream.requests, tc.wantRequests)
			if tc.wantRequests >= 2 {
				require.Equal(t, http.MethodPost, upstream.requests[1].Method)
				require.Equal(t, glmUseResetCreditURL, upstream.requests[1].URL.String())
				require.Equal(t, "fake-glm-key", upstream.requests[1].Header.Get("Authorization"))
				body, err := io.ReadAll(upstream.requests[1].Body)
				require.NoError(t, err)
				require.JSONEq(t, `{"targetType":"PERSONAL","recordId":9,"grantType":"GLOBAL","resetType":"WEEK","requestId":"operation-1"}`, string(body))
			}
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, repo.updates)
				return
			}
			require.NoError(t, err)
			require.True(t, result.CachePersisted)
			require.Equal(t, result.ResetCredits, repo.updates["glm_reset_credit_snapshot"])
			if tc.readBack == cards {
				require.Equal(t, 1, result.ResetCredits.AvailableCount)
				require.Equal(t, int64(10), result.ResetCredits.Credits[0].RecordID)
			} else {
				require.Nil(t, result.ResetCredits)
			}
		})
	}
}
