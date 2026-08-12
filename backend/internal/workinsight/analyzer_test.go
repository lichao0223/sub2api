package workinsight

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type analyzerAccountRepoStub struct {
	service.AccountRepository
	accounts []service.Account
	account  *service.Account
}

func (r analyzerAccountRepoStub) ListActive(context.Context) ([]service.Account, error) {
	return r.accounts, nil
}
func (r analyzerAccountRepoStub) GetByID(context.Context, int64) (*service.Account, error) {
	return r.account, nil
}

func TestAnalyzerModelsUsesMappingAndOAuthDefaults(t *testing.T) {
	mapped := &service.Account{Credentials: map[string]any{"model_mapping": map[string]any{"alias-b": "upstream-b", "wild-*": "upstream", "alias-a": "upstream-a"}}}
	require.Equal(t, []string{"alias-a", "alias-b"}, analyzerModels(mapped))

	oauth := &service.Account{Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{}}
	require.Contains(t, analyzerModels(oauth), "gpt-5.5")
}

func TestAnalyzerAccountsOnlyReturnsOpenAIPlatform(t *testing.T) {
	credentials := map[string]any{"base_url": "https://example.com/v1", "api_key": "token", "models": []any{"model"}}
	serviceUnderTest := &Service{accounts: analyzerAccountRepoStub{accounts: []service.Account{
		{ID: 1, Name: "OpenAI", Platform: service.PlatformOpenAI, Credentials: credentials},
		{ID: 2, Name: "Anthropic", Platform: service.PlatformAnthropic, Credentials: credentials},
	}}}

	accounts, err := serviceUnderTest.AnalyzerAccounts(context.Background())
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, int64(1), accounts[0].ID)
}

func TestResolveAnalyzerRejectsNonOpenAIManagedAccount(t *testing.T) {
	account := &service.Account{ID: 2, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	serviceUnderTest := &Service{accounts: analyzerAccountRepoStub{account: account}}
	cfg := storedConfig{Config: DefaultConfig()}
	cfg.AnalyzerSource, cfg.AnalyzerAccountID, cfg.AnalyzerModel = "account", account.ID, "model"

	_, err := serviceUnderTest.resolveAnalyzer(context.Background(), cfg)
	require.ErrorContains(t, err, "must use openai platform")
}

func TestAnalyzerProbeMessageSanitizesFailures(t *testing.T) {
	require.Equal(t, "分析账号鉴权失败（HTTP 401）", analyzerProbeMessage(errors.New("analyzer_http_401")))
	require.Equal(t, "模型无法处理当前请求格式（HTTP 422）", analyzerProbeMessage(errors.New("analyzer_http_422")))
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
	require.Len(t, chunks, 3)
	require.Equal(t, int64(2), chunks[0][0].ID, "the newest duplicate is retained")
	var retained strings.Builder
	for _, chunk := range chunks {
		require.LessOrEqual(t, chunkTokens(chunk), 8)
		for _, sample := range chunk {
			if sample.ID == 3 {
				_, _ = retained.WriteString(sample.Text)
			}
		}
	}
	require.Equal(t, strings.Repeat("新", 30), retained.String(), "oversized input is split without truncation")
}

func TestAnalyzeChunkRepairsInvalidJSONOnce(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		if call == 1 {
			require.Equal(t, map[string]any{"type": "json_object"}, payload["response_format"])
			require.Equal(t, false, payload["enable_thinking"])
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "not-json"}}}, "usage": map[string]int{"prompt_tokens": 3, "completion_tokens": 1}})
			return
		}
		require.NotContains(t, payload, "response_format")
		require.Equal(t, false, payload["enable_thinking"])
		writeAnalyzerResult(w, "修复结构化输出。")
	}))
	defer server.Close()

	_, input, output, count, err := (&Service{}).analyzeChunkResilient(context.Background(), analysisEndpoint{baseURL: server.URL, token: "token-canary", model: "model-canary", timeoutSeconds: 2}, "", []analysisInput{{ID: 1, Text: "canary", EstimatedTokens: 2}}, false)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Equal(t, int64(8), input)
	require.Equal(t, int64(3), output)
}

func TestAnalyzeChunkFallsBackWhenJSONModeIsUnsupported(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		if calls.Add(1) == 1 {
			require.Contains(t, payload, "response_format")
			http.Error(w, `{"error":"response_format unsupported"}`, http.StatusBadRequest)
			return
		}
		require.NotContains(t, payload, "response_format")
		require.Equal(t, false, payload["enable_thinking"])
		writeAnalyzerResult(w, "降级后完成分析。")
	}))
	defer server.Close()

	result, _, _, count, err := (&Service{}).analyzeChunkResilient(context.Background(), analysisEndpoint{baseURL: server.URL, token: "token-canary", model: "model-canary", timeoutSeconds: 2}, "", []analysisInput{{ID: 1, Text: "canary", EstimatedTokens: 2}}, false)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Equal(t, "降级后完成分析。", result.WorkSummary)
}

