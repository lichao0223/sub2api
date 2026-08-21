package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// PrepareMultimodal converts image blocks to descriptions with a vision model.
// A configured vision group uses the existing group scheduler; otherwise the
// already-selected source account is used for backward compatibility.
func (s *GatewayService) PrepareMultimodal(
	ctx context.Context,
	c *gin.Context,
	openAIService *OpenAIGatewayService,
	account *Account,
	body []byte,
) ([]byte, *MultimodalBridgeUsage, error) {
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	policy := account.multimodalPolicy(model)
	hasImageInput := requestBodyHasImageInput(body)
	if hasImageInput || policy.Mode != "" || policy.VisionModel != "" || policy.VisionGroupID > 0 {
		slog.Info("multimodal_bridge_check",
			"source_account_id", account.ID,
			"source_platform", account.Platform,
			"source_model", model,
			"policy_mode", policy.Mode,
			"vision_group_id", policy.VisionGroupID,
			"vision_model", policy.VisionModel,
			"image_input_detected", hasImageInput,
			"body_bytes", len(body),
		)
	}
	if policy.Mode != multimodalModeVisionToText || !hasImageInput {
		if hasImageInput || policy.Mode != "" || policy.VisionModel != "" || policy.VisionGroupID > 0 {
			slog.Info("multimodal_bridge_skipped",
				"source_account_id", account.ID,
				"source_model", model,
				"reason", multimodalSkipReason(policy.Mode, policy.VisionModel, hasImageInput),
			)
		}
		return body, nil, nil
	}
	slog.Info("multimodal_bridge_started",
		"source_account_id", account.ID,
		"source_platform", account.Platform,
		"source_model", model,
		"vision_group_id", policy.VisionGroupID,
		"vision_model", policy.VisionModel,
	)
	if policy.VisionModel == "" {
		slog.Warn("multimodal_bridge_missing_vision_model", "source_account_id", account.ID, "source_model", model)
		return body, nil, fmt.Errorf("vision-to-text requires a vision model")
	}

	root, images, err := imageBridgeInput(body)
	if err != nil || len(images) == 0 {
		slog.Warn("multimodal_bridge_input_parse_failed",
			"source_account_id", account.ID,
			"source_model", model,
			"error", err,
		)
		return body, nil, err
	}
	slog.Info("multimodal_bridge_images_parsed", "source_account_id", account.ID, "image_count", len(images))

	targetAccount := account
	if policy.VisionGroupID > 0 {
		selectedAccount, release, selectErr := s.selectMultimodalVisionAccount(ctx, policy.VisionGroupID, policy.VisionModel)
		if selectErr != nil {
			slog.Warn("multimodal_bridge_vision_account_selection_failed",
				"source_account_id", account.ID,
				"vision_group_id", policy.VisionGroupID,
				"vision_model", policy.VisionModel,
				"error", selectErr,
			)
			return body, nil, selectErr
		}
		targetAccount = selectedAccount
		slog.Info("multimodal_bridge_vision_account_selected",
			"source_account_id", account.ID,
			"vision_group_id", policy.VisionGroupID,
			"vision_account_id", targetAccount.ID,
			"vision_platform", targetAccount.Platform,
			"vision_model", policy.VisionModel,
		)
		defer release()
	}
	if targetAccount.Type != AccountTypeAPIKey && !targetAccount.IsOpenAIOAuth() {
		return body, nil, fmt.Errorf("vision-to-text requires an API-key or OpenAI OAuth target account")
	}
	if targetAccount.Platform != PlatformAnthropic && !targetAccount.IsOpenAICompatible() {
		slog.Warn("multimodal_bridge_unsupported_vision_platform",
			"account_id", targetAccount.ID,
			"platform", targetAccount.Platform,
			"vision_model", policy.VisionModel,
		)
		return body, nil, fmt.Errorf("vision-to-text target platform does not support image description")
	}

	visionModel := targetAccount.GetMappedModel(policy.VisionModel)
	startedAt := time.Now()
	usage := &MultimodalBridgeUsage{
		Account:        targetAccount,
		PricingGroupID: policy.VisionGroupID,
		Model:          visionModel,
	}
	descriptions := make([]string, 0, len(images))
	for index, imageURL := range images {
		description, requestID, tokens, describeErr := s.describeMultimodalImage(
			ctx, c, openAIService, targetAccount, visionModel, imageURL, index+1,
		)
		if describeErr != nil {
			slog.Warn("multimodal_bridge_vision_request_failed",
				"source_account_id", account.ID,
				"vision_account_id", targetAccount.ID,
				"vision_platform", targetAccount.Platform,
				"vision_model", visionModel,
				"image_index", index+1,
				"error", describeErr,
			)
			return body, nil, describeErr
		}
		slog.Info("multimodal_bridge_image_described",
			"source_account_id", account.ID,
			"vision_account_id", targetAccount.ID,
			"vision_model", visionModel,
			"image_index", index+1,
			"description_bytes", len(description),
			"request_id", requestID,
		)
		if usage.RequestID == "" {
			usage.RequestID = requestID
		}
		usage.InputTokens += tokens.InputTokens
		usage.OutputTokens += tokens.OutputTokens
		usage.CacheCreationInputTokens += tokens.CacheCreationInputTokens
		usage.CacheReadInputTokens += tokens.CacheReadInputTokens
		descriptions = append(descriptions, description)
	}
	usage.Duration = time.Since(startedAt)

	rewritten, err := rewriteImagesAsText(root, descriptions)
	if err != nil {
		slog.Warn("multimodal_bridge_rewrite_failed", "source_account_id", account.ID, "source_model", model, "error", err)
		return body, nil, err
	}
	slog.Info("multimodal_bridge_completed",
		"source_account_id", account.ID,
		"source_model", model,
		"vision_account_id", targetAccount.ID,
		"vision_model", visionModel,
		"image_count", len(images),
		"duration_ms", usage.Duration.Milliseconds(),
		"input_tokens", usage.InputTokens,
		"output_tokens", usage.OutputTokens,
	)
	return rewritten, usage, nil
}

