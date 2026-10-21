package tokenestimate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCount(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want int
	}{
		{"ASCII", `{"input":"abcdefgh"}`, 3},
		{"round up", `{"input":"abcde"}`, 2},
		{"Unicode", `{"input":"中文🙂"}`, 3},
		{"escaped Unicode", `{"input":"\u4e2d\u6587\ud83d\ude42"}`, 3},
		{"mixed", `{"input":"abcd中文"}`, 3},
		{"settings ignored", `{"model":"ignored","metadata":{"text":"ignored"},"max_tokens":999999,"input":"abcd"}`, 2},
		{"combined inputs", `{"system":"abcd","instructions":"efgh","input":"ijkl"}`, 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, Count([]byte(tt.body), 300000))
		})
	}
}

func TestCountLimitBoundaryAndEarlyStop(t *testing.T) {
	for _, length := range []int{999999, 1000000, 1000001, 2000000} {
		raw, err := json.Marshal(map[string]string{"input": strings.Repeat("a", length)})
		require.NoError(t, err)
		want := min((length*3+9)/10, 300001)
		require.Equal(t, want, Count(raw, 300000))
	}
	raw, err := json.Marshal(map[string]string{"input": strings.Repeat("中", 500001)})
	require.NoError(t, err)
	require.Equal(t, 300001, Count(raw, 300000))
}

func TestCountIncludesApproximateImageTokens(t *testing.T) {
	low := []byte(`{"input":[{"type":"input_image","detail":"low","image_url":"https://example.test/a.png"}]}`)
	high := []byte(`{"input":[{"type":"input_image","width":1024,"height":1024,"image_url":"https://example.test/a.png"}]}`)
	gemini := []byte(`{"contents":[{"parts":[{"inlineData":{"mimeType":"image/png","data":"opaque"}}]}]}`)
	require.Equal(t, 85, Count(low, 300000))
	require.Equal(t, 765, Count(high, 300000))
	require.Equal(t, 260, Count(gemini, 300000))
}

func TestCountIncludesHistoryToolsAndResultsAcrossProtocols(t *testing.T) {
	large := strings.Repeat("x", 401)
	for _, body := range []any{
		map[string]any{"messages": []any{map[string]string{"role": "system", "content": large}}},
		map[string]any{"messages": []any{map[string]string{"role": "assistant", "content": large}, map[string]string{"role": "user", "content": "hi"}}},
		map[string]any{"messages": []any{map[string]string{"role": "tool", "content": large}}},
		map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{map[string]string{"type": "tool_result", "content": large}}}}},
		map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "document", "source": map[string]string{"type": "text", "data": large}}}}}},
		map[string]any{"input": []any{map[string]string{"type": "function_call", "arguments": large}}},
		map[string]any{"input": []any{map[string]string{"type": "function_call_output", "output": large}}},
		map[string]any{"tools": []any{map[string]string{"description": large}}},
		map[string]any{"tools": []any{map[string]any{"parameters": map[string]any{"properties": map[string]any{large: map[string]string{"type": "image"}}}}}},
		map[string]any{"systemInstruction": map[string]any{"parts": []any{map[string]string{"text": large}}}},
		map[string]any{"contents": []any{map[string]any{"parts": []any{map[string]any{"functionResponse": map[string]any{"response": map[string]string{"type": "image", "data": large}}}}}}},
		map[string]any{"response": map[string]any{"input": large}},
		map[string]any{"response": map[string]any{"tools": []any{map[string]any{"parameters": map[string]string{"type": "image", "description": large}}}}},
		map[string]any{"input": []string{large}},
		map[string]any{"questions": map[string]any{"q": map[string]string{"instructions": large}}},
	} {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		require.Equal(t, 101, Count(raw, 100), string(raw[:min(len(raw), 100)]))
	}
}

func TestCountSkipsMediaButCountsAdjacentText(t *testing.T) {
	large := strings.Repeat("A", 2000)
	for _, part := range []any{
		map[string]string{"type": "input_image", "image_url": "data:image/png;base64," + large},
		map[string]any{"type": "image", "source": map[string]string{"data": large}},
		map[string]any{"type": "input_file", "file_data": large},
		map[string]any{"type": "input_audio", "input_audio": map[string]string{"data": large}},
		map[string]any{"inlineData": map[string]string{"mimeType": "image/png", "data": large}},
		map[string]any{"file_data": map[string]string{"file_uri": large}},
		map[string]string{"type": "reasoning", "encrypted_content": large, "signature": large},
	} {
		body := map[string]any{"input": []any{map[string]any{"content": []any{part, map[string]string{"type": "input_text", "text": "hello"}}}}}
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		if strings.Contains(string(raw), `"input_image"`) || strings.Contains(string(raw), `"type":"image"`) || strings.Contains(string(raw), "image/png") {
			require.GreaterOrEqual(t, Count(raw, 100), 85)
		} else {
			require.Less(t, Count(raw, 100), 100)
		}
		body["instructions"] = strings.Repeat("x", 401)
		raw, err = json.Marshal(body)
		require.NoError(t, err)
		if strings.Contains(string(raw), "image/png") || strings.Contains(string(raw), `"input_image"`) || strings.Contains(string(raw), `"type":"image"`) {
			require.GreaterOrEqual(t, Count(raw, 100), 100)
		} else {
			require.Equal(t, 101, Count(raw, 100))
		}
	}
}

func TestCountBoundsNestedJSONTraversal(t *testing.T) {
	body := `{"input":` + strings.Repeat(`[`, 100) + `"` + strings.Repeat("x", 1000) + `"` + strings.Repeat(`]`, 100) + `}`
	require.Equal(t, 101, Count([]byte(body), 100))
}

func BenchmarkCountLargeInput(b *testing.B) {
	raw, err := json.Marshal(map[string]string{"input": strings.Repeat("input text ", 1000000)})
	require.NoError(b, err)
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		Count(raw, 300000)
	}
}
