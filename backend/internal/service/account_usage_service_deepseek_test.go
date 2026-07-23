//go:build unit

package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountUsageServiceDeepSeekBalance(t *testing.T) {
	account := &Account{
		ID: 95, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "deepseek-key"},
		Extra:       map[string]any{"model_provider": "deepseek"},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"3346.66","granted_balance":"0.00","topped_up_balance":"3346.66"}]}`,
		)),
	}}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, nil, nil, nil, nil, upstream, NewUsageCache(), nil, nil)

	usage, err := svc.GetUsage(t.Context(), account.ID, true)
	require.NoError(t, err)
	require.Equal(t, deepSeekBalanceURL, upstream.lastReq.URL.String())
	require.Equal(t, "Bearer deepseek-key", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, []AccountBalance{{Currency: "CNY", TotalBalance: "3346.66"}}, usage.Balances)
}
