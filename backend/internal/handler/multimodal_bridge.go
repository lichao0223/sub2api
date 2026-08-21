package handler

import (
	"context"
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type openAIMultimodalForward func([]byte) (*service.OpenAIForwardResult, error)
type anthropicMultimodalForward func([]byte) (*service.ForwardResult, error)

func (h *OpenAIGatewayHandler) forwardWithMultimodal(
	ctx context.Context,
	c *gin.Context,
	account *service.Account,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	body []byte,
	forward openAIMultimodalForward,
) (*service.OpenAIForwardResult, error) {
	slog.Info("multimodal_bridge_forward_entered", "source_account_id", account.ID, "source_platform", account.Platform, "body_bytes", len(body))
	if h.multimodalGatewayService == nil {
		slog.Warn("multimodal_bridge_service_unavailable", "source_account_id", account.ID)
		return forward(body)
	}
	preparedBody, usage, err := h.multimodalGatewayService.PrepareMultimodal(
		ctx, c, h.gatewayService, account, body,
	)
	if err != nil {
		slog.Warn("multimodal_bridge_forward_failed", "source_account_id", account.ID, "error", err)
		return nil, err
	}
	if usage != nil {
		slog.Info("multimodal_bridge_usage_record_scheduled", "source_account_id", account.ID, "vision_account_id", usage.Account.ID, "vision_model", usage.Model)
		h.recordOpenAIMultimodalUsage(ctx, c, usage.Account, apiKey, subscription, body, usage)
	}
	slog.Info("multimodal_bridge_forward_prepared", "source_account_id", account.ID, "rewritten", usage != nil, "body_bytes", len(preparedBody))
	return forward(preparedBody)
}

func (h *GatewayHandler) forwardWithMultimodal(
	ctx context.Context,
	c *gin.Context,
	account *service.Account,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	body []byte,
	forward anthropicMultimodalForward,
) (*service.ForwardResult, error) {
	slog.Info("multimodal_bridge_forward_entered", "source_account_id", account.ID, "source_platform", account.Platform, "body_bytes", len(body))
	preparedBody, usage, err := h.gatewayService.PrepareMultimodal(
		ctx, c, h.openAIGatewayService, account, body,
	)
	if err != nil {
		slog.Warn("multimodal_bridge_forward_failed", "source_account_id", account.ID, "error", err)
		return nil, err
	}
	if usage != nil {
		slog.Info("multimodal_bridge_usage_record_scheduled", "source_account_id", account.ID, "vision_account_id", usage.Account.ID, "vision_model", usage.Model)
		h.recordAnthropicMultimodalUsage(ctx, c, usage.Account, apiKey, subscription, body, usage)
	}
	slog.Info("multimodal_bridge_forward_prepared", "source_account_id", account.ID, "rewritten", usage != nil, "body_bytes", len(preparedBody))
	return forward(preparedBody)
}

func (h *OpenAIGatewayHandler) recordOpenAIMultimodalUsage(
	ctx context.Context,
	c *gin.Context,
	account *service.Account,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	body []byte,
	usage *service.MultimodalBridgeUsage,
) {
	requestID := usage.RequestID
	if requestID == "" {
		requestID = "mm_" + uuid.NewString()
	}
	result := &service.OpenAIForwardResult{
		RequestID:        requestID,
		Model:            usage.Model,
		BillingModel:     usage.Model,
		UpstreamModel:    usage.Model,
		UpstreamEndpoint: EndpointChatCompletions,
		Duration:         usage.Duration,
		Usage: service.OpenAIUsage{
			InputTokens:              usage.InputTokens,
			OutputTokens:             usage.OutputTokens,
			CacheCreationInputTokens: usage.CacheCreationInputTokens,
			CacheReadInputTokens:     usage.CacheReadInputTokens,
		},
	}
	inboundEndpoint := GetInboundEndpoint(c)
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	requestPayloadHash := service.HashUsageRequestPayload(body)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	h.submitOpenAIUsageRecordTask(ctx, result, func(taskCtx context.Context) {
		if err := h.gatewayService.RecordUsage(taskCtx, &service.OpenAIRecordUsageInput{
			Result:             result,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   EndpointChatCompletions,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: requestPayloadHash,
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      quotaPlatform,
			PricingGroupID:     usage.PricingGroupID,
		}); err != nil {
			logger.L().Error("openai multimodal bridge usage record failed", zap.Error(err))
		}
	})
}

func (h *GatewayHandler) recordAnthropicMultimodalUsage(
	ctx context.Context,
	c *gin.Context,
	account *service.Account,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	body []byte,
	usage *service.MultimodalBridgeUsage,
) {
	requestID := usage.RequestID
	if requestID == "" {
		requestID = "mm_" + uuid.NewString()
	}
	result := &service.ForwardResult{
		RequestID:     requestID,
		Model:         usage.Model,
		UpstreamModel: usage.Model,
		Duration:      usage.Duration,
		Usage: service.ClaudeUsage{
			InputTokens:              usage.InputTokens,
			OutputTokens:             usage.OutputTokens,
			CacheCreationInputTokens: usage.CacheCreationInputTokens,
			CacheReadInputTokens:     usage.CacheReadInputTokens,
		},
	}
	inboundEndpoint := GetInboundEndpoint(c)
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	requestPayloadHash := service.HashUsageRequestPayload(body)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	h.submitUsageRecordTask(ctx, func(taskCtx context.Context) {
		if err := h.gatewayService.RecordUsage(taskCtx, &service.RecordUsageInput{
			Result:             result,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   EndpointMessages,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: requestPayloadHash,
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      quotaPlatform,
			PricingGroupID:     usage.PricingGroupID,
		}); err != nil {
			logger.L().Error("anthropic multimodal bridge usage record failed", zap.Error(err))
		}
	})
}
