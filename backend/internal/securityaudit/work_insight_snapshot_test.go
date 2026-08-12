package securityaudit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractWorkInsightSnapshot_KeepsOnlyUserInput(t *testing.T) {
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
			require.NotContains(t, snapshot.Text, "system keep")
			require.NotContains(t, snapshot.Text, "developer keep")
			require.NotContains(t, snapshot.Text, "assistant drop")
			require.NotContains(t, snapshot.Text, "tool drop")
			require.NotContains(t, strings.ToLower(snapshot.Text), "abcd1234")
			require.NotEmpty(t, snapshot.Segments)
			require.Len(t, snapshot.PromptHash, 64)
		})
	}
}

func TestExtractWorkInsightSnapshot_SystemPromptDoesNotConsumeUserBudget(t *testing.T) {
	body, err := json.Marshal(map[string]any{"messages": []any{
		map[string]any{"role": "system", "content": strings.Repeat("system ", 100)},
		map[string]any{"role": "user", "content": "用户真实任务"},
	}})
	require.NoError(t, err)
	snapshot, err := ExtractWorkInsightSnapshot(Request{Protocol: "openai_chat", Body: body}, 20)
	require.NoError(t, err)
	require.Equal(t, "用户真实任务", snapshot.Text)
	require.Equal(t, snapshot.PromptChars, snapshot.AnalyzedChars)
}

func TestExtractLatestWorkInsightSnapshotUsesNewestUserInput(t *testing.T) {
	snapshot, err := ExtractLatestWorkInsightSnapshot(Request{Protocol: "openai_chat", Body: []byte(`{"messages":[{"role":"user","content":"历史对话"},{"role":"assistant","content":"完成"},{"role":"user","content":"今天的新任务"}]}`)}, 16000)
	require.NoError(t, err)
	require.Equal(t, "今天的新任务", snapshot.Text)
	require.Len(t, snapshot.Segments, 1)
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
	require.False(t, snapshot.Truncated, "redaction becoming shorter is not truncation")
}

func TestExtractWorkInsightSnapshot_ReportsActualTruncation(t *testing.T) {
	body, err := json.Marshal(map[string]any{"input": []any{
		map[string]any{"role": "user", "content": strings.Repeat("a", 30)},
	}})
	require.NoError(t, err)
	snapshot, err := ExtractWorkInsightSnapshot(Request{Protocol: "openai_responses", Body: body}, 20)
	require.NoError(t, err)
	require.True(t, snapshot.Truncated)
}

func TestWorkInsightSnapshotRetainsOnlyNewConversationSuffix(t *testing.T) {
	previous, err := ExtractWorkInsightSnapshot(Request{Protocol: "openai_chat", Body: []byte(`{"messages":[{"role":"user","content":"昨天任务"},{"role":"assistant","content":"完成"},{"role":"user","content":"昨天补充"}]}`)}, 2<<20)
	require.NoError(t, err)
	current, err := ExtractWorkInsightSnapshot(Request{Protocol: "openai_chat", Body: []byte(`{"messages":[{"role":"user","content":"昨天任务"},{"role":"assistant","content":"完成"},{"role":"user","content":"昨天补充"},{"role":"assistant","content":"完成"},{"role":"user","content":"今天新增"}]}`)}, 2<<20)
	require.NoError(t, err)

	require.True(t, current.RetainAfterPrefix(previous.MessageCount, previous.PromptHash))
	require.Equal(t, []string{"今天新增"}, current.Segments)
	require.Equal(t, "今天新增", current.Text)
	require.Equal(t, len([]rune("今天新增")), current.PromptChars)

	different, err := ExtractWorkInsightSnapshot(Request{Protocol: "openai_chat", Body: []byte(`{"messages":[{"role":"user","content":"长度相同"},{"role":"user","content":"今天新增"}]}`)}, 2<<20)
	require.NoError(t, err)
	require.False(t, different.RetainAfterPrefix(previous.MessageCount, previous.PromptHash), "equal counts or lengths must not merge different content")
}
