package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

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
