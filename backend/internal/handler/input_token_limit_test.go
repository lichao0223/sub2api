package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func inputLimitBody(t *testing.T, text string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]string{"input": text})
	require.NoError(t, err)
	return body
}

func TestInputTokenLimitRunsBeforeAuditAndCachedCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stage := range []string{"http", "first_turn", "subsequent_turn"} {
		t.Run(stage, func(t *testing.T) {
			c, _ := securityAuditErrorTestContext(t)
			c.Set(securityAuditCompletedContextKey, true)
			engine := &turnCountingEngine{mode: securityaudit.ModeBlocking}
			decision := runSecurityAudit(c, nil, securityaudit.NewCoordinator(nil, engine), nil, nil, nil,
				middleware2.AuthSubject{UserID: 7}, "openai_responses", "test-model",
				inputLimitBody(t, strings.Repeat(" the", 300001)), stage)
			require.NotNil(t, decision)
			require.False(t, decision.AllowNextStage)
			require.Equal(t, inputTokenLimitErrorCode, decision.ErrorCode)
			require.Contains(t, decision.ClientMessage, "300000")
			require.Zero(t, engine.evaluates.Load(), "must not invoke the audit model")
			require.Zero(t, engine.enqueues.Load())
			require.Equal(t, http.StatusBadRequest, decision.HTTPStatus)
		})
	}
}

func TestInputTokenLimitBoundaryAndConfig(t *testing.T) {
	c, _ := securityAuditErrorTestContext(t)
	require.Nil(t, checkInputTokenLimit(c, nil, nil, "m", inputLimitBody(t, strings.Repeat(" the", 300000)), "http"))
	require.NotNil(t, checkInputTokenLimit(c, nil, nil, "m", inputLimitBody(t, strings.Repeat(" the", 300001)), "http"))
	require.Nil(t, checkInputTokenLimit(c, nil, nil, "m", inputLimitBody(t, strings.Repeat("中", 300000)), "http"))
	require.NotNil(t, checkInputTokenLimit(c, nil, nil, "m", inputLimitBody(t, strings.Repeat("中", 300001)), "http"))
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxInputTokens: 10}}
	require.Nil(t, checkInputTokenLimit(c, nil, cfg, "m", inputLimitBody(t, strings.Repeat(" the", 10)), "http"))
	require.NotNil(t, checkInputTokenLimit(c, nil, cfg, "m", inputLimitBody(t, strings.Repeat(" the", 11)), "http"))
}

func TestInputTokenLimitDoesNotRejectHistoryFromCharacterRatios(t *testing.T) {
	c, _ := securityAuditErrorTestContext(t)
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxInputTokens: 40000}}
	// The former ASCII ratio estimates 40,920 tokens; DeepSeek counts about 34k.
	history := strings.Repeat("the quick brown fox jumps over the lazy dog.\n", 3100)
	body, err := json.Marshal(map[string]any{
		"input": []any{map[string]string{"type": "function_call_output", "output": history}},
	})
	require.NoError(t, err)
	for _, model := range []string{"deepseek-flash-latest", "gpt-5", "claude-sonnet", "gemini-pro"} {
		require.Nil(t, checkInputTokenLimit(c, nil, cfg, model, body, "http"), model)
	}
	decision := checkInputTokenLimit(c, nil, cfg, "deepseek-flash-latest",
		inputLimitBody(t, strings.Repeat(" the", 40001)), "http")
	require.NotNil(t, decision)
	require.Contains(t, decision.ClientMessage, "local DeepSeek count reached 40001")
}

