package workinsight

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	openaiapi "github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type analysisInput struct {
	ID              int64
	Text            string
	EstimatedTokens int
}

type analysisEndpoint struct {
	baseURL, token, model string
	timeoutSeconds        int
	account               *service.Account
}

type analyzerFailure struct {
	cause  error
	detail string
}

func (e *analyzerFailure) Error() string { return e.cause.Error() }
func (e *analyzerFailure) Unwrap() error { return e.cause }

func (s *Service) openAIAnalyzerAccount(ctx context.Context, id int64) (*service.Account, error) {
	if s.accounts == nil {
		return nil, errors.New("account repository unavailable")
	}
	account, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account == nil || !account.IsActive() {
		return nil, errors.New("analyzer account unavailable")
	}
	if account.Platform != service.PlatformOpenAI {
		return nil, errors.New("analyzer account must use openai platform")
	}
	return account, nil
}

func (s *Service) resolveAnalyzer(ctx context.Context, cfg storedConfig) (analysisEndpoint, error) {
	endpoint := analysisEndpoint{baseURL: cfg.AnalyzerBaseURL, model: cfg.AnalyzerModel, timeoutSeconds: cfg.AnalysisTimeoutSeconds}
	if cfg.AnalyzerSource == "custom" {
		endpoint.token = cfg.AnalyzerToken
		if endpoint.token == "" {
			token, err := s.AnalyzerToken()
			if err != nil {
				return endpoint, err
			}
			endpoint.token = token
		}
	} else {
		account, err := s.openAIAnalyzerAccount(ctx, cfg.AnalyzerAccountID)
		if err != nil {
			return endpoint, err
		}
		endpoint.account = account
		endpoint.baseURL = strings.TrimSpace(account.GetCredential("base_url"))
		endpoint.token = strings.TrimSpace(account.GetCredential("api_key"))
		if endpoint.baseURL == "" {
			endpoint.baseURL = account.GetOpenAIBaseURL()
		}
		if endpoint.token == "" {
			endpoint.token = account.GetOpenAIAccessToken()
		}
	}
	if _, err := securityaudit.ChatCompletionsURL(endpoint.baseURL); err != nil {
		return endpoint, err
	}
	if endpoint.token == "" || endpoint.model == "" {
		return endpoint, errors.New("analyzer credentials incomplete")
	}
	return endpoint, nil
}

func (s *Service) Probe(ctx context.Context, request Config) ProbeResult {
	started := time.Now()
	request.normalize()
	stored := storedConfig{Config: request}
	endpoint, err := s.resolveAnalyzer(ctx, stored)
	if err == nil {
		_, _, _, _, err = s.analyzeChunkResilient(ctx, endpoint, "", []analysisInput{{ID: 1, Text: "连接检测样本：整理一份简短会议纪要。", EstimatedTokens: 16}}, false)
	}
	result := ProbeResult{OK: err == nil, Status: "ok", Message: "分析节点连接正常", LatencyMS: time.Since(started).Milliseconds(), CheckedAt: time.Now().UTC()}
	if err != nil {
		logger.L().Error("work insight analyzer probe failed", zap.String("source", request.AnalyzerSource), zap.Int64("account_id", request.AnalyzerAccountID), zap.String("model", request.AnalyzerModel), zap.Error(err))
		result.Status, result.Message = "error", analyzerProbeMessage(err)
	}
	return result
}

func (s *Service) AnalyzerAccounts(ctx context.Context) ([]AnalyzerAccount, error) {
	if s.accounts == nil {
		return nil, errors.New("account repository unavailable")
	}
	accounts, err := s.accounts.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]AnalyzerAccount, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if account.Platform != service.PlatformOpenAI {
			continue
		}
		baseURL := strings.TrimSpace(account.GetCredential("base_url"))
		token := strings.TrimSpace(account.GetCredential("api_key"))
		if baseURL == "" {
			baseURL = account.GetOpenAIBaseURL()
		}
		if token == "" {
			token = account.GetOpenAIAccessToken()
		}
		if baseURL == "" || token == "" {
			continue
		}
		models := analyzerModels(account)
		result = append(result, AnalyzerAccount{ID: account.ID, Name: account.Name, Platform: account.Platform, Models: models})
	}
	return result, nil
}

