package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// PrepareMultimodal converts image blocks to descriptions with a vision model
// on the same OpenAI API-key account.
func (s *OpenAIGatewayService) PrepareMultimodal(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) ([]byte, *MultimodalBridgeUsage, error) {
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	policy := account.multimodalPolicy(model)
	if policy.Mode != multimodalModeVisionToText || !requestBodyHasImageInput(body) {
		return body, nil, nil
	}
	if account.Type != AccountTypeAPIKey || policy.VisionModel == "" {
		return body, nil, fmt.Errorf("vision-to-text requires an API-key account and vision model")
	}

	root, images, err := imageBridgeInput(body)
	if err != nil || len(images) == 0 {
		return body, nil, err
	}

	startedAt := time.Now()
	usage := &MultimodalBridgeUsage{Model: policy.VisionModel}
	descriptions := make([]string, 0, len(images))
	for index, imageURL := range images {
		description, requestID, tokens, describeErr := s.describeOpenAIImage(
			ctx, c, account, policy.VisionModel, imageURL, index+1,
		)
		if describeErr != nil {
			return body, nil, describeErr
		}
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
		return body, nil, err
	}
	return rewritten, usage, nil
}

func (s *OpenAIGatewayService) describeOpenAIImage(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	visionModel string,
	imageURL string,
	index int,
) (string, string, OpenAIUsage, error) {
	token, targetURL, err := s.resolveCCFallbackTarget(account)
	if err != nil {
		return "", "", OpenAIUsage{}, err
	}
	requestBody, err := json.Marshal(map[string]any{
		"model":      visionModel,
		"stream":     false,
		"max_tokens": multimodalBridgeMaxTokens,
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": multimodalBridgePrompt(index)},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}},
			},
		}},
	})
	if err != nil {
		return "", "", OpenAIUsage{}, fmt.Errorf("build vision request: %w", err)
	}

	resp, err := s.sendCCUpstreamRequest(ctx, c, account, targetURL, requestBody, false, token, account.GetOpenAIUserAgent(), "")
	if err != nil {
		return "", "", OpenAIUsage{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, nil)
	if err != nil {
		return "", "", OpenAIUsage{}, fmt.Errorf("read vision response: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		message := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
		if message == "" {
			message = fmt.Sprintf("vision model returned status %d", resp.StatusCode)
		}
		return "", "", OpenAIUsage{}, fmt.Errorf("%s", message)
	}

	description := strings.TrimSpace(gjson.GetBytes(respBody, "choices.0.message.content").String())
	if description == "" {
		return "", "", OpenAIUsage{}, fmt.Errorf("vision model returned an empty description")
	}
	usage, _ := extractOpenAIUsageFromJSONBytes(respBody)
	requestID := firstNonEmpty(gjson.GetBytes(respBody, "id").String(), resp.Header.Get("x-request-id"))
	return description, requestID, usage, nil
}

func multimodalBridgePrompt(index int) string {
	return fmt.Sprintf(
		"Describe image %d in concise, factual text for a downstream text model. Include visible text, UI elements, diagrams, errors, and spatial relationships.",
		index,
	)
}
