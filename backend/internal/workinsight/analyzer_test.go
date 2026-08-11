package workinsight

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAnalyzerModelsUsesMappingAndOAuthDefaults(t *testing.T) {
	mapped := &service.Account{Credentials: map[string]any{"model_mapping": map[string]any{"alias-b": "upstream-b", "wild-*": "upstream", "alias-a": "upstream-a"}}}
	require.Equal(t, []string{"alias-a", "alias-b"}, analyzerModels(mapped))

	oauth := &service.Account{Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{}}
	require.Contains(t, analyzerModels(oauth), "gpt-5.5")
}

func TestAnalyzerProbeMessageSanitizesFailures(t *testing.T) {
	require.Equal(t, "分析账号鉴权失败（HTTP 401）", analyzerProbeMessage(errors.New("analyzer_http_401")))
	require.Equal(t, "分析节点连接失败", analyzerProbeMessage(errors.New("dial tcp 10.0.0.1: secret failure")))
}

func TestProbeRequestsACompleteAnalysisResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.NotContains(t, body.Messages[1].Content, "仅返回任务类型")
		require.Contains(t, body.Messages[0].Content, `evidence_level 只能是 "explicit" 或 "unknown"`)
		writeAnalyzerResult(w, "整理一份简短会议纪要。")
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.AnalyzerSource, cfg.AnalyzerBaseURL, cfg.AnalyzerToken, cfg.AnalyzerModel = "custom", server.URL, "token", "model"
	result := (&Service{}).Probe(context.Background(), cfg)
	require.True(t, result.OK, result.Message)
}

func TestBuildAnalysisChunksDeduplicatesAndHonorsBudget(t *testing.T) {
	chunks := buildAnalysisChunks([]analysisInput{
		{ID: 1, Text: "重复 历史", EstimatedTokens: 3},
		{ID: 2, Text: "重复   历史", EstimatedTokens: 3},
		{ID: 3, Text: strings.Repeat("新", 30), EstimatedTokens: 10},
	}, 8)
	require.Len(t, chunks, 2)
	require.Equal(t, int64(2), chunks[0][0].ID, "the newest duplicate is retained")
	for _, chunk := range chunks {
		require.LessOrEqual(t, chunkTokens(chunk), 8)
	}
}

func TestAnalyzeChunkRepairsInvalidJSONOnce(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "not-json"}}}, "usage": map[string]int{"prompt_tokens": 3, "completion_tokens": 1}})
			return
		}
		writeAnalyzerResult(w, "修复结构化输出。")
	}))
	defer server.Close()

	_, input, output, count, err := (&Service{}).analyzeChunkResilient(context.Background(), analysisEndpoint{baseURL: server.URL, token: "token-canary", model: "model-canary", timeoutSeconds: 2}, "", []analysisInput{{ID: 1, Text: "canary", EstimatedTokens: 2}}, false)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Equal(t, int64(8), input)
	require.Equal(t, int64(3), output)
}

func TestAnalyzeChunkSplitsContextFailureOnce(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, `{"error":"context_length_exceeded"}`, http.StatusRequestEntityTooLarge)
			return
		}
		writeAnalyzerResult(w, "拆分后完成分析。")
	}))
	defer server.Close()

	result, _, _, count, err := (&Service{}).analyzeChunkResilient(context.Background(), analysisEndpoint{baseURL: server.URL, token: "token-canary", model: "model-canary", timeoutSeconds: 2}, "", []analysisInput{{ID: 1, Text: "first", EstimatedTokens: 2}, {ID: 2, Text: "second", EstimatedTokens: 2}}, true)
	require.NoError(t, err)
	require.Equal(t, 3, count)
	require.Equal(t, []string{"其他"}, result.TaskCategories)
}

func writeAnalyzerResult(w http.ResponseWriter, summary string) {
	content, _ := json.Marshal(BatchResult{WorkSummary: summary, TaskCategories: []string{"其他"}, EvidenceLevel: "unknown"})
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(content)}}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 2}})
}

func TestAnalyzeChunkValidatesStructuredResultAndEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer token-canary", r.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.NotContains(t, body, "response_format", "OpenAI-compatible analyzers may not implement JSON mode")
		require.NotContains(t, body, "temperature", "some compatible models only accept their default temperature")
		content := `{"work_summary":"排查 sub2api 网关问题并补充测试。","task_categories":["问题排查","测试用例"],"explicit_projects":["sub2api"],"explicit_modules":["网关"],"change_types":["Bug 修复"],"business_topics":["路由"],"representative_items":[{"source_sample_ids":[11],"summary":"排查网关路由问题。","task_categories":["问题排查"],"explicit_projects":["sub2api"],"explicit_modules":["网关"]},{"source_sample_ids":[11],"summary":"请排查 sub2api 的网关路由问题","task_categories":["问题排查"],"explicit_projects":["sub2api"],"explicit_modules":["网关"]},{"source_sample_ids":[999],"summary":"虚构样本。","task_categories":["问题排查"],"explicit_projects":[],"explicit_modules":[]}],"evidence_level":"explicit"}`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}, "usage": map[string]int{"prompt_tokens": 20, "completion_tokens": 10}})
	}))
	defer server.Close()

	result, input, output, err := (&Service{}).analyzeChunk(context.Background(), analysisEndpoint{baseURL: server.URL, token: "token-canary", model: "model-canary", timeoutSeconds: 2}, "", []analysisInput{{ID: 11, Text: "请排查 sub2api 的网关路由问题", EstimatedTokens: 10}})
	require.NoError(t, err)
	require.Equal(t, int64(20), input)
	require.Equal(t, int64(10), output)
	require.Equal(t, []string{"sub2api"}, result.ExplicitProjects)
	require.Len(t, result.RepresentativeItems, 1, "invented IDs and verbatim prompt excerpts are removed")
}

func TestValidateBatchResultRejectsUnknownCategoryAndUnsupportedProject(t *testing.T) {
	base := BatchResult{WorkSummary: "摘要", TaskCategories: []string{"自造分类"}, EvidenceLevel: "unknown"}
	require.ErrorContains(t, validateBatchResult(&base, []analysisInput{{ID: 1, Text: "canary"}}), "task category")
	base = BatchResult{WorkSummary: "摘要", TaskCategories: []string{"其他"}, ExplicitProjects: []string{"不存在项目"}, EvidenceLevel: "explicit"}
	require.ErrorContains(t, validateBatchResult(&base, []analysisInput{{ID: 1, Text: "canary"}}), "evidence")
}

func TestValidateBatchResultNormalizesUnknownEvidenceLevel(t *testing.T) {
	result := BatchResult{WorkSummary: "整理会议纪要。", TaskCategories: []string{"会议纪要"}, EvidenceLevel: "低"}
	require.NoError(t, validateBatchResult(&result, []analysisInput{{ID: 1, Text: "整理会议纪要"}}))
	require.Equal(t, "unknown", result.EvidenceLevel)
}

func TestContextLimitStopsAfterCompensatingSplit(t *testing.T) {
	err := errors.New("analyzer_context_length")
	require.Equal(t, "analyzer_context_length", analyzerErrorCode(err))
	require.False(t, analyzerRetryable(err))
}
