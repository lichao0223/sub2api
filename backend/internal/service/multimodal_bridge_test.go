package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIPrepareMultimodal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"vision-request"}},
		Body: io.NopCloser(bytes.NewBufferString(
			"data: {\"id\":\"vision-response\",\"choices\":[{\"delta\":{\"content\":\"A settings \"}}]}\n\n" +
				"data: {\"id\":\"vision-response\",\"choices\":[{\"delta\":{\"content\":\"page.\"}}]}\n\n" +
				"data: {\"id\":\"vision-response\",\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":3}}\n\n" +
				"data: [DONE]\n\n",
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
	require.True(t, gjson.GetBytes(upstream.requestBodies[0], "stream").Bool())
	require.True(t, gjson.GetBytes(upstream.requestBodies[0], "stream_options.include_usage").Bool())
	require.Contains(t, string(prepared), "Image 1 description: A settings page.")
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.InputTokens)
	require.Equal(t, 3, usage.OutputTokens)
}

func TestAnthropicPrepareMultimodal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(bytes.NewBufferString(
			"event: message_start\n" +
				"data: {\"type\":\"message_start\",\"message\":{\"id\":\"vision-response\",\"usage\":{\"input_tokens\":8}}}\n\n" +
				"event: content_block_delta\n" +
				"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"ignore\"}}\n\n" +
				"event: content_block_delta\n" +
				"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"A diagram.\"}}\n\n" +
				"event: message_delta\n" +
				"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":2}}\n\n",
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
	require.True(t, gjson.GetBytes(upstream.requestBodies[0], "stream").Bool())
	require.Contains(t, string(prepared), "Image 1 description: A diagram.")
	require.NotNil(t, usage)
	require.Equal(t, 8, usage.InputTokens)
	require.Equal(t, 2, usage.OutputTokens)
}

func TestKimiAPIKeyPrepareMultimodalUsesOpenAICompatibleBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(bytes.NewBufferString(
			"data: {\"id\":\"vision-response\",\"choices\":[{\"delta\":{\"content\":\"A diagram.\"}}]}\n\n" +
				"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":2}}\n\n",
		)),
	}}}
	openAISvc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	svc := &GatewayService{cfg: &config.Config{}}
	account := multimodalBridgeAccount(PlatformKimi)
	account.Credentials["base_url"] = "https://example.com"
	account.Credentials["api_key"] = "kimi-key"
	body := []byte(`{"model":"text-only","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}]}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	prepared, _, err := svc.PrepareMultimodal(context.Background(), c, openAISvc, account, body)

	require.NoError(t, err)
	require.Contains(t, string(prepared), "Image 1 description: A diagram.")
	require.Equal(t, "vision-model", gjson.GetBytes(upstream.requestBodies[0], "model").String())
}

func TestOpenAIOAuthDescribeImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"vision-request"}},
		Body: io.NopCloser(bytes.NewBufferString(
			"event: response.created\n" +
				"data: {\"type\":\"response.created\",\"response\":{\"id\":\"vision-response\",\"status\":\"in_progress\"}}\n\n" +
				"event: response.output_text.delta\n" +
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"A settings \"}\n\n" +
				"event: response.output_text.delta\n" +
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"page.\"}\n\n" +
				"event: response.completed\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"vision-response\",\"status\":\"completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":3,\"total_tokens\":13}}}\n\n",
		)),
	}}}
	openAISvc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := &Account{
		ID:          2,
		Name:        "chatgpt-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token"},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	description, requestID, usage, err := openAISvc.describeOpenAIImage(
		context.Background(), c, account, "gpt-5.5", "https://example.com/a.png", 1,
	)
	require.NoError(t, err)
	require.Equal(t, "A settings page.", description)
	require.Equal(t, "vision-response", requestID)
	require.Equal(t, 10, usage.InputTokens)
	require.Equal(t, 3, usage.OutputTokens)
	require.True(t, gjson.GetBytes(upstream.requestBodies[0], "stream").Bool())
	require.Equal(t, "input_image", gjson.GetBytes(upstream.requestBodies[0], "input.0.content.1.type").String())
	require.Equal(t, "https://example.com/a.png", gjson.GetBytes(upstream.requestBodies[0], "input.0.content.1.image_url").String())
}

func TestMultimodalPricingUsesVisionGroupWithoutChangingSourceGroup(t *testing.T) {
	const sourceGroupID int64 = 12
	const visionGroupID int64 = 34
	inputPrice := 2.0
	outputPrice := 4.0
	cache := newEmptyChannelCache()
	cache.pricingByGroupModel[channelModelKey{groupID: visionGroupID, model: "gpt-vision"}] = &ChannelModelPricing{
		BillingMode: BillingModeToken,
		InputPrice:  &inputPrice,
		OutputPrice: &outputPrice,
	}
	cache.channelByGroupID[visionGroupID] = &Channel{ID: visionGroupID, Status: StatusActive}
	cache.groupPlatform[visionGroupID] = ""
	cache.loadedAt = time.Now()
	channelService := &ChannelService{}
	channelService.cache.Store(cache)
	billingService := NewBillingService(&config.Config{}, nil)
	svc := &GatewayService{
		billingService: billingService,
		resolver:       NewModelPricingResolver(channelService, billingService),
	}
	sourceKey := &APIKey{GroupID: i64p(sourceGroupID), Group: &Group{ID: sourceGroupID}}

	cost := svc.calculateRecordUsageCost(
		context.Background(),
		&ForwardResult{Model: "gpt-vision", Usage: ClaudeUsage{InputTokens: 1000, OutputTokens: 100}},
		apiKeyForPricingGroup(sourceKey, visionGroupID),
		"gpt-vision",
		1,
		1,
		time.Now(),
	)

	require.Positive(t, cost.TotalCost)
	require.Equal(t, sourceGroupID, sourceKey.Group.ID)
	require.Equal(t, sourceGroupID, *sourceKey.GroupID)
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
