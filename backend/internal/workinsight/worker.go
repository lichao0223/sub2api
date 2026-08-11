package workinsight

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	jobBatchScheduler = "ai_work_insight_batch_scheduler"
	jobReconciliation = "ai_work_insight_reconciliation"
	jobDailyFinalize  = "ai_work_insight_daily_finalize"
	jobCleanup        = "ai_work_insight_cleanup"
)

func (s *Service) scheduler(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			cfg := s.config.Load()
			if cfg == nil || !cfg.Enabled {
				continue
			}
			forceReason := ""
			if cfg.AnalysisTriggerMode == "fixed_time" {
				location, err := time.LoadLocation(cfg.Timezone)
				if err == nil {
					local := now.In(location)
					minute := local.Format("15:04")
					for _, value := range cfg.FixedTimes {
						if value == minute {
							forceReason = "fixed"
							break
						}
					}
				}
			}
			started := time.Now()
			workCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			expired, err := s.repo.DropExpiredSamples(workCtx, now.Add(-time.Duration(cfg.MaxJobAgeMinutes)*time.Minute))
			if err == nil {
				s.deletePayloads(workCtx, expired)
			}
			created := 0
			if err == nil {
				created, err = s.repo.CreateDueBatches(workCtx, now, cfg.Config, forceReason)
			}
			cancel()
			s.reportJob(jobBatchScheduler, started, fmt.Sprintf("queued_batches=%d expired_samples=%d", created, len(expired)), err)
		}
	}
}

func (s *Service) maintenance(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	initial := time.NewTimer(10 * time.Second)
	defer initial.Stop()
	lastReconcile, lastFinalize, lastCleanup := "", "", ""
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-initial.C:
			s.runReconciliation(ctx, now)
			lastFinalize = s.tryFinalizePrevious(ctx, now, lastFinalize)
		case now := <-ticker.C:
			cfg := s.config.Load()
			if cfg == nil || !cfg.Enabled {
				continue
			}
			location, err := time.LoadLocation(cfg.Timezone)
			if err != nil {
				continue
			}
			local := now.In(location)
			if key := local.Format("2006-01-02 15"); local.Minute() == 0 && lastReconcile != key {
				lastReconcile = key
				s.runReconciliation(ctx, now)
				lastFinalize = s.tryFinalizePrevious(ctx, now, lastFinalize)
			}
			if local.Format("15:04") == cfg.DailyFinalizeTime {
				lastFinalize = s.tryFinalizePrevious(ctx, now, lastFinalize)
			}
			if cfg.CleanupEnabled {
				if key := local.Format("2006-01-02") + " " + cfg.CleanupTime; local.Format("15:04") == cfg.CleanupTime && lastCleanup != key {
					lastCleanup = key
					s.runCleanup(ctx, now, *cfg)
				}
			}
		}
	}
}

func (s *Service) runReconciliation(parent context.Context, now time.Time) {
	s.pruneIngressSessions(now.Add(-24 * time.Hour))
	cfg := s.config.Load()
	if cfg == nil || !cfg.Enabled {
		return
	}
	started := time.Now()
	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		s.reportJob(jobReconciliation, started, "", err)
		return
	}
	dateText := now.In(location).Format("2006-01-02")
	date, _ := time.Parse("2006-01-02", dateText)
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	users, err := s.reconcileDate(ctx, date, dateText, cfg.Timezone)
	s.reportJob(jobReconciliation, started, fmt.Sprintf("users=%d", users), err)
}

func (s *Service) tryFinalizePrevious(ctx context.Context, now time.Time, completed string) string {
	cfg := s.config.Load()
	if cfg == nil || !cfg.Enabled {
		return completed
	}
	key, date, due := previousFinalizeTarget(now, *cfg)
	if !due || key == completed {
		return completed
	}
	if complete, err := s.runFinalize(ctx, date, *cfg); err == nil && complete {
		return key
	}
	return completed
}

func previousFinalizeTarget(now time.Time, cfg storedConfig) (string, time.Time, bool) {
	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return "", time.Time{}, false
	}
	local := now.In(location)
	if local.Format("15:04") < cfg.DailyFinalizeTime {
		return "", time.Time{}, false
	}
	date := local.AddDate(0, 0, -1)
	return date.Format("2006-01-02"), date, true
}

func (s *Service) reconcileDate(ctx context.Context, date time.Time, dateText, timezone string) (int64, error) {
	users, err := s.repo.ReconcileUsage(ctx, date, timezone)
	if err == nil {
		err = s.reconcileCoverage(ctx, date, dateText)
	}
	return users, err
}

