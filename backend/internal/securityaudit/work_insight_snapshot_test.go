package securityaudit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractWorkInsightSnapshot_WhitelistsRequestRoles(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		body     string
	}{
		{"chat", "openai_chat_completions", `{"messages":[{"role":"system","content":"system keep"},{"role":"developer","content":"developer keep"},{"role":"user","content":"user keep TOKEN=abcd1234"},{"role":"assistant","content":"assistant drop"},{"role":"tool","content":"tool drop"}]}`},
		{"responses", "openai_responses", `{"instructions":"system keep","input":[{"role":"user","content":[{"type":"input_text","text":"user keep"}]},{"role":"assistant","content":[{"type":"output_text","text":"assistant drop"}]},{"type":"function_call_output","output":"tool drop"}]}`},
		{"anthropic", "anthropic_messages", `{"system":"system keep","messages":[{"role":"user","content":"user keep"},{"role":"assistant","content":"assistant drop"},{"role":"tool","content":"tool drop"}]}`},
		{"gemini", "gemini", `{"systemInstruction":{"parts":[{"text":"system keep"}]},"contents":[{"role":"user","parts":[{"text":"user keep"}]},{"role":"model","parts":[{"text":"assistant drop"}]}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := ExtractWorkInsightSnapshot(Request{Protocol: tt.protocol, Body: []byte(tt.body)}, 16000)
			require.NoError(t, err)
			require.Contains(t, snapshot.Text, "user keep")
			require.Contains(t, snapshot.Text, "system keep")
			require.NotContains(t, snapshot.Text, "assistant drop")
			require.NotContains(t, snapshot.Text, "tool drop")
			require.NotContains(t, strings.ToLower(snapshot.Text), "abcd1234")
			require.NotEmpty(t, snapshot.Segments)
			require.Len(t, snapshot.PromptHash, 64)
		})
	}
}

func TestExtractWorkInsightSnapshot_RejectsMediaAndAssistantOnly(t *testing.T) {
	_, err := ExtractWorkInsightSnapshot(Request{Protocol: "images", Body: []byte(`{"prompt":"image prompt"}`)}, 100)
	require.ErrorIs(t, err, ErrNoPromptText)
	_, err = ExtractWorkInsightSnapshot(Request{Protocol: "openai_chat", Body: []byte(`{"messages":[{"role":"assistant","content":"drop"}]}`)}, 100)
	require.ErrorIs(t, err, ErrNoPromptText)
}

func TestExtractWorkInsightSnapshot_RedactsCredentialShapes(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJjYW5hcnkifQ.signature_canary"
	base64 := strings.Repeat("QUJD", 20)
	longToken := strings.Repeat("aB1_", 20)
	content := "URL https://example.test/cb?access_token=query-canary&x=1\nJSON {\"refresh_token\":\"json-canary-secret\"}\nJWT " + jwt +
		"\n-----BEGIN PRIVATE KEY-----\nprivate-canary\n-----END PRIVATE KEY-----\nBASE64 " + base64 + "\nTOKEN " + longToken
	body, err := json.Marshal(map[string]any{"messages": []any{map[string]any{"role": "user", "content": content}}})
	require.NoError(t, err)
	snapshot, err := ExtractWorkInsightSnapshot(Request{Protocol: "openai_chat", Body: body}, 16000)
	require.NoError(t, err)
	for _, secret := range []string{"query-canary", "json-canary-secret", jwt, "private-canary", base64, longToken} {
		require.NotContains(t, snapshot.Text, secret)
	}
	require.Contains(t, snapshot.Text, "access_token=***")
	require.Contains(t, snapshot.Text, "***JWT***")
	require.Contains(t, snapshot.Text, "***PRIVATE_BLOCK***")
	require.Contains(t, snapshot.Text, "***BASE64***")
	require.Contains(t, snapshot.Text, "***TOKEN***")
}