func TestAnalyzeChunkRemovesThinkingOptionWhenNodeRejectsIt(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		call := calls.Add(1)
		if call < 3 {
			require.Equal(t, false, payload["enable_thinking"])
			http.Error(w, `{"error":"unknown field enable_thinking"}`, http.StatusUnprocessableEntity)
			return
		}
		require.NotContains(t, payload, "enable_thinking")
		writeAnalyzerResult(w, "移除扩展参数后完成分析。")
	}))
	defer server.Close()

	result, _, _, count, err := (&Service{}).analyzeChunkResilient(context.Background(), analysisEndpoint{baseURL: server.URL, token: "token-canary", model: "model-canary", timeoutSeconds: 2}, "", []analysisInput{{ID: 1, Text: "canary", EstimatedTokens: 2}}, false)
	require.NoError(t, err)
	require.Equal(t, 3, count)
	require.Equal(t, "移除扩展参数后完成分析。", result.WorkSummary)
}

func TestDecodeAnalyzerResultAcceptsWrappedJSON(t *testing.T) {
	content := "模型说明文字\n```json\n{\"work_summary\":\"整理会议纪要。\",\"task_categories\":[\"会议纪要\"],\"explicit_projects\":[],\"explicit_modules\":[],\"change_types\":[],\"business_topics\":[],\"representative_items\":[],\"evidence_level\":\"explicit\"}\n```"
	result, err := decodeAnalyzerResult(content)
	require.NoError(t, err)
	require.Equal(t, "整理会议纪要。", result.WorkSummary)
}

func TestParseAnalyzerResponseAcceptsReasoningAndArrayContent(t *testing.T) {
	valid := `{"work_summary":"整理会议纪要。","task_categories":["会议纪要"],"explicit_projects":[],"explicit_modules":[],"change_types":[],"business_topics":[],"representative_items":[],"evidence_level":"explicit"}`
	samples := []analysisInput{{ID: 1, Text: "整理会议纪要"}}

	reasoningOnly := []byte(`{"choices":[{"message":{"content":"","reasoning_content":` + strconv.Quote("分析过程\n"+valid) + `},"finish_reason":"stop"}]}`)
	result, _, _, err := parseAnalyzerResponse(reasoningOnly, samples)
	require.NoError(t, err)
	require.Equal(t, "整理会议纪要。", result.WorkSummary)

	arrayContent := []byte(`{"choices":[{"message":{"content":[{"type":"text","text":` + strconv.Quote(valid) + `}]},"finish_reason":"stop"}]}`)
	result, _, _, err = parseAnalyzerResponse(arrayContent, samples)
	require.NoError(t, err)
	require.Equal(t, "整理会议纪要。", result.WorkSummary)
}

func TestParseAnalyzerResponseAcceptsSummaryArrayAndStringSampleIDs(t *testing.T) {
	content := `{"work_summary":["整理简短的会议纪要"],"task_categories":["会议纪要"],"explicit_projects":[],"explicit_modules":[],"change_types":[],"business_topics":[],"representative_items":[{"source_sample_ids":["1"],"summary":"整理简短会议纪要","task_categories":["会议纪要"],"explicit_projects":[],"explicit_modules":[]}],"evidence_level":"explicit"}`
	raw := []byte(`{"choices":[{"message":{"content":null,"reasoning":` + strconv.Quote(content) + `},"finish_reason":"stop"}],"usage":{"prompt_tokens":342,"completion_tokens":146}}`)

	result, input, output, err := parseAnalyzerResponse(raw, []analysisInput{{ID: 1, Text: "连接检测样本：整理一份简短会议纪要。"}})
	require.NoError(t, err)
	require.Equal(t, "- 整理简短的会议纪要", result.WorkSummary)
	require.Equal(t, int64(342), input)
	require.Equal(t, int64(146), output)
	require.Equal(t, []int64{1}, result.RepresentativeItems[0].SourceSampleIDs)
}

func TestParseAnalyzerResponseReportsTruncatedOutput(t *testing.T) {
	_, _, _, err := parseAnalyzerResponse([]byte(`{"choices":[{"message":{"content":"{\\\"work_summary\\\":"},"finish_reason":"length"}]}`), nil)
	require.EqualError(t, err, "analyzer output truncated")
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

func TestAnalyzeChunkUsesInternalGatewayForManagedAPIKeyAccount(t *testing.T) {
	var directCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		directCalls.Add(1)
	}))
	defer server.Close()

	account := &service.Account{Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}
	_, _, _, err := (&Service{}).analyzeChunk(context.Background(), analysisEndpoint{
		baseURL: server.URL, token: "token-canary", model: "model-canary", timeoutSeconds: 2, account: account,
	}, "", []analysisInput{{ID: 1, Text: "canary", EstimatedTokens: 2}})
	require.EqualError(t, err, "analyzer gateway unavailable")
	require.Zero(t, directCalls.Load(), "managed accounts must not bypass the internal gateway")
}