func TestInputTokenLimitRejectsImageHeavyInputBeforeAudit(t *testing.T) {
	c, _ := securityAuditErrorTestContext(t)
	image := `{"type":"input_image","image_url":"https://example.test/image.png"}`
	body := []byte(`{"input":[` + strings.TrimSuffix(strings.Repeat(image+",", 393), ",") + `]}`)
	engine := &turnCountingEngine{mode: securityaudit.ModeBlocking}
	decision := runSecurityAudit(c, nil, securityaudit.NewCoordinator(nil, engine), nil, nil, nil,
		middleware2.AuthSubject{UserID: 7}, "openai_responses", "test-model", body, "http")
	require.NotNil(t, decision)
	require.Equal(t, inputTokenLimitErrorCode, decision.ErrorCode)
	require.False(t, decision.AllowNextStage)
	require.Zero(t, engine.evaluates.Load())
	require.Zero(t, engine.enqueues.Load())
}

func TestInputTokenLimitRejectsWithoutAuditConfiguredAndLogsHTTPError(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 1)
	gin.SetMode(gin.TestMode)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops), securityAuditMediaTestMiddleware)
	forwarded := false
	router.POST("/v1/responses", func(c *gin.Context) {
		h := &OpenAIGatewayHandler{}
		key, _ := middleware2.GetAPIKeyFromContext(c)
		subject, _ := middleware2.GetAuthSubjectFromContext(c)
		decision := h.checkSecurityAudit(c, nil, key, subject, "openai_responses", "test-model",
			inputLimitBody(t, strings.Repeat(" the", 300001)))
		if decision != nil && !decision.AllowNextStage {
			h.openAISecurityAuditError(c, decision)
			return
		}
		forwarded = true
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
	require.False(t, forwarded)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), inputTokenLimitErrorCode)
	require.Contains(t, recorder.Body.String(), "invalid_request_error")
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	entry := (<-opsErrorLogQueue).entry
	require.Equal(t, "request", entry.ErrorPhase)
	require.Equal(t, "client_request", entry.ErrorSource)
	require.Equal(t, "invalid_request_error", entry.ErrorType)
	require.True(t, entry.IsBusinessLimited)
	require.Equal(t, http.StatusBadRequest, entry.StatusCode)
	require.Equal(t, int64(7), *entry.UserID)
	require.Equal(t, int64(9), *entry.APIKeyID)
	require.Equal(t, "test-model", entry.Model)
	require.Nil(t, entry.AccountID)
	require.Contains(t, entry.ErrorBody, inputTokenLimitErrorCode)
	require.Contains(t, entry.ErrorMessage, "300000")
	require.NotContains(t, entry.ErrorBody, strings.Repeat(" the", 25))
	require.Equal(t, string(inputLimitBody(t, strings.Repeat(" the", 300001))), decodeRequestSnapshot(t, entry.RequestSnapshotGzip))
}

