package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	if account.IsOpenAIOAuth() {
		return s.describeOpenAIOAuthImage(ctx, c, account, visionModel, imageURL, index)
	}

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

func (s *OpenAIGatewayService) describeOpenAIOAuthImage(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	visionModel string,
	imageURL string,
	index int,
) (string, string, OpenAIUsage, error) {
	requestBody, err := json.Marshal(map[string]any{
		"model":             visionModel,
		"stream":            false,
		"max_output_tokens": multimodalBridgeMaxTokens,
		"input": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": multimodalBridgePrompt(index)},
				map[string]any{"type": "input_image", "image_url": imageURL},
			},
		}},
	})
	if err != nil {
		return "", "", OpenAIUsage{}, fmt.Errorf("build vision request: %w", err)
	}

	recorder := httptest.NewRecorder()
	capture, _ := gin.CreateTestContext(recorder)
	if c != nil {
		isolated := c.Copy()
		isolated.Writer = capture.Writer
		capture = isolated
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/v1/responses", nil)
	if err != nil {
		return "", "", OpenAIUsage{}, fmt.Errorf("build vision context: %w", err)
	}
	if c != nil && c.Request != nil {
		req.Header = c.Request.Header.Clone()
	}
	capture.Request = req
	SetOpenAIClientTransport(capture, OpenAIClientTransportHTTP)

	result, err := s.Forward(ctx, capture, account, requestBody)
	if err != nil {
		return "", "", OpenAIUsage{}, fmt.Errorf("vision model request failed: %w", err)
	}
	var response any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		return "", "", OpenAIUsage{}, fmt.Errorf("parse vision response: %w", err)
	}
	description := strings.TrimSpace(extractOpenAIResponsesCompletedText(response))
	if description == "" {
		return "", "", OpenAIUsage{}, fmt.Errorf("vision model returned an empty description")
	}
	requestID := firstNonEmpty(result.ResponseID, result.RequestID)
	return description, requestID, result.Usage, nil
}

func multimodalBridgePrompt(index int) string {
	return fmt.Sprintf(
		"Describe image %d in concise, factual text for a downstream text model. Include visible text, UI elements, diagrams, errors, and spatial relationships.",
		index,
	)
}