func TestValidateBatchResultRejectsUnknownCategoryAndFiltersUnsupportedProject(t *testing.T) {
	base := BatchResult{WorkSummary: "摘要", TaskCategories: []string{"自造分类"}, EvidenceLevel: "unknown"}
	require.ErrorContains(t, validateBatchResult(&base, []analysisInput{{ID: 1, Text: "canary"}}), "task category")
	base = BatchResult{WorkSummary: "摘要", TaskCategories: []string{"其他"}, ExplicitProjects: []string{"不存在项目"}, EvidenceLevel: "explicit"}
	require.NoError(t, validateBatchResult(&base, []analysisInput{{ID: 1, Text: "canary"}}))
	require.Empty(t, base.ExplicitProjects)
}

func TestValidateBatchResultDoesNotTurnAQueryIntoTroubleshooting(t *testing.T) {
	samples := []analysisInput{{ID: 1, Text: "查询南昌天气"}}
	wrong := BatchResult{WorkSummary: "- 排查天气查询接口调用逻辑", TaskCategories: []string{"问题排查"}, EvidenceLevel: "explicit"}
	require.ErrorContains(t, validateBatchResult(&wrong, samples), "inferred troubleshooting")

	accurate := BatchResult{
		WorkSummary: "- 查询南昌天气", TaskCategories: []string{"其他"}, EvidenceLevel: "explicit",
		RepresentativeItems: []RepresentativeItem{{SourceSampleIDs: []int64{1}, Summary: "查询南昌天气", TaskCategories: []string{"其他"}}},
	}
	require.NoError(t, validateBatchResult(&accurate, samples))
	require.Equal(t, "查询南昌天气", accurate.RepresentativeItems[0].Summary)
}

func TestMergeBatchResultsDeduplicatesRepresentativeItems(t *testing.T) {
	item := RepresentativeItem{SourceSampleIDs: []int64{1}, Summary: "整理安全规范流程", TaskCategories: []string{"文档写作"}}
	merged := mergeBatchResults([]BatchResult{{RepresentativeItems: []RepresentativeItem{item}}, {RepresentativeItems: []RepresentativeItem{{SourceSampleIDs: []int64{2}, Summary: "整理  安全规范流程", TaskCategories: []string{"其他"}}}}})
	require.Len(t, merged.RepresentativeItems, 1)
}

func TestValidateBatchResultNormalizesUnknownEvidenceLevel(t *testing.T) {
	result := BatchResult{WorkSummary: "整理会议纪要。", TaskCategories: []string{"会议纪要"}, EvidenceLevel: "低"}
	require.NoError(t, validateBatchResult(&result, []analysisInput{{ID: 1, Text: "整理会议纪要"}}))
	require.Equal(t, "unknown", result.EvidenceLevel)
}

func TestValidateBatchResultTruncatesLongSummary(t *testing.T) {
	result := BatchResult{WorkSummary: strings.Repeat("长", 301), TaskCategories: []string{"其他"}, EvidenceLevel: "explicit"}
	require.NoError(t, validateBatchResult(&result, nil))
	require.Len(t, []rune(result.WorkSummary), 300)
}

func TestMergeBatchResultsFormatsHumanReadableWorkList(t *testing.T) {
	result := mergeBatchResults([]BatchResult{
		{WorkSummary: "- 新增功能：支持日志分页\n- 修复问题：停止任务后仍写回结果"},
		{WorkSummary: "修复问题：停止任务后仍写回结果；完成事项：补充回归测试"},
	})
	require.Equal(t, "- 新增功能：支持日志分页\n- 修复问题：停止任务后仍写回结果\n- 完成事项：补充回归测试", result.WorkSummary)
}

func TestMergeBatchResultsKeepsRepresentativeItemsInDailySummary(t *testing.T) {
	result := mergeBatchResults([]BatchResult{
		{WorkSummary: "- 整理安全规范流程", RepresentativeItems: []RepresentativeItem{{Summary: "整理安全规范流程"}, {Summary: "排查账户锁定问题"}}},
	})
	require.Equal(t, "- 整理安全规范流程\n- 排查账户锁定问题", result.WorkSummary)
}

func TestValidateBatchResultLimitsDailySummaryToFiveThemes(t *testing.T) {
	result := BatchResult{WorkSummary: "- 一\n- 二\n- 三\n- 四\n- 五\n- 六", TaskCategories: []string{"其他"}}
	require.ErrorContains(t, validateBatchResult(&result, []analysisInput{{ID: 1, Text: "一二三四五六"}}), "too many work summary items")
}

func TestContextLimitStopsAfterCompensatingSplit(t *testing.T) {
	err := errors.New("analyzer_context_length")
	require.Equal(t, "analyzer_context_length", analyzerErrorCode(err))
	require.False(t, analyzerRetryable(err))
}

func TestAnalyzerValidationErrorsAreNotReportedAsUnavailable(t *testing.T) {
	require.Equal(t, "analyzer_invalid_result", analyzerErrorCode(errors.New("too many result values")))
	require.Equal(t, "analyzer_invalid_result", analyzerErrorCode(errors.New("invalid result value")))
}
