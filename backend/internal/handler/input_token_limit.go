package handler

import (
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tokenestimate"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const inputTokenLimitErrorCode = "input_token_limit_exceeded"

func checkInputTokenLimit(c *gin.Context, reqLog *zap.Logger, cfg *config.Config, model string, body []byte, stage string, settings ...*service.SettingService) *securityaudit.Decision {
	limit := config.DefaultGatewayMaxInputTokens
	if cfg != nil && cfg.Gateway.MaxInputTokens > 0 {
		limit = cfg.Gateway.MaxInputTokens
	}
	if len(settings) > 0 && settings[0] != nil {
		limit = settings[0].GetGatewayMaxInputTokens(c.Request.Context())
	}
	estimated := tokenestimate.Count(body, limit)
	if estimated <= limit {
		return nil
	}
	message := fmt.Sprintf("Estimated input exceeds the %d token limit (at least %d tokens). Reduce conversation history, tool results or input text.", limit, estimated)
	c.Set(opsModelKey, model)
	service.MarkOpsRequestScopedError(c, "invalid_request_error")
	if isSecurityAuditWebSocketStage(stage) {
		// WebSocket's HTTP status is already 101; use the existing in-band error logger.
		service.MarkOpsStreamErrorValue(c, service.OpsStreamError{
			ErrType: "invalid_request_error", Code: inputTokenLimitErrorCode, Message: message,
			IntendedStatus: http.StatusBadRequest, RequestScoped: true,
		})
	}
	if reqLog != nil {
		reqLog.Warn("gateway.input_token_limit_exceeded", zap.String("model", model),
			zap.String("stage", stage), zap.Int("body_bytes", len(body)),
			zap.Int("estimated_input_tokens_at_least", estimated), zap.Int("input_token_limit", limit))
	}
	return &securityaudit.Decision{
		Kind: securityaudit.DecisionBlock, HTTPStatus: http.StatusBadRequest,
		ErrorCode: inputTokenLimitErrorCode, ClientMessage: message, AllowNextStage: false,
	}
}
