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
	"strings"
	"time"
	"unicode/utf8"

	openaiapi "github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
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
		if s.accounts == nil {
			return endpoint, errors.New("analyzer account repository unavailable")
		}
		account, err := s.accounts.GetByID(ctx, cfg.AnalyzerAccountID)
		if err != nil {
			return endpoint, err
		}
		if account == nil || !account.IsActive() {
			return endpoint, errors.New("analyzer account unavailable")
		}
		endpoint.account = account
		endpoint.baseURL = strings.TrimSpace(account.GetCredential("base_url"))
		endpoint.token = strings.TrimSpace(account.GetCredential("api_key"))
		if account.Platform == service.PlatformOpenAI {
			if endpoint.baseURL == "" {
				endpoint.baseURL = account.GetOpenAIBaseURL()
			}
			if endpoint.token == "" {
				endpoint.token = account.GetOpenAIAccessToken()
			}
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
		_, _, _, err = s.analyzeChunk(ctx, endpoint, "", []analysisInput{{ID: 1, Text: "连接检测样本：整理一份简短会议纪要。", EstimatedTokens: 16}})
	}
	result := ProbeResult{OK: err == nil, Status: "ok", Message: "分析节点连接正常", LatencyMS: time.Since(started).Milliseconds(), CheckedAt: time.Now().UTC()}
	if err != nil {
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
		baseURL := strings.TrimSpace(account.GetCredential("base_url"))
		token := strings.TrimSpace(account.GetCredential("api_key"))
		if account.Platform == service.PlatformOpenAI {
			if baseURL == "" {
				baseURL = account.GetOpenAIBaseURL()
			}
			if token == "" {
				token = account.GetOpenAIAccessToken()
			}
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
	case isInvalidAnalyzerResult(err):
		return "分析节点响应格式不兼容"
	case strings.Contains(message, "credentials incomplete"):
		return "分析账号配置不完整"
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
		sample := samples[i]
		maxRunes := budget * 3
		if utf8.RuneCountInString(sample.Text) > maxRunes {
			sample.Text = string([]rune(sample.Text)[:maxRunes])
		}
		sample.EstimatedTokens = max(1, (utf8.RuneCountInString(sample.Text)+2)/3)
		deduped = append(deduped, sample)
	}
	slices.Reverse(deduped)
	var chunks [][]analysisInput
	for _, sample := range deduped {
		if len(chunks) == 0 || chunkTokens(chunks[len(chunks)-1])+sample.EstimatedTokens > budget {
			chunks = append(chunks, []analysisInput{sample})
		} else {
			chunks[len(chunks)-1] = append(chunks[len(chunks)-1], sample)
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
	if previousSummary != "" {
		_, _ = input.WriteString("上一版结构化摘要：\n" + previousSummary + "\n\n")
	}
	_, _ = input.WriteString("本批脱敏样本：\n")
	for _, sample := range samples {
		fmt.Fprintf(&input, "<sample id=%d>\n%s\n</sample>\n", sample.ID, sample.Text)
	}
	payload := map[string]any{
		"model": endpoint.model, "temperature": 0,
		"messages": []map[string]string{{"role": "system", "content": analyzerInstruction}, {"role": "user", "content": input.String()}},
	}
	body, _ := json.Marshal(payload)
	if endpoint.account != nil && endpoint.account.IsOpenAIOAuth() {
		if s.openAIGateway == nil {
			return BatchResult{}, 0, 0, errors.New("analyzer gateway unavailable")
		}
		recorder := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(recorder)
		ginContext.Request = httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		ginContext.Request.Header.Set("Content-Type", "application/json")
		_, gatewayErr := s.openAIGateway.ForwardAsChatCompletions(ctx, ginContext, endpoint.account, body, "", endpoint.model)
		if gatewayErr != nil {
			if recorder.Code >= 400 {
				return BatchResult{}, 0, 0, fmt.Errorf("analyzer_http_%d", recorder.Code)
			}
			return BatchResult{}, 0, 0, gatewayErr
		}
		return parseAnalyzerResponse(recorder.Body.Bytes(), samples)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return BatchResult{}, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+endpoint.token)
	resp, err := client.Do(req)
	if err != nil {
		return BatchResult{}, 0, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024+1))
	if err != nil {
		return BatchResult{}, 0, 0, err
	}
	if len(raw) > 256*1024 {
		return BatchResult{}, 0, 0, errors.New("analyzer response too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		lower := strings.ToLower(string(raw))
		if resp.StatusCode == http.StatusRequestEntityTooLarge || strings.Contains(lower, "context_length") || strings.Contains(lower, "context length") || strings.Contains(lower, "maximum context") {
			return BatchResult{}, 0, 0, errors.New("analyzer_context_length")
		}
		return BatchResult{}, 0, 0, fmt.Errorf("analyzer_http_%d", resp.StatusCode)
	}
	return parseAnalyzerResponse(raw, samples)
}

func parseAnalyzerResponse(raw []byte, samples []analysisInput) (BatchResult, int64, int64, error) {
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Choices) != 1 {
		return BatchResult{}, 0, 0, errors.New("invalid analyzer response")
	}
	content := strings.TrimSpace(envelope.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(content)))
	decoder.DisallowUnknownFields()
	var result BatchResult
	if err := decoder.Decode(&result); err != nil {
		return BatchResult{}, envelope.Usage.PromptTokens, envelope.Usage.CompletionTokens, errors.New("invalid analyzer JSON")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return BatchResult{}, envelope.Usage.PromptTokens, envelope.Usage.CompletionTokens, errors.New("invalid analyzer JSON")
	}
	if err := validateBatchResult(&result, samples); err != nil {
		return BatchResult{}, envelope.Usage.PromptTokens, envelope.Usage.CompletionTokens, err
	}
	return result, envelope.Usage.PromptTokens, envelope.Usage.CompletionTokens, nil
}