func analyzerModels(account *service.Account) []string {
	models := stringSlice(account.Credentials["models"])
	for model := range account.GetModelMapping() {
		if !strings.ContainsAny(model, "*?") {
			models = append(models, model)
		}
	}
	if len(models) == 0 && account.IsOpenAIOAuth() {
		for _, model := range openaiapi.DefaultModelIDs() {
			if strings.HasPrefix(model, "gpt-") && !strings.Contains(model, "image") {
				models = append(models, model)
			}
		}
	}
	unique := make(map[string]struct{}, len(models))
	result := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" || utf8.RuneCountInString(model) > 128 {
			continue
		}
		if _, exists := unique[model]; !exists {
			unique[model] = struct{}{}
			result = append(result, model)
		}
	}
	sort.Strings(result)
	return result
}

func analyzerProbeMessage(err error) string {
	message := err.Error()
	switch {
	case strings.Contains(message, "analyzer_http_400"):
		return "模型或请求格式不受支持（HTTP 400）"
	case strings.Contains(message, "analyzer_http_422"):
		return "模型无法处理当前请求格式（HTTP 422）"
	case strings.Contains(message, "analyzer_http_401"):
		return "分析账号鉴权失败（HTTP 401）"
	case strings.Contains(message, "analyzer_http_403"):
		return "分析账号无权限（HTTP 403）"
	case strings.Contains(message, "analyzer_http_429"):
		return "分析账号已限流（HTTP 429）"
	case strings.Contains(message, "analyzer_http_404"):
		return "分析接口地址不存在（HTTP 404）"
	case strings.Contains(message, "analyzer_http_5"):
		return "分析节点服务异常"
	case strings.Contains(message, "output truncated"):
		return "模型输出过长且未返回完整结果"
	case strings.Contains(message, "response too large"):
		return "模型响应超过 256 KiB，请确认 thinking 已关闭"
	case isInvalidAnalyzerResult(err):
		return "分析节点响应格式不兼容"
	case strings.Contains(message, "credentials incomplete"):
		return "分析账号配置不完整"
	case strings.Contains(message, "must use openai platform"):
		return "请选择 OpenAI 平台的分析账号"
	default:
		return "分析节点连接失败"
	}
}