func multimodalSkipReason(mode, visionModel string, hasImageInput bool) string {
	if !hasImageInput {
		return "image_input_not_detected"
	}
	if mode != multimodalModeVisionToText {
		return "policy_mode_not_vision_to_text"
	}
	if strings.TrimSpace(visionModel) == "" {
		return "vision_model_not_configured"
	}
	return "not_applicable"
}

func (s *GatewayService) selectMultimodalVisionAccount(
	ctx context.Context,
	groupID int64,
	model string,
) (*Account, func(), error) {
	excluded := make(map[int64]struct{})
	for {
		selection, err := s.SelectAccountWithLoadAwareness(ctx, &groupID, "", model, excluded, "", 0)
		if err != nil {
			return nil, nil, fmt.Errorf("select vision account: %w", err)
		}
		account := selection.Account
		if account == nil {
			return nil, nil, fmt.Errorf("select vision account: no account returned")
		}
		if (account.Type != AccountTypeAPIKey && !account.IsOpenAIOAuth()) ||
			(account.Platform != PlatformAnthropic && !account.IsOpenAICompatible()) {
			if selection.Acquired && selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			excluded[account.ID] = struct{}{}
			continue
		}
		if !selection.Acquired || selection.ReleaseFunc == nil {
			excluded[account.ID] = struct{}{}
			continue
		}
		return account, selection.ReleaseFunc, nil
	}
}

func (s *GatewayService) describeMultimodalImage(
	ctx context.Context,
	c *gin.Context,
	openAIService *OpenAIGatewayService,
	account *Account,
	visionModel string,
	imageURL string,
	index int,
) (string, string, ClaudeUsage, error) {
	if account.Platform != PlatformAnthropic {
		if openAIService == nil {
			return "", "", ClaudeUsage{}, fmt.Errorf("OpenAI-compatible vision gateway is unavailable")
		}
		slog.Info("multimodal_bridge_upstream_request",
			"vision_account_id", account.ID,
			"vision_platform", account.Platform,
			"vision_model", visionModel,
			"image_index", index,
			"protocol", "openai_chat_completions",
		)
		description, requestID, usage, err := openAIService.describeOpenAIImage(
			ctx, c, account, visionModel, imageURL, index,
		)
		return description, requestID, ClaudeUsage{
			InputTokens:              usage.InputTokens,
			OutputTokens:             usage.OutputTokens,
			CacheCreationInputTokens: usage.CacheCreationInputTokens,
			CacheReadInputTokens:     usage.CacheReadInputTokens,
		}, err
	}
	return s.describeAnthropicImage(ctx, c, account, visionModel, imageURL, index)
}

