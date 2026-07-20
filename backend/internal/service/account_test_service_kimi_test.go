//go:build unit

package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAccountTestServiceKimiUsesCodingAPIAndFingerprintHeaders(t *testing.T) {
	t.Setenv(kimi.EnvDeviceName, "test-host")
	t.Setenv(kimi.EnvDeviceModel, "TestOS 1.0 arm64")
	t.Setenv(kimi.EnvOSVersion, "test-kernel-1")
	gin.SetMode(gin.TestMode)

	account := &Account{
		ID: 81, Name: "kimi", Platform: PlatformKimi, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "kimi-access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			"device_id":    "device-123",
			"model_mapping": map[string]any{
				"kimi": "kimi-for-coding",
			},
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n" +
				"data: [DONE]\n\n",
		)),
	}}
	svc := &AccountTestService{
		accountRepo:       repo,
		kimiTokenProvider: NewKimiTokenProvider(repo, nil),
		httpUpstream:      upstream,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/81/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "kimi", "hi", AccountTestModeDefault)
	require.NoError(t, err)
	require.Equal(t, kimi.DefaultBaseURL+"/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer kimi-access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, kimi.UserAgent, upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, "device-123", upstream.lastReq.Header.Get("X-Msh-Device-Id"))
	require.Equal(t, "kimi-for-coding", gjson.GetBytes(upstream.lastBody, "model").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}
