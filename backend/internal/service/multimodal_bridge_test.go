package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIPrepareMultimodal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"vision-request"}},
		Body: io.NopCloser(bytes.NewBufferString(
			`{"id":"vision-response","choices":[{"message":{"content":"A settings page."}}],` +
				`"usage":{"prompt_tokens":10,"completion_tokens":3}}`,
		)),
	}}}
	openAISvc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	svc := &GatewayService{cfg: &config.Config{}}
	account := multimodalBridgeAccount(PlatformOpenAI)
	body := []byte(`{"model":"text-only","messages":[{"role":"user","content":[` +
		`{"type":"text","text":"What is this?"},` +
		`{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}]}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	prepared, usage, err := svc.PrepareMultimodal(context.Background(), c, openAISvc, account, body)
	require.NoError(t, err)
	require.Equal(t, "vision-model", gjson.GetBytes(upstream.requestBodies[0], "model").String())
	require.Contains(t, string(prepared), "Image 1 description: A settings page.")
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.InputTokens)
	require.Equal(t, 3, usage.OutputTokens)
}

func TestAnthropicPrepareMultimodal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body: io.NopCloser(bytes.NewBufferString(
			`{"id":"vision-response","content":[{"type":"text","text":"A diagram."}],` +
				`"usage":{"input_tokens":8,"output_tokens":2}}`,
		)),
	}}}
	svc := &GatewayService{
		httpUpstream:        upstream,
		cfg:                 &config.Config{},
		tlsFPProfileService: &TLSFingerprintProfileService{},
	}
	account := multimodalBridgeAccount(PlatformAnthropic)
	body := []byte(`{"model":"text-only","messages":[{"role":"user","content":[` +
		`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"YWJj"}}]}]}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	prepared, usage, err := svc.PrepareMultimodal(context.Background(), c, nil, account, body)
	require.NoError(t, err)
	require.Equal(t, "vision-model", gjson.GetBytes(upstream.requestBodies[0], "model").String())
	require.Contains(t, string(prepared), "Image 1 description: A diagram.")
	require.NotNil(t, usage)
	require.Equal(t, 8, usage.InputTokens)
	require.Equal(t, 2, usage.OutputTokens)
}

func multimodalBridgeAccount(platform string) *Account {
	return &Account{
		ID:          1,
		Name:        "bridge",
		Platform:    platform,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":                       "test-key",
			"base_url":                      "https://example.com",
			"model_mapping":                 map[string]any{"alias": "text-only"},
			multimodalDefaultModeKey:        "vision_to_text",
			multimodalDefaultVisionModelKey: "vision-model",
		},
	}
}