func (s *GatewayService) describeAnthropicImage(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	visionModel string,
	imageURL string,
	index int,
) (string, string, ClaudeUsage, error) {
	imageBlock, err := anthropicImageBlock(imageURL)
	if err != nil {
		return "", "", ClaudeUsage{}, err
	}
	requestBody, err := json.Marshal(map[string]any{
		"model":      visionModel,
		"stream":     true,
		"max_tokens": multimodalBridgeMaxTokens,
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": multimodalBridgePrompt(index)},
				imageBlock,
			},
		}},
	})
	if err != nil {
		return "", "", ClaudeUsage{}, fmt.Errorf("build vision request: %w", err)
	}

	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return "", "", ClaudeUsage{}, err
	}
	upstreamReq, _, err := s.buildUpstreamRequest(ctx, c, account, requestBody, token, tokenType, visionModel, true, false)
	if err != nil {
		return "", "", ClaudeUsage{}, fmt.Errorf("build vision upstream request: %w", err)
	}
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	slog.Info("multimodal_bridge_upstream_request",
		"vision_account_id", account.ID,
		"vision_platform", account.Platform,
		"vision_model", visionModel,
		"image_index", index,
		"protocol", "anthropic_messages",
	)
	resp, err := s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		slog.Warn("multimodal_bridge_upstream_transport_failed", "vision_account_id", account.ID, "vision_model", visionModel, "image_index", index, "error", err)
		return "", "", ClaudeUsage{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, nil)
	if err != nil {
		return "", "", ClaudeUsage{}, fmt.Errorf("read vision response: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		message := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
		if message == "" {
			message = fmt.Sprintf("vision model returned status %d", resp.StatusCode)
		}
		slog.Warn("multimodal_bridge_upstream_response_failed", "vision_account_id", account.ID, "vision_model", visionModel, "image_index", index, "status", resp.StatusCode, "error", message)
		return "", "", ClaudeUsage{}, fmt.Errorf("%s", message)
	}

	var description strings.Builder
	var usage ClaudeUsage
	requestID := resp.Header.Get("request-id")
	var streamErr string
	forEachOpenAISSEDataPayload(string(respBody), func(data []byte) {
		s.parseSSEUsage(string(data), &usage)
		if gjson.GetBytes(data, "type").String() == "content_block_delta" &&
			gjson.GetBytes(data, "delta.type").String() == "text_delta" {
			_, _ = description.WriteString(gjson.GetBytes(data, "delta.text").String())
		}
		requestID = firstNonEmpty(gjson.GetBytes(data, "message.id").String(), requestID)
		streamErr = firstNonEmpty(streamErr, gjson.GetBytes(data, "error.message").String())
	})
	if streamErr != "" {
		return "", "", ClaudeUsage{}, fmt.Errorf("%s", sanitizeUpstreamErrorMessage(streamErr))
	}
	result := strings.TrimSpace(description.String())
	if result == "" {
		slog.Warn("multimodal_bridge_empty_description", "vision_account_id", account.ID, "vision_model", visionModel, "image_index", index)
		return "", "", ClaudeUsage{}, fmt.Errorf("vision model returned an empty description")
	}
	return result, requestID, usage, nil
}

func anthropicImageBlock(imageURL string) (map[string]any, error) {
	if strings.HasPrefix(imageURL, "data:") {
		header, data, ok := strings.Cut(imageURL, ",")
		mediaType := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
		if !ok || !strings.HasSuffix(header, ";base64") || mediaType == "" || data == "" {
			return nil, fmt.Errorf("invalid base64 image")
		}
		return map[string]any{
			"type":   "image",
			"source": map[string]any{"type": "base64", "media_type": mediaType, "data": data},
		}, nil
	}
	return map[string]any{
		"type":   "image",
		"source": map[string]any{"type": "url", "url": imageURL},
	}, nil
}
