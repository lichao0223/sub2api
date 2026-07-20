//go:build unit

package service

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
	"github.com/stretchr/testify/require"
)

const kimiUsageTestPayload = `{
  "usage":{"limit":"100","remaining":"75","resetTime":"2026-07-27T04:06:11Z"},
  "limits":[{"window":{"duration":300,"timeUnit":"TIME_UNIT_MINUTE"},"detail":{"limit":"100","remaining":"60","resetTime":"2026-07-20T09:06:11Z"}}]
}`

func TestAccountUsageServiceKimiOAuthUsesOfficialUsageEndpoint(t *testing.T) {
	account := &Account{
		ID: 91, Platform: PlatformKimi, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "oauth-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(kimiUsageTestPayload)),
	}}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, nil, nil, nil, NewKimiTokenProvider(repo, nil), upstream, NewUsageCache(), nil, nil)

	usage, err := svc.GetUsage(t.Context(), account.ID, true)
	require.NoError(t, err)
	require.Equal(t, kimi.DefaultBaseURL+"/usages", upstream.lastReq.URL.String())
	require.Equal(t, http.MethodGet, upstream.lastReq.Method)
	require.Equal(t, "Bearer oauth-token", upstream.lastReq.Header.Get("Authorization"))
	require.InDelta(t, 40, usage.FiveHour.Utilization, 0.001)
	require.Equal(t, int64(40), usage.FiveHour.UsedRequests)
	require.InDelta(t, 25, usage.SevenDay.Utilization, 0.001)
}

func TestAccountUsageServiceKimiAPIKeyProviderUsesAccountKey(t *testing.T) {
	for index, platform := range []string{PlatformOpenAI, PlatformAnthropic} {
		t.Run(platform, func(t *testing.T) {
			account := &Account{
				ID: int64(92 + index), Platform: platform, Type: AccountTypeAPIKey, Concurrency: 1,
				Credentials: map[string]any{"api_key": "api-key-token", "base_url": kimi.DefaultBaseURL},
				Extra:       map[string]any{"model_provider": "kimi"},
			}
			repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(kimiUsageTestPayload)),
			}}
			svc := NewAccountUsageService(repo, nil, nil, nil, nil, nil, nil, nil, nil, upstream, NewUsageCache(), nil, nil)

			usage, err := svc.GetUsage(t.Context(), account.ID, true)
			require.NoError(t, err)
			require.Equal(t, "Bearer api-key-token", upstream.lastReq.Header.Get("Authorization"))
			require.NotNil(t, usage.FiveHour)
			require.NotNil(t, usage.SevenDay)
		})
	}
}
