package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

const (
	multimodalDefaultModeKey        = "multimodal_default_mode"
	multimodalModelModesKey         = "multimodal_model_modes"
	multimodalDefaultVisionModelKey = "multimodal_default_vision_model"
	multimodalVisionModelsKey       = "multimodal_vision_models"
	multimodalModeReject            = "reject"
	multimodalModeVisionToText      = "vision_to_text"
	// ponytail: fixed cap prevents unbounded secondary calls; make configurable only if real workloads need more.
	multimodalBridgeMaxImages = 8
	multimodalBridgeMaxTokens = 512
)

type multimodalRequestContextKey struct{}

type multimodalPolicy struct {
	Mode        string
	VisionModel string
}

// MultimodalBridgeUsage is billed as a separate request because its model may
// have different pricing from the downstream text model.
type MultimodalBridgeUsage struct {
	RequestID                string
	Model                    string
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
	Duration                 time.Duration
}

// WithMultimodalRequest marks image requests so the existing schedulers can
// skip accounts configured not to accept them.
func WithMultimodalRequest(ctx context.Context, body []byte) context.Context {
	if !requestBodyHasImageInput(body) {
		return ctx
	}
	return context.WithValue(ctx, multimodalRequestContextKey{}, true)
}

func isMultimodalRequest(ctx context.Context) bool {
	multimodal, _ := ctx.Value(multimodalRequestContextKey{}).(bool)
	return multimodal
}

func (a *Account) acceptsMultimodalRequest(requestedModel string) bool {
	if a == nil || (a.Platform != PlatformOpenAI && a.Platform != PlatformAnthropic) {
		return true
	}

	policy := a.multimodalPolicy(requestedModel)
	if policy.Mode == multimodalModeVisionToText {
		return a.Type == AccountTypeAPIKey && policy.VisionModel != ""
	}
	return policy.Mode != multimodalModeReject
}

func (a *Account) multimodalPolicy(requestedModel string) multimodalPolicy {
	mode, _ := a.Credentials[multimodalDefaultModeKey].(string)
	visionModel, _ := a.Credentials[multimodalDefaultVisionModelKey].(string)
	mappedModel := a.GetMappedModel(requestedModel)
	if value := credentialStringMapValue(a.Credentials[multimodalModelModesKey], mappedModel); value != "" {
		mode = value
	}
	if value := credentialStringMapValue(a.Credentials[multimodalVisionModelsKey], mappedModel); value != "" {
		visionModel = value
	}
	return multimodalPolicy{
		Mode:        strings.TrimSpace(strings.ToLower(mode)),
		VisionModel: strings.TrimSpace(visionModel),
	}
}

func credentialStringMapValue(raw any, key string) string {
	switch values := raw.(type) {
	case map[string]any:
		value, _ := values[key].(string)
		return value
	case map[string]string:
		return values[key]
	default:
		return ""
	}
}

func requestBodyHasImageInput(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	return jsonValueHasImageInput(gjson.GetBytes(body, "input")) ||
		jsonValueHasImageInput(gjson.GetBytes(body, "messages"))
}

func jsonValueHasImageInput(value gjson.Result) bool {
	if !value.Exists() {
		return false
	}
	if value.IsArray() {
		found := false
		value.ForEach(func(_, item gjson.Result) bool {
			found = jsonValueHasImageInput(item)
			return !found
		})
		return found
	}
	if !value.IsObject() {
		return false
	}

	switch strings.TrimSpace(strings.ToLower(value.Get("type").String())) {
	case "image", "image_url", "input_image":
		return true
	}
	return jsonValueHasImageInput(value.Get("content"))
}

func imageBridgeInput(body []byte) (map[string]any, []string, error) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, nil, fmt.Errorf("parse multimodal request: %w", err)
	}
	var images []string
	for _, key := range []string{"input", "messages"} {
		collectImageBridgeInputs(root[key], &images)
	}
	if len(images) == 0 {
		return nil, nil, fmt.Errorf("vision-to-text requires URL or base64 image input")
	}
	if len(images) > multimodalBridgeMaxImages {
		return nil, nil, fmt.Errorf("vision-to-text supports at most %d images per request", multimodalBridgeMaxImages)
	}
	return root, images, nil
}

func collectImageBridgeInputs(value any, images *[]string) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectImageBridgeInputs(item, images)
		}
	case map[string]any:
		if imageURL := imageBridgeURL(typed); imageURL != "" {
			*images = append(*images, imageURL)
			return
		}
		collectImageBridgeInputs(typed["content"], images)
	}
}

func imageBridgeURL(part map[string]any) string {
	partType := strings.TrimSpace(strings.ToLower(fmt.Sprint(part["type"])))
	switch partType {
	case "image_url", "input_image":
		switch value := part["image_url"].(type) {
		case string:
			return validImageBridgeURL(value)
		case map[string]any:
			return validImageBridgeURL(fmt.Sprint(value["url"]))
		}
	case "image":
		source, _ := part["source"].(map[string]any)
		switch strings.TrimSpace(strings.ToLower(fmt.Sprint(source["type"]))) {
		case "base64":
			mediaType := strings.TrimSpace(fmt.Sprint(source["media_type"]))
			data := strings.TrimSpace(fmt.Sprint(source["data"]))
			if mediaType != "" && data != "" {
				return "data:" + mediaType + ";base64," + data
			}
		case "url":
			return strings.TrimSpace(fmt.Sprint(source["url"]))
		}
	}
	return ""
}

func validImageBridgeURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || isEmptyBase64DataURI(value) {
		return ""
	}
	return value
}

func rewriteImagesAsText(root map[string]any, descriptions []string) ([]byte, error) {
	index := 0
	for _, key := range []string{"input", "messages"} {
		root[key] = rewriteImageBridgeValue(root[key], descriptions, &index)
	}
	if index != len(descriptions) {
		return nil, fmt.Errorf("multimodal image count changed during rewrite")
	}
	body, err := json.Marshal(root)
	if err == nil && requestBodyHasImageInput(body) {
		return nil, fmt.Errorf("vision-to-text cannot convert one or more image inputs")
	}
	return body, err
}

func rewriteImageBridgeValue(value any, descriptions []string, index *int) any {
	switch typed := value.(type) {
	case []any:
		for i, item := range typed {
			typed[i] = rewriteImageBridgeValue(item, descriptions, index)
		}
	case map[string]any:
		if imageBridgeURL(typed) != "" {
			if *index >= len(descriptions) {
				return typed
			}
			partType := strings.TrimSpace(strings.ToLower(fmt.Sprint(typed["type"])))
			textType := "text"
			if partType == "input_image" {
				textType = "input_text"
			}
			text := fmt.Sprintf("Image %d description: %s", *index+1, strings.TrimSpace(descriptions[*index]))
			*index++
			return map[string]any{"type": textType, "text": text}
		}
		typed["content"] = rewriteImageBridgeValue(typed["content"], descriptions, index)
	}
	return value
}
