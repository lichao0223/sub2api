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
// on the same Anthropic API-key account.
func (s *GatewayService) PrepareMultimodal(
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
		description, requestID, tokens, describeErr := s.describeAnthropicImage(
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
		"stream":     false,
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
	upstreamReq, _, err := s.buildUpstreamRequest(ctx, c, account, requestBody, token, tokenType, visionModel, false, false)
	if err != nil {
		return "", "", ClaudeUsage{}, fmt.Errorf("build vision upstream request: %w", err)
	}
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
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
		return "", "", ClaudeUsage{}, fmt.Errorf("%s", message)
	}

	description := firstAnthropicText(respBody)
	if description == "" {
		return "", "", ClaudeUsage{}, fmt.Errorf("vision model returned an empty description")
	}
	usage := parseClaudeUsageFromResponseBody(respBody)
	requestID := firstNonEmpty(gjson.GetBytes(respBody, "id").String(), resp.Header.Get("request-id"))
	return description, requestID, *usage, nil
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

func firstAnthropicText(body []byte) string {
	content := gjson.GetBytes(body, "content")
	var text string
	content.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "text" {
			text = strings.TrimSpace(item.Get("text").String())
		}
		return text == ""
	})
	return text
}