func (s *Service) analyzeChunkResilient(ctx context.Context, endpoint analysisEndpoint, previousSummary string, samples []analysisInput, allowSplit bool) (BatchResult, int64, int64, int, error) {
	result, input, output, err := s.analyzeChunk(ctx, endpoint, previousSummary, samples)
	if err == nil {
		return result, input, output, 1, nil
	}
	if isInvalidAnalyzerResult(err) {
		repaired, retryInput, retryOutput, retryErr := s.analyzeChunk(ctx, endpoint, previousSummary, samples)
		return repaired, input + retryInput, output + retryOutput, 2, retryErr
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
	return strings.Contains(value, "invalid") || strings.Contains(value, "evidence") || strings.Contains(value, "too many")
}

func validateBatchResult(result *BatchResult, samples []analysisInput) error {
	result.WorkSummary = strings.TrimSpace(result.WorkSummary)
	if result.WorkSummary == "" || utf8.RuneCountInString(result.WorkSummary) > 300 {
		return errors.New("invalid work summary")
	}
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
	if result.EvidenceLevel != "explicit" && result.EvidenceLevel != "unknown" {
		return errors.New("invalid evidence level")
	}
	if len(result.RepresentativeItems) > 10 {
		return errors.New("too many representative items")
	}
	validItems := result.RepresentativeItems[:0]
	for _, item := range result.RepresentativeItems {
		item.Summary = strings.TrimSpace(item.Summary)
		if item.Summary == "" || utf8.RuneCountInString(item.Summary) > 120 || containsVerbatimSample(item.Summary, samples) || len(item.SourceSampleIDs) == 0 || len(item.SourceSampleIDs) > 20 {
			continue
		}
		sort.Slice(item.SourceSampleIDs, func(i, j int) bool { return item.SourceSampleIDs[i] < item.SourceSampleIDs[j] })
		item.SourceSampleIDs = slices.Compact(item.SourceSampleIDs)
		valid := true
		for _, id := range item.SourceSampleIDs {
			if _, ok := validIDs[id]; !ok {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		item.TaskCategories, err = validateList(item.TaskCategories, true)
		if err != nil {
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
		validItems = append(validItems, item)
	}
	result.RepresentativeItems = validItems
	return nil
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
			return nil, errors.New("invalid task category")
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
	for _, value := range result {
		if !strings.Contains(evidence, strings.ToLower(value)) {
			return nil, errors.New("result lacks explicit evidence")
		}
	}
	return result, nil
}

func mergeBatchResults(results []BatchResult) BatchResult {
	merged := BatchResult{EvidenceLevel: "unknown"}
	var summaries []string
	for _, result := range results {
		summaries = append(summaries, result.WorkSummary)
		merged.TaskCategories = mergeUnique(merged.TaskCategories, result.TaskCategories)
		merged.ExplicitProjects = mergeUnique(merged.ExplicitProjects, result.ExplicitProjects)
		merged.ExplicitModules = mergeUnique(merged.ExplicitModules, result.ExplicitModules)
		merged.ChangeTypes = mergeUnique(merged.ChangeTypes, result.ChangeTypes)
		merged.BusinessTopics = mergeUnique(merged.BusinessTopics, result.BusinessTopics)
		merged.RepresentativeItems = append(merged.RepresentativeItems, result.RepresentativeItems...)
		if result.EvidenceLevel == "explicit" {
			merged.EvidenceLevel = "explicit"
		}
	}
	merged.WorkSummary = truncateRunes(strings.Join(mergeUnique(nil, summaries), "；"), 300)
	if len(merged.RepresentativeItems) > 10 {
		merged.RepresentativeItems = merged.RepresentativeItems[:10]
	}
	return merged
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

const analyzerInstruction = `你是企业 AI 使用洞察分析器。只分析脱敏后的用户请求，不做绩效、合规或是否工作的判断。项目和模块只能收录输入中明确出现的名称，不得推断。返回且仅返回 JSON 对象，字段严格为 work_summary、task_categories、explicit_projects、explicit_modules、change_types、business_topics、representative_items、evidence_level。task_categories 只能从以下枚举选择：代码开发、问题排查、测试用例、接口文档、需求分析、方案设计、数据分析、SQL/报表、运维部署、日志分析、文档写作、翻译润色、会议纪要、客服支持、培训学习、其他。representative_items 每项包含 source_sample_ids、summary、task_categories、explicit_projects、explicit_modules。未明确出现的项目和模块返回空数组。`