func (s *Service) reconcileCoverage(ctx context.Context, date time.Time, dateText string) error {
	var cursor uint64
	pattern := "sub2api:ai_work_insight:" + dateText + ":eligible:*"
	for {
		keys, next, err := s.redis.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			return err
		}
		for _, key := range keys {
			userID, err := strconv.ParseInt(key[strings.LastIndex(key, ":")+1:], 10, 64)
			if err != nil {
				continue
			}
			eligible, err := s.redis.Get(ctx, key).Int()
			if err != nil {
				return err
			}
			covered, username, err := s.repo.CoveredSessions(ctx, userID, date)
			if err != nil {
				return err
			}
			if err := s.repo.PersistCoverage(ctx, userID, username, date, eligible, covered); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func (s *Service) runFinalize(parent context.Context, local time.Time, cfg storedConfig) (bool, error) {
	started := time.Now()
	date, _ := time.Parse("2006-01-02", local.Format("2006-01-02"))
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	_, err := s.repo.ReconcileUsage(ctx, date, cfg.Timezone)
	if err == nil {
		err = s.reconcileCoverage(ctx, date, local.Format("2006-01-02"))
	}
	var finalized int64
	complete := false
	if err == nil {
		finalized, err = s.repo.Finalize(ctx, date)
	}
	if err == nil {
		var pending bool
		pending, err = s.repo.HasOpenBatches(ctx, date)
		complete = err == nil && !pending
	}
	s.reportJob(jobDailyFinalize, started, fmt.Sprintf("dates=1 users=%d complete=%t", finalized, complete), err)
	return complete, err
}

func (s *Service) runCleanup(parent context.Context, now time.Time, cfg storedConfig) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	var total CleanupResult
	for {
		result, err := s.repo.Cleanup(ctx, now, cfg.Config)
		total.Samples += result.Samples
		total.Batches += result.Batches
		total.Daily += result.Daily
		if err != nil || !cleanupHasMore(result, cfg.CleanupBatchSize) {
			s.reportJob(jobCleanup, started, fmt.Sprintf("samples=%d batches=%d daily=%d", total.Samples, total.Batches, total.Daily), err)
			return
		}
	}
}

func cleanupHasMore(result CleanupResult, batchSize int) bool {
	limit := int64(batchSize)
	return result.Samples >= limit || result.Batches >= limit || result.Daily >= limit
}

func (s *Service) reportJob(name string, started time.Time, result string, runErr error) {
	if s.ops == nil {
		return
	}
	finished, duration := time.Now().UTC(), time.Since(started).Milliseconds()
	started = started.UTC()
	input := &service.OpsUpsertJobHeartbeatInput{JobName: name, LastRunAt: &started, LastDurationMs: &duration}
	if runErr == nil {
		input.LastSuccessAt, input.LastResult = &finished, &result
	} else {
		code := name + "_failed"
		input.LastErrorAt, input.LastError = &finished, &code
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.ops.UpsertJobHeartbeat(ctx, input)
}

func (s *Service) batchWorker(ctx context.Context, workerID int) {
	defer s.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cfg := s.config.Load()
			if cfg == nil || !cfg.Enabled || workerID >= cfg.WorkerCount || s.analyzerPaused(time.Now()) {
				continue
			}
			workCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.AnalysisTimeoutSeconds+10)*time.Second)
			s.processNextBatch(workCtx, *cfg)
			cancel()
		}
	}
}