func stringSlice(value any) []string {
	var values []string
	switch raw := value.(type) {
	case []string:
		values = append(values, raw...)
	case []any:
		for _, item := range raw {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
	}
	return values
}

func buildAnalysisChunks(samples []analysisInput, budget int) [][]analysisInput {
	seen := map[string]struct{}{}
	deduped := make([]analysisInput, 0, len(samples))
	for i := len(samples) - 1; i >= 0; i-- {
		normalized := strings.Join(strings.Fields(samples[i].Text), " ")
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		deduped = append(deduped, samples[i])
	}
	slices.Reverse(deduped)
	var chunks [][]analysisInput
	for _, sample := range deduped {
		text := []rune(sample.Text)
		maxRunes := max(1, budget*3)
		for start := 0; start < len(text); start += maxRunes {
			end := min(start+maxRunes, len(text))
			part := sample
			part.Text = string(text[start:end])
			part.EstimatedTokens = max(1, (end-start+2)/3)
			if len(chunks) == 0 || chunkTokens(chunks[len(chunks)-1])+part.EstimatedTokens > budget {
				chunks = append(chunks, []analysisInput{part})
			} else {
				chunks[len(chunks)-1] = append(chunks[len(chunks)-1], part)
			}
		}
	}
	return chunks
}

func chunkTokens(chunk []analysisInput) int {
	total := 0
	for _, sample := range chunk {
		total += sample.EstimatedTokens
	}
	return total
}

func (s *Service) analyzeChunk(ctx context.Context, endpoint analysisEndpoint, previousSummary string, samples []analysisInput) (BatchResult, int64, int64, error) {
	return s.analyzeChunkWithMode(ctx, endpoint, previousSummary, samples, false, false)
}

func (s *Service) analyzeChunkWithMode(ctx context.Context, endpoint analysisEndpoint, previousSummary string, samples []analysisInput, jsonMode, disableThinking bool) (BatchResult, int64, int64, error) {
	url, err := securityaudit.ChatCompletionsURL(endpoint.baseURL)
	if err != nil {
		return BatchResult{}, 0, 0, err
	}
	active := securityaudit.ActiveEndpoint{BaseURL: endpoint.baseURL, TimeoutMS: endpoint.timeoutSeconds * 1000}
	client, err := securityaudit.NewSecureHTTPClient(active)
	if err != nil {
		return BatchResult{}, 0, 0, err
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	var input strings.Builder
	validationSamples := samples
	if previousSummary != "" {
		_, _ = input.WriteString("上一版结构化摘要：\n" + previousSummary + "\n\n")
		validationSamples = append([]analysisInput{{Text: previousSummary}}, samples...)
	}
	_, _ = input.WriteString("本批脱敏样本：\n")
	for _, sample := range samples {
		fmt.Fprintf(&input, "<sample id=%d>\n%s\n</sample>\n", sample.ID, sample.Text)
	}
	payload := map[string]any{
		"model":    endpoint.model,
		"stream":   false,
		"messages": []map[string]string{{"role": "system", "content": analyzerInstruction}, {"role": "user", "content": input.String()}},
	}
	if jsonMode {
		payload["response_format"] = map[string]string{"type": "json_object"}
	}
	if disableThinking {
		payload["enable_thinking"] = false
	}
	body, _ := json.Marshal(payload)
	if endpoint.account != nil {
		if s.openAIGateway == nil {
			return BatchResult{}, 0, 0, errors.New("analyzer gateway unavailable")
		}
		recorder := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(recorder)
		ginContext.Request = httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		ginContext.Request.Header.Set("Content-Type", "application/json")
		_, gatewayErr := s.openAIGateway.ForwardAsChatCompletions(ctx, ginContext, endpoint.account, body, "", endpoint.model)
		if gatewayErr != nil {
			logAnalyzerFailure(endpoint, "gateway", jsonMode, disableThinking, recorder.Body.Bytes(), gatewayErr)
			if recorder.Code >= 400 {
				return BatchResult{}, 0, 0, withAnalyzerDetail(fmt.Errorf("analyzer_http_%d", recorder.Code), recorder.Body.Bytes())
			}
			return BatchResult{}, 0, 0, withAnalyzerDetail(gatewayErr, recorder.Body.Bytes())
		}
		result, promptTokens, completionTokens, parseErr := parseAnalyzerResponse(recorder.Body.Bytes(), validationSamples)
		if parseErr != nil {
			logAnalyzerFailure(endpoint, "parse", jsonMode, disableThinking, recorder.Body.Bytes(), parseErr)
			parseErr = withAnalyzerDetail(parseErr, recorder.Body.Bytes())
		}
		return result, promptTokens, completionTokens, parseErr
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return BatchResult{}, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+endpoint.token)
	resp, err := client.Do(req)
	if err != nil {
		logAnalyzerFailure(endpoint, "request", jsonMode, disableThinking, nil, err)
		return BatchResult{}, 0, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024+1))
	if err != nil {
		return BatchResult{}, 0, 0, err
	}
	if len(raw) > 256*1024 {
		err := errors.New("analyzer response too large")
		logAnalyzerFailure(endpoint, "response_limit", jsonMode, disableThinking, raw, err)
		return BatchResult{}, 0, 0, withAnalyzerDetail(err, raw)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logAnalyzerFailure(endpoint, fmt.Sprintf("http_%d", resp.StatusCode), jsonMode, disableThinking, raw, fmt.Errorf("analyzer_http_%d", resp.StatusCode))
		lower := strings.ToLower(string(raw))
		if resp.StatusCode == http.StatusRequestEntityTooLarge || strings.Contains(lower, "context_length") || strings.Contains(lower, "context length") || strings.Contains(lower, "maximum context") {
			return BatchResult{}, 0, 0, withAnalyzerDetail(errors.New("analyzer_context_length"), raw)
		}
		return BatchResult{}, 0, 0, withAnalyzerDetail(fmt.Errorf("analyzer_http_%d", resp.StatusCode), raw)
	}
	result, promptTokens, completionTokens, parseErr := parseAnalyzerResponse(raw, validationSamples)
	if parseErr != nil {
		logAnalyzerFailure(endpoint, "parse", jsonMode, disableThinking, raw, parseErr)
		parseErr = withAnalyzerDetail(parseErr, raw)
	}
	return result, promptTokens, completionTokens, parseErr
}

func withAnalyzerDetail(err error, raw []byte) error {
	if err == nil {
		return nil
	}
	detail := analyzerResponseDetail(raw)
	if detail == "" {
		return err
	}
	return &analyzerFailure{cause: err, detail: truncateRunes(detail, 4096)}
}

func analyzerResponseDetail(raw []byte) string {
	var envelope struct {
		Choices []struct {
			Message struct {
				Content          json.RawMessage `json:"content"`
				ReasoningContent string          `json:"reasoning_content"`
				Reasoning        string          `json:"reasoning"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &envelope) == nil && len(envelope.Choices) > 0 {
		message := envelope.Choices[0].Message
		if content := strings.TrimSpace(analyzerMessageText(message.Content)); content != "" {
			return content
		}
		if reasoning := strings.TrimSpace(message.ReasoningContent); reasoning != "" {
			return reasoning
		}
		if reasoning := strings.TrimSpace(message.Reasoning); reasoning != "" {
			return reasoning
		}
	}
	return strings.TrimSpace(string(raw))
}

func analyzerErrorDetail(err error) string {
	var failure *analyzerFailure
	if errors.As(err, &failure) {
		return failure.detail
	}
	return ""
}

func logAnalyzerFailure(endpoint analysisEndpoint, stage string, jsonMode, disableThinking bool, raw []byte, err error) {
	const previewLimit = 16 * 1024
	preview := raw
	if len(preview) > previewLimit {
		preview = preview[:previewLimit]
	}
	accountID := int64(0)
	if endpoint.account != nil {
		accountID = endpoint.account.ID
	}
	logger.L().Warn("work insight analyzer attempt failed",
		zap.String("stage", stage), zap.Int64("account_id", accountID), zap.String("model", endpoint.model),
		zap.Bool("json_mode", jsonMode), zap.Bool("thinking_disabled", disableThinking), zap.Int("response_bytes", len(raw)),
		zap.Bool("response_truncated", len(raw) > len(preview)), zap.ByteString("response_preview", preview), zap.Error(err))
}

func parseAnalyzerResponse(raw []byte, samples []analysisInput) (BatchResult, int64, int64, error) {
	var envelope struct {
		Choices []struct {
			Message struct {
				Content          json.RawMessage `json:"content"`
				ReasoningContent string          `json:"reasoning_content"`
				Reasoning        string          `json:"reasoning"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Choices) != 1 {
		return BatchResult{}, 0, 0, errors.New("invalid analyzer response")
	}
	choice := envelope.Choices[0]
	result, err := decodeAnalyzerResult(analyzerMessageText(choice.Message.Content))
	if err != nil {
		reasoning := choice.Message.ReasoningContent
		if reasoning == "" {
			reasoning = choice.Message.Reasoning
		}
		result, err = decodeAnalyzerResult(reasoning)
	}
	if err != nil {
		if choice.FinishReason == "length" {
			return BatchResult{}, envelope.Usage.PromptTokens, envelope.Usage.CompletionTokens, errors.New("analyzer output truncated")
		}
		return BatchResult{}, envelope.Usage.PromptTokens, envelope.Usage.CompletionTokens, errors.New("invalid analyzer JSON")
	}
	if err := validateBatchResult(&result, samples); err != nil {
		return BatchResult{}, envelope.Usage.PromptTokens, envelope.Usage.CompletionTokens, err
	}
	return result, envelope.Usage.PromptTokens, envelope.Usage.CompletionTokens, nil
}

func analyzerMessageText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var object struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &object) == nil && object.Text != "" {
		return object.Text
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var joined strings.Builder
	for _, part := range parts {
		_, _ = joined.WriteString(part.Text)
	}
	return joined.String()
}

func decodeAnalyzerResult(content string) (BatchResult, error) {
	content = strings.TrimSpace(content)
	for offset := 0; offset < len(content); {
		index := strings.IndexByte(content[offset:], '{')
		if index < 0 {
			break
		}
		offset += index
		var raw json.RawMessage
		if err := json.NewDecoder(strings.NewReader(content[offset:])).Decode(&raw); err != nil {
			offset++
			continue
		}
		raw, err := normalizeAnalyzerResult(raw)
		if err != nil {
			offset++
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var result BatchResult
		if err := decoder.Decode(&result); err == nil {
			return result, nil
		}
		offset++
	}
	return BatchResult{}, errors.New("invalid analyzer JSON")
}

func normalizeAnalyzerResult(raw json.RawMessage) (json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	if value, ok := object["work_summary"]; ok {
		var text string
		if json.Unmarshal(value, &text) != nil {
			var items []string
			if err := json.Unmarshal(value, &items); err != nil {
				return nil, err
			}
			clean := items[:0]
			for _, item := range items {
				if item = strings.TrimSpace(strings.TrimLeft(item, "-•* ")); item != "" {
					clean = append(clean, item)
				}
			}
			if len(clean) == 0 {
				return nil, errors.New("empty work summary")
			}
			object["work_summary"], _ = json.Marshal("- " + strings.Join(clean, "\n- "))
		}
	}
	var representativeItems []map[string]json.RawMessage
	if value, ok := object["representative_items"]; ok && json.Unmarshal(value, &representativeItems) == nil {
		for _, item := range representativeItems {
			for _, field := range []string{"task_categories", "explicit_projects", "explicit_modules"} {
				if value, ok := item[field]; ok {
					var values []string
					if json.Unmarshal(value, &values) != nil {
						var single string
						if json.Unmarshal(value, &single) == nil && strings.TrimSpace(single) != "" {
							item[field], _ = json.Marshal([]string{strings.TrimSpace(single)})
						}
					}
				}
			}
			var values []json.RawMessage
			if json.Unmarshal(item["source_sample_ids"], &values) != nil {
				continue
			}
			ids := make([]int64, 0, len(values))
			for _, value := range values {
				var id int64
				if json.Unmarshal(value, &id) != nil {
					var text string
					if json.Unmarshal(value, &text) != nil {
						return nil, errors.New("invalid source sample ID")
					}
					parsed, err := strconv.ParseInt(text, 10, 64)
					if err != nil {
						return nil, errors.New("invalid source sample ID")
					}
					id = parsed
				}
				ids = append(ids, id)
			}
			item["source_sample_ids"], _ = json.Marshal(ids)
		}
		object["representative_items"], _ = json.Marshal(representativeItems)
	}
	return json.Marshal(object)
}

func (s *Service) analyzeChunkResilient(ctx context.Context, endpoint analysisEndpoint, previousSummary string, samples []analysisInput, allowSplit bool) (BatchResult, int64, int64, int, error) {
	result, input, output, err := s.analyzeChunkWithMode(ctx, endpoint, previousSummary, samples, true, true)
	if err == nil {
		return result, input, output, 1, nil
	}
	if isInvalidAnalyzerResult(err) || isJSONModeUnsupported(err) {
		repaired, retryInput, retryOutput, retryErr := s.analyzeChunkWithMode(ctx, endpoint, previousSummary, samples, false, true)
		input, output = input+retryInput, output+retryOutput
		if retryErr == nil {
			return repaired, input, output, 2, retryErr
		}
		repaired, retryInput, retryOutput, retryErr = s.analyzeChunk(ctx, endpoint, previousSummary, samples)
		return repaired, input + retryInput, output + retryOutput, 3, retryErr
	}
	if err.Error() != "analyzer_context_length" || !allowSplit {
		return BatchResult{}, input, output, 1, err
	}
	left, right := splitAnalysisInputs(samples)
	if len(right) == 0 {
		return BatchResult{}, input, output, 1, err
	}
	first, in1, out1, calls1, err := s.analyzeChunkResilient(ctx, endpoint, previousSummary, left, false)
	if err != nil {
		return BatchResult{}, input + in1, output + out1, 1 + calls1, err
	}
	second, in2, out2, calls2, err := s.analyzeChunkResilient(ctx, endpoint, previousSummary, right, false)
	if err != nil {
		return BatchResult{}, input + in1 + in2, output + out1 + out2, 1 + calls1 + calls2, err
	}
	return mergeBatchResults([]BatchResult{first, second}), input + in1 + in2, output + out1 + out2, 1 + calls1 + calls2, nil
}

func isJSONModeUnsupported(err error) bool {
	return err != nil && (err.Error() == "analyzer_http_400" || err.Error() == "analyzer_http_422")
}

func splitAnalysisInputs(samples []analysisInput) ([]analysisInput, []analysisInput) {
	if len(samples) > 1 {
		middle := len(samples) / 2
		return samples[:middle], samples[middle:]
	}
	if len(samples) == 0 {
		return nil, nil
	}
	runes := []rune(samples[0].Text)
	if len(runes) < 2 {
		return samples, nil
	}
	middle := len(runes) / 2
	left, right := samples[0], samples[0]
	left.Text, right.Text = string(runes[:middle]), string(runes[middle:])
	left.EstimatedTokens, right.EstimatedTokens = max(1, (middle+2)/3), max(1, (len(runes)-middle+2)/3)
	return []analysisInput{left}, []analysisInput{right}
}

func isInvalidAnalyzerResult(err error) bool {
	if err == nil {
		return false
	}
	value := err.Error()
	return strings.Contains(value, "invalid") || strings.Contains(value, "truncated") || strings.Contains(value, "evidence") || strings.Contains(value, "too many")
}

func validateBatchResult(result *BatchResult, samples []analysisInput) error {
	result.WorkSummary = strings.TrimSpace(result.WorkSummary)
	if result.WorkSummary == "" {
		return errors.New("invalid work summary")
	}
	if len(strings.FieldsFunc(result.WorkSummary, func(r rune) bool { return r == '\n' || r == '；' })) > 5 {
		return errors.New("too many work summary items")
	}
	result.WorkSummary = truncateRunes(result.WorkSummary, 300)
	validIDs := map[int64]struct{}{}
	var evidence strings.Builder
	for _, sample := range samples {
		validIDs[sample.ID] = struct{}{}
		_, _ = evidence.WriteString(strings.ToLower(sample.Text))
		_ = evidence.WriteByte('\n')
	}
	var err error
	if result.TaskCategories, err = validateList(result.TaskCategories, true); err != nil {
		return err
	}
	if slices.Contains(result.TaskCategories, "问题排查") && !supportsTroubleshooting(evidence.String()) {
		return errors.New("invalid inferred troubleshooting category")
	}
	if hasUnsupportedWorkClaim(result.WorkSummary, evidence.String()) {
		return errors.New("invalid inferred work action")
	}
	if result.ExplicitProjects, err = validateEvidenceList(result.ExplicitProjects, evidence.String()); err != nil {
		return err
	}
	if result.ExplicitModules, err = validateEvidenceList(result.ExplicitModules, evidence.String()); err != nil {
		return err
	}
	if result.ChangeTypes, err = validateList(result.ChangeTypes, false); err != nil {
		return err
	}
	if result.BusinessTopics, err = validateList(result.BusinessTopics, false); err != nil {
		return err
	}
	if result.EvidenceLevel != "explicit" {
		result.EvidenceLevel = "unknown"
	}
	if len(result.RepresentativeItems) > 10 {
		return errors.New("too many representative items")
	}
	validItems := result.RepresentativeItems[:0]
	seenItems := map[string]struct{}{}
	for _, item := range result.RepresentativeItems {
		item.Summary = strings.TrimSpace(item.Summary)
		if item.Summary == "" || utf8.RuneCountInString(item.Summary) > 120 || containsVerbatimSample(item.Summary, samples) || len(item.SourceSampleIDs) == 0 || len(item.SourceSampleIDs) > 20 {
			continue
		}
		sort.Slice(item.SourceSampleIDs, func(i, j int) bool { return item.SourceSampleIDs[i] < item.SourceSampleIDs[j] })
		item.SourceSampleIDs = slices.Compact(item.SourceSampleIDs)
		valid := true
		var itemEvidence strings.Builder
		for _, id := range item.SourceSampleIDs {
			if _, ok := validIDs[id]; !ok {
				valid = false
				break
			}
			for _, sample := range samples {
				if sample.ID == id {
					_, _ = itemEvidence.WriteString(sample.Text)
					_ = itemEvidence.WriteByte('\n')
				}
			}
		}
		if !valid || hasUnsupportedWorkClaim(item.Summary, itemEvidence.String()) {
			continue
		}
		item.TaskCategories, err = validateList(item.TaskCategories, true)
		if err != nil {
			continue
		}
		if slices.Contains(item.TaskCategories, "问题排查") && !supportsTroubleshooting(itemEvidence.String()) {
			continue
		}
		item.ExplicitProjects, err = validateEvidenceList(item.ExplicitProjects, evidence.String())
		if err != nil {
			continue
		}
		item.ExplicitModules, err = validateEvidenceList(item.ExplicitModules, evidence.String())
		if err != nil {
			continue
		}
		key := representativeItemKey(item)
		if _, exists := seenItems[key]; exists {
			continue
		}
		seenItems[key] = struct{}{}
		validItems = append(validItems, item)
	}
	result.RepresentativeItems = validItems
	return nil
}

func hasUnsupportedWorkClaim(summary, evidence string) bool {
	summary, evidence = strings.ToLower(summary), strings.ToLower(evidence)
	inquiry := false
	for _, cue := range []string{"查询", "咨询", "了解", "解释", "是什么", "怎么", "如何", "请问", "获取", "查看"} {
		if strings.Contains(evidence, cue) {
			inquiry = true
			break
		}
	}
	if !inquiry || supportsTroubleshooting(evidence) {
		return false
	}
	for _, claim := range []string{"新增", "开发", "实现", "修复", "排查", "设计", "部署", "完成", "修改", "优化"} {
		if strings.Contains(summary, claim) && !strings.Contains(evidence, claim) {
			return true
		}
	}
	return false
}

func supportsTroubleshooting(evidence string) bool {
	evidence = strings.ToLower(evidence)
	for _, cue := range []string{"排查", "问题", "报错", "错误", "异常", "故障", "失败", "无法", "不能", "不生效", "bug"} {
		if strings.Contains(evidence, cue) {
			return true
		}
	}
	return false
}

func containsVerbatimSample(summary string, samples []analysisInput) bool {
	summary = strings.ToLower(strings.Join(strings.Fields(summary), " "))
	if utf8.RuneCountInString(summary) < 8 {
		return false
	}
	for _, sample := range samples {
		text := strings.ToLower(strings.Join(strings.Fields(sample.Text), " "))
		if strings.Contains(text, summary) || (utf8.RuneCountInString(text) >= 8 && strings.Contains(summary, text)) {
			return true
		}
	}
	return false
}

func validateList(values []string, categories bool) ([]string, error) {
	if len(values) > 10 {
		return nil, errors.New("too many result values")
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || utf8.RuneCountInString(value) > 64 {
			return nil, errors.New("invalid result value")
		}
		if categories && !slices.Contains(TaskCategories, value) {
			value = "其他"
		}
		result = append(result, value)
	}
	sort.Strings(result)
	return slices.Compact(result), nil
}

func validateEvidenceList(values []string, evidence string) ([]string, error) {
	result, err := validateList(values, false)
	if err != nil {
		return nil, err
	}
	verified := result[:0]
	for _, value := range result {
		if strings.Contains(evidence, strings.ToLower(value)) {
			verified = append(verified, value)
		}
	}
	return verified, nil
}

func mergeBatchResults(results []BatchResult) BatchResult {
	merged := BatchResult{EvidenceLevel: "unknown"}
	var summaries []string
	seenSummaries := map[string]struct{}{}
	addSummary := func(value string) {
		if len(summaries) >= 5 {
			return
		}
		value = strings.TrimSpace(strings.TrimLeft(value, "-•* "))
		key := strings.ToLower(strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, value))
		if key != "" {
			if _, seen := seenSummaries[key]; !seen {
				summaries = append(summaries, value)
				seenSummaries[key] = struct{}{}
			}
		}
	}
	for _, result := range results {
		for _, summary := range strings.FieldsFunc(result.WorkSummary, func(r rune) bool { return r == '\n' || r == '；' }) {
			addSummary(summary)
		}
		merged.TaskCategories = mergeUnique(merged.TaskCategories, result.TaskCategories)
		merged.ExplicitProjects = mergeUnique(merged.ExplicitProjects, result.ExplicitProjects)
		merged.ExplicitModules = mergeUnique(merged.ExplicitModules, result.ExplicitModules)
		merged.ChangeTypes = mergeUnique(merged.ChangeTypes, result.ChangeTypes)
		merged.BusinessTopics = mergeUnique(merged.BusinessTopics, result.BusinessTopics)
		for _, item := range result.RepresentativeItems {
			key := representativeItemKey(item)
			found := false
			for _, existing := range merged.RepresentativeItems {
				if representativeItemKey(existing) == key {
					found = true
					break
				}
			}
			if !found && len(merged.RepresentativeItems) < 10 {
				merged.RepresentativeItems = append(merged.RepresentativeItems, item)
				addSummary(item.Summary)
			}
		}
		if result.EvidenceLevel == "explicit" {
			merged.EvidenceLevel = "explicit"
		}
	}
	merged.WorkSummary = "- " + strings.Join(summaries, "\n- ")
	if len(merged.RepresentativeItems) > 10 {
		merged.RepresentativeItems = merged.RepresentativeItems[:10]
	}
	return merged
}

func mergeWorkSummaries(latest, previous string) string {
	return mergeBatchResults([]BatchResult{{WorkSummary: latest}, {WorkSummary: previous}}).WorkSummary
}

func representativeItemKey(item RepresentativeItem) string {
	if summary := strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, item.Summary)); summary != "" {
		return "summary:" + summary
	}
	if len(item.SourceSampleIDs) > 0 {
		ids := make([]string, len(item.SourceSampleIDs))
		for i, id := range item.SourceSampleIDs {
			ids[i] = strconv.FormatInt(id, 10)
		}
		return "id:" + strings.Join(ids, ",")
	}
	return "summary:"
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

const analyzerInstruction = `你是企业 AI 使用洞察分析器。只分析脱敏后的用户请求，不做绩效、合规或是否工作的判断。必须忠实保留用户表达的意图和事实，不得把查询、咨询、了解、解释等请求升级为开发、实现、排查、修复、设计、部署或已完成事项；例如“查询南昌天气”只能概括为“查询南昌天气”，不能写成“排查天气查询接口调用逻辑”。不得添加输入中未出现的接口、功能、逻辑、系统、故障或完成状态。项目和模块只能收录输入中明确出现的名称，不得推断。返回且仅返回 JSON 对象，字段严格为 work_summary、task_categories、explicit_projects、explicit_modules、change_types、business_topics、representative_items、evidence_level。task_categories 只能从以下枚举选择：代码开发、问题排查、测试用例、接口文档、需求分析、方案设计、数据分析、SQL/报表、运维部署、日志分析、文档写作、翻译润色、会议纪要、客服支持、培训学习、其他；只有输入明确描述问题、错误、异常、失败或排查行为时才能使用“问题排查”。evidence_level 只能是 "explicit" 或 "unknown"。work_summary 是截至当前的当日汇总：如果输入包含“上一版结构化摘要”，必须把上一版与本批样本按相同主题重新归纳，合并重复、近似、上下游步骤和同一问题的排查/修复，不得逐项追加细节；始终输出最多 5 条主题级总结，每行以 "- " 开头。查询就写“查询…”，咨询就写“咨询…”，了解就写“了解…”，只有输入明确表达相应动作时才可写“新增、开发、实现、修复、排查、设计、部署、完成、修改、优化”。不要写“用户询问”“提出要求”“希望”“请求”等对话描述。representative_items 每项包含 source_sample_ids、summary、task_categories、explicit_projects、explicit_modules；summary 每项只写一件具体事项，动作和对象都必须能由对应 source_sample_ids 的输入直接支持，同批相同事项合并。未明确出现的项目和模块返回空数组。`