func TestInputTokenLimitWebSocketErrorIsRecordedAfterUpgrade(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 1)
	gin.SetMode(gin.TestMode)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops), securityAuditMediaTestMiddleware)
	router.GET("/v1/responses", func(c *gin.Context) {
		c.Status(http.StatusSwitchingProtocols)
		setOpenAIClientTransportWS(c)
		c.Set(opsAccountIDKey, int64(123))
		c.Set(opsUpstreamModelKey, "previous-turn-model")
		service.BeginOpsStreamTurn(c, 2)
		decision := checkInputTokenLimit(c, nil, nil, "test-model", inputLimitBody(t, strings.Repeat(" the", 300001)), "subsequent_turn")
		require.NotNil(t, decision)
		require.Equal(t, coderws.StatusPolicyViolation, securityAuditWSCloseStatus(decision))
		require.Equal(t, inputTokenLimitErrorCode, securityAuditWSCloseReason(decision))
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/responses", nil))
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	entry := (<-opsErrorLogQueue).entry
	require.Equal(t, http.StatusBadRequest, entry.StatusCode)
	require.Equal(t, "request", entry.ErrorPhase)
	require.Equal(t, "client_request", entry.ErrorSource)
	require.Contains(t, entry.ErrorBody, inputTokenLimitErrorCode)
	require.Nil(t, entry.UpstreamStatusCode)
	require.Nil(t, entry.AccountID)
	require.Empty(t, entry.UpstreamModel)
	require.Empty(t, entry.UpstreamEndpoint)
}

func TestInputTokenLimitProtocolErrorEnvelopes(t *testing.T) {
	for _, write := range []func(*gin.Context, *securityaudit.Decision){
		(&OpenAIGatewayHandler{}).openAISecurityAuditError,
		(&GatewayHandler{}).openAISecurityAuditError,
		(&GatewayHandler{}).responsesSecurityAuditError,
		(&GatewayHandler{}).anthropicSecurityAuditError,
		(&OpenAIGatewayHandler{}).anthropicSecurityAuditError,
		googleSecurityAuditError,
	} {
		c, recorder := securityAuditErrorTestContext(t)
		decision := checkInputTokenLimit(c, nil, nil, "m", inputLimitBody(t, strings.Repeat(" the", 300001)), "http")
		write(c, decision)
		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Contains(t, recorder.Body.String(), inputTokenLimitErrorCode)
		require.NotContains(t, recorder.Body.String(), "permission_error")
	}
}

func TestInputTokenLimitRealGenerationHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxBodySize: 4 * 1024 * 1024}}
	engine := &turnCountingEngine{mode: securityaudit.ModeBlocking}
	coordinator := securityaudit.NewCoordinator(nil, engine)
	openAI := &OpenAIGatewayHandler{
		cfg: cfg, gatewayService: &service.OpenAIGatewayService{},
		billingCacheService: &service.BillingCacheService{}, apiKeyService: &service.APIKeyService{},
		concurrencyHelper:        &ConcurrencyHelper{concurrencyService: &service.ConcurrencyService{}},
		securityAuditCoordinator: coordinator,
	}
	gateway := &GatewayHandler{cfg: cfg, gatewayService: &service.GatewayService{}, securityAuditCoordinator: coordinator}
	large := strings.Repeat(" the", 300001)
	chatBody := map[string]any{"model": "test-model", "max_tokens": 100, "messages": []any{map[string]string{"role": "user", "content": large}}}
	responsesBody := map[string]any{"model": "test-model", "input": large}
	geminiBody := map[string]any{"contents": []any{map[string]any{"parts": []any{map[string]string{"text": large}}}}}
	for _, tt := range []struct {
		name, path, platform string
		handle               gin.HandlerFunc
		body                 any
	}{
		{"openai responses", "/v1/responses", service.PlatformOpenAI, openAI.Responses, responsesBody},
		{"openai chat", "/v1/chat/completions", service.PlatformOpenAI, openAI.ChatCompletions, chatBody},
		{"openai messages", "/v1/messages", service.PlatformOpenAI, openAI.Messages, chatBody},
		{"claude responses", "/v1/responses", service.PlatformAnthropic, gateway.Responses, responsesBody},
		{"claude chat", "/v1/chat/completions", service.PlatformAnthropic, gateway.ChatCompletions, chatBody},
		{"claude messages", "/v1/messages", service.PlatformAnthropic, gateway.Messages, chatBody},
		{"gemini", "/v1beta/models/*modelAction", service.PlatformGemini, gateway.GeminiV1BetaModels, geminiBody},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(securityAuditMediaTestMiddleware, func(c *gin.Context) {
				key, _ := middleware2.GetAPIKeyFromContext(c)
				key.Group.Platform = tt.platform
				key.Group.AllowMessagesDispatch = true
				c.Next()
			})
			router.POST(tt.path, tt.handle)
			raw, err := json.Marshal(tt.body)
			require.NoError(t, err)
			path := tt.path
			if tt.platform == service.PlatformGemini {
				path = "/v1beta/models/gemini-test:generateContent"
			}
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw)))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			require.NotPanics(t, func() { router.ServeHTTP(recorder, request) })
			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
			require.Contains(t, recorder.Body.String(), inputTokenLimitErrorCode)
			require.Zero(t, engine.evaluates.Load(), "must reject before audit or unconfigured downstream services")
		})
	}
}
