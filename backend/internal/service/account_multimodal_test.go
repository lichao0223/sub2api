package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountMultimodalScheduling(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping":           map[string]any{"gpt-alias": "text-only"},
			multimodalDefaultModeKey:  "passthrough",
			multimodalModelModesKey:   map[string]any{"text-only": "vision_to_text"},
			multimodalVisionModelsKey: map[string]any{"text-only": "gpt-4.1-mini"},
		},
		Type: AccountTypeAPIKey,
	}

	require.True(t, account.acceptsMultimodalRequest("gpt-alias"))
	require.Equal(t, "gpt-4.1-mini", account.multimodalPolicy("gpt-alias").VisionModel)
	require.True(t, account.acceptsMultimodalRequest("vision-model"))
	require.True(t, isMultimodalRequest(WithMultimodalRequest(context.Background(),
		[]byte(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}]}`))))
	require.True(t, isMultimodalRequest(WithMultimodalRequest(context.Background(),
		[]byte(`{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","data":"abc"}}]}]}`))))
	require.False(t, isMultimodalRequest(WithMultimodalRequest(context.Background(),
		[]byte(`{"messages":[{"role":"user","content":"image_url is only text"}]}`))))
}

func TestRewriteMultimodalImagesAsText(t *testing.T) {
	body := []byte(`{"model":"text-only","messages":[{"role":"user","content":[` +
		`{"type":"image_url","image_url":{"url":"https://example.com/a.png"}},` +
		`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"YWJj"}}]}]}`)

	root, images, err := imageBridgeInput(body)
	require.NoError(t, err)
	require.Equal(t, []string{
		"https://example.com/a.png",
		"data:image/png;base64,YWJj",
	}, images)

	rewritten, err := rewriteImagesAsText(root, []string{"first", "second"})
	require.NoError(t, err)
	require.False(t, requestBodyHasImageInput(rewritten))
	require.Contains(t, string(rewritten), "Image 1 description: first")
	require.Contains(t, string(rewritten), "Image 2 description: second")

	_, _, err = imageBridgeInput([]byte(
		`{"input":[{"type":"input_image","file_id":"file-123"}]}`,
	))
	require.ErrorContains(t, err, "URL or base64")
}