func (s *Service) processNextBatch(ctx context.Context, cfg storedConfig) {
	now := time.Now()
	batch, claimed, err := s.repo.ClaimBatch(ctx, now, time.Duration(cfg.AnalysisTimeoutSeconds+20)*time.Second)
	if err != nil || !claimed {
		return
	}
	samples, err := s.repo.LoadBatchSamples(ctx, batch.ID)
	if err != nil {
		s.finishBatchFailure(ctx, cfg, *batch, nil, "sample_load_failed", true)
		return
	}
	if len(samples) == 0 {
		s.finishBatchFailure(ctx, cfg, *batch, samples, "samples_missing", false)
		return
	}
	if batchExpired(*batch, cfg, now) {
		s.finishBatchFailure(ctx, cfg, *batch, samples, "job_expired", false)
		return
	}
	inputs := make([]analysisInput, 0, len(samples))
	for _, sample := range samples {
		payload, err := s.redis.Get(ctx, PayloadKeyPrefix+strconv.FormatInt(sample.ID, 10)).Result()
		if err != nil {
			s.finishBatchFailure(ctx, cfg, *batch, samples, "payload_missing", false)
			return
		}
		for _, text := range decodePayload(payload) {
			inputs = append(inputs, analysisInput{ID: sample.ID, Text: text, EstimatedTokens: max(1, (utf8.RuneCountInString(text)+2)/3)})
		}
	}
	chunks := buildAnalysisChunks(inputs, cfg.MaxInputTokens)
	if len(chunks) == 0 {
		s.finishBatchFailure(ctx, cfg, *batch, samples, "payload_empty", false)
		return
	}
	endpoint, err := s.resolveAnalyzer(ctx, cfg)
	if err != nil {
		s.finishBatchFailure(ctx, cfg, *batch, samples, "analyzer_config_invalid", false)
		return
	}
	previous, _, err := s.repo.DailySummary(ctx, batch.UserID, batch.LocalDate)
	if err != nil {
		s.finishBatchFailure(ctx, cfg, *batch, samples, "summary_load_failed", true)
		return
	}
	results := make([]BatchResult, 0, len(chunks))
	var inputTokens, outputTokens int64
	callCount := 0
	for _, chunk := range chunks {
		if batchExpired(*batch, cfg, time.Now()) {
			s.finishBatchFailure(ctx, cfg, *batch, samples, "job_expired", false)
			return
		}
		result, in, out, calls, err := s.analyzeChunkResilient(ctx, endpoint, previous, chunk, true)
		if err != nil {
			retryable := analyzerRetryable(err)
			if retryable {
				s.pauseAnalyzer(time.Now())
			}
			s.finishBatchFailure(ctx, cfg, *batch, samples, analyzerErrorCode(err), retryable)
			return
		}
		results, inputTokens, outputTokens, callCount = append(results, result), inputTokens+in, outputTokens+out, callCount+calls
	}
	eligible, _ := s.redis.Get(ctx, "sub2api:ai_work_insight:"+batch.LocalDate.Format("2006-01-02")+":eligible:"+strconv.FormatInt(batch.UserID, 10)).Int()
	result := mergeBatchResults(results)
	if err := s.repo.CompleteBatch(ctx, *batch, result, cfg.AnalyzerModel, inputTokens, outputTokens, callCount, eligible, cfg.Timezone); err != nil {
		s.finishBatchFailure(ctx, cfg, *batch, samples, "summary_write_conflict", true)
		return
	}
	s.deletePayloads(ctx, samples)
}

func batchExpired(batch Batch, cfg storedConfig, now time.Time) bool {
	return !now.Before(batch.CreatedAt.Add(time.Duration(cfg.MaxJobAgeMinutes) * time.Minute))
}

func (s *Service) finishBatchFailure(ctx context.Context, cfg storedConfig, batch Batch, samples []BatchSample, code string, retryable bool) {
	expires := batch.CreatedAt.Add(time.Duration(cfg.MaxJobAgeMinutes) * time.Minute)
	next := time.Now().Add(time.Duration(1<<min(batch.Attempts, 5)) * time.Second)
	if retryable && batch.Attempts < 3 && next.Before(expires) {
		_ = s.repo.RetryBatch(ctx, batch, next, code)
		return
	}
	if !time.Now().Before(expires) {
		code = "job_expired"
	}
	dropped, err := s.repo.DropBatch(ctx, batch, code)
	if err == nil {
		if len(dropped) > 0 {
			samples = dropped
		}
		s.deletePayloads(ctx, samples)
	}
}

func (s *Service) pauseAnalyzer(now time.Time) {
	s.analyzerPauseUntil.Store(now.Add(30 * time.Second).UnixMilli())
}

func (s *Service) analyzerPaused(now time.Time) bool {
	return now.UnixMilli() < s.analyzerPauseUntil.Load()
}

func (s *Service) deletePayloads(ctx context.Context, samples []BatchSample) {
	if len(samples) == 0 {
		return
	}
	keys := make([]string, len(samples))
	ids := make([]string, len(samples))
	for i, sample := range samples {
		ids[i] = strconv.FormatInt(sample.ID, 10)
		keys[i] = PayloadKeyPrefix + ids[i]
	}
	if s.redis.Del(ctx, keys...).Err() == nil {
		s.releasePayloadSlots(ctx, ids)
	}
}

func analyzerErrorCode(err error) string {
	value := err.Error()
	if strings.HasPrefix(value, "analyzer_http_") {
		return value
	}
	switch value {
	case "analyzer_context_length":
		return value
	case "invalid analyzer JSON", "invalid analyzer response", "invalid work summary", "invalid task category", "result lacks explicit evidence":
		return "analyzer_invalid_result"
	default:
		return "analyzer_unavailable"
	}
}

func analyzerRetryable(err error) bool {
	if err == nil {
		return false
	}
	code := err.Error()
	if code == "analyzer_context_length" {
		return false
	}
	if strings.HasPrefix(code, "analyzer_http_") {
		status, _ := strconv.Atoi(strings.TrimPrefix(code, "analyzer_http_"))
		return status == 429 || status >= 500
	}
	return !errors.Is(err, context.Canceled) && !strings.Contains(code, "invalid") && !strings.Contains(code, "evidence")
}
