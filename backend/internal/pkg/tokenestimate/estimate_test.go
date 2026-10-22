package tokenestimate

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
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
		{"ASCII", `{"input":"abcdefgh"}`, 2},
		{"ten English characters", `{"input":"abcdefghij"}`, 3},
		{"five Chinese characters", `{"input":"一二三四五"}`, 3},
		{"short ASCII", `{"input":"abcde"}`, 2},
		{"Unicode", `{"input":"中文🙂"}`, 3},
		{"escaped Unicode", `{"input":"\u4e2d\u6587\ud83d\ude42"}`, 3},
		{"mixed", `{"input":"abcd中文"}`, 2},
		{"settings ignored", `{"model":"ignored","metadata":{"text":"ignored"},"max_tokens":999999,"input":"abcd"}`, 1},
		{"combined inputs", `{"system":"abcd","instructions":"efgh","input":"ijkl"}`, 5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, Count([]byte(tt.body), 300000))
		})
	}
}

func TestCountLimitBoundaryAndEarlyStop(t *testing.T) {
	for _, length := range []int{0, 99, 100, 101, 200} {
		raw, err := json.Marshal(map[string]string{"input": strings.Repeat(" the", length)})
		require.NoError(t, err)
		require.Equal(t, min(length, 101), Count(raw, 100))
	}
	for _, length := range []int{99, 100, 101} {
		raw, err := json.Marshal(map[string]string{"input": strings.Repeat("中", length)})
		require.NoError(t, err)
		require.Equal(t, min(length, 101), Count(raw, 100))
	}
}

func TestCountIncludesApproximateImageTokens(t *testing.T) {
	low := []byte(`{"input":[{"type":"input_image","detail":"low","image_url":"https://example.test/a.png"}]}`)
	high := []byte(`{"input":[{"type":"input_image","width":1024,"height":1024,"image_url":"https://example.test/a.png"}]}`)
	gemini := []byte(`{"contents":[{"parts":[{"inlineData":{"mimeType":"image/png","data":"opaque"}}]}]}`)
	require.Equal(t, 85, Count(low, 300000))
	require.Equal(t, 765, Count(high, 300000))
	require.Equal(t, 259, Count(gemini, 300000))
	for _, body := range []string{
		`{"input":[{"type":"image_url","image_url":{"url":"https://example.test/a.png","detail":"low"}}]}`,
		`{"input":[{"type":"image","source":{"type":"base64","data":"invalid"}}]}`,
		`{"input":[{"type":"input_image","width":9223372036854775807,"height":9223372036854775807}]}`,
		`{"input":{"fileData":{"mimeType":"image/png","fileUri":"https://example.test/a.png"}}}`,
	} {
		require.Equal(t, 81, Count([]byte(body), 80), body)
	}
}

func TestCountImageReadsBoundedHeaders(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 513, 1))))
	data := base64.StdEncoding.EncodeToString(buf.Bytes())
	for _, part := range []any{
		map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + data, "width": 1, "height": 1},
		map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64," + data}},
		map[string]any{"type": "image", "source": map[string]string{"type": "base64", "data": data}},
	} {
		raw, err := json.Marshal(map[string]any{"input": []any{part}})
		require.NoError(t, err)
		require.Equal(t, 425, Count(raw, 300000), "two image tiles; embedded dimensions override client hints")
	}
}

func TestCountIncludesHistoryToolsAndResultsAcrossProtocols(t *testing.T) {
	large := strings.Repeat(" the", 101)
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

func TestCountDoesNotTreatMediaBytesAsText(t *testing.T) {
	large := strings.Repeat("A", 2000)
	for _, tt := range []struct {
		part        any
		imageTokens int
	}{
		{map[string]string{"type": "input_image", "image_url": "data:image/png;base64," + large}, 765},
		{map[string]any{"type": "image", "source": map[string]string{"data": large}}, 765},
		{map[string]any{"type": "input_file", "file_data": large}, 0},
		{map[string]any{"type": "input_audio", "input_audio": map[string]string{"data": large}}, 0},
		{map[string]any{"inlineData": map[string]string{"mimeType": "image/png", "data": large}}, 258},
		{map[string]any{"file_data": map[string]string{"file_uri": large}}, 0},
		{map[string]string{"type": "reasoning", "encrypted_content": large, "signature": large}, 0},
	} {
		body := map[string]any{"input": []any{tt.part}}
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		before := Count(raw, 300000)
		require.GreaterOrEqual(t, before, tt.imageTokens)
		require.Less(t, before, tt.imageTokens+20, "media bytes must not contribute to text count")
		body["instructions"] = "hello"
		raw, err = json.Marshal(body)
		require.NoError(t, err)
		require.Greater(t, Count(raw, 300000), before, "text adjacent to media must count")
		body["instructions"] = strings.Repeat(" the", 101)
		raw, err = json.Marshal(body)
		require.NoError(t, err)
		require.Equal(t, 101, Count(raw, 100))
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
