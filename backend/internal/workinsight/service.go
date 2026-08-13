package workinsight

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const samplingScript = `
local last = tonumber(redis.call('HGET', KEYS[1], 'last_ms') or '0')
local sid = redis.call('HGET', KEYS[1], 'session_id')
local fresh = (last == 0 or tonumber(ARGV[1]) - last > tonumber(ARGV[2]))
if fresh then
  sid = ARGV[3] .. ':' .. redis.call('INCR', KEYS[5])
  redis.call('INCR', KEYS[2])
  redis.call('HSET', KEYS[1], 'coverage', 'uncovered', 'claim_token', '', 'claim_ms', '0')
end
redis.call('HSET', KEYS[1], 'last_ms', ARGV[1], 'session_id', sid)
for i=1,5 do redis.call('EXPIRE', KEYS[i], ARGV[9]) end
local coverage = redis.call('HGET', KEYS[1], 'coverage') or 'uncovered'
local claim_ms = tonumber(redis.call('HGET', KEYS[1], 'claim_ms') or '0')
if coverage == 'claiming' and tonumber(ARGV[1]) - claim_ms > 30000 then
  coverage = 'uncovered'
  redis.call('HSET', KEYS[1], 'coverage', coverage, 'claim_token', '', 'claim_ms', '0')
end
local coverage_claim = coverage == 'uncovered'
local forced_reason = ARGV[10]
local selected = coverage_claim or forced_reason ~= '' or ARGV[4] == '1'
if not selected then return {sid, fresh and '1' or '0', '0', 'rate_miss'} end
if tonumber(redis.call('GET', KEYS[3]) or '0') >= tonumber(ARGV[5]) then return {sid, fresh and '1' or '0', '0', 'user_limit'} end
if tonumber(redis.call('GET', KEYS[4]) or '0') >= tonumber(ARGV[6]) then return {sid, fresh and '1' or '0', '0', 'global_limit'} end
redis.call('INCR', KEYS[3]); redis.call('INCR', KEYS[4])
redis.call('EXPIRE', KEYS[3], ARGV[9]); redis.call('EXPIRE', KEYS[4], ARGV[9])
if coverage_claim then
  redis.call('HSET', KEYS[1], 'coverage', 'claiming', 'claim_token', ARGV[8], 'claim_ms', ARGV[1])
end
return {sid, fresh and '1' or '0', '1', coverage_claim and 'session_coverage' or (forced_reason ~= '' and forced_reason or 'rate')}
`

const finishSamplingScript = `
if ARGV[2] == '0' then
  if tonumber(redis.call('GET', KEYS[2]) or '0') > 0 then redis.call('DECR', KEYS[2]) end
  if tonumber(redis.call('GET', KEYS[3]) or '0') > 0 then redis.call('DECR', KEYS[3]) end
end
if ARGV[3] == '1' and redis.call('HGET', KEYS[1], 'claim_token') == ARGV[1] then
  if ARGV[2] == '1' then
    redis.call('HSET', KEYS[1], 'coverage', 'covered', 'claim_token', '', 'claim_ms', '0')
  else
    redis.call('HSET', KEYS[1], 'coverage', 'uncovered', 'claim_token', '', 'claim_ms', '0')
  end
end
return 1
`

const reservePayloadScript = `
local expired = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
local released = 0
for _, id in ipairs(expired) do
  released = released + tonumber(redis.call('HGET', KEYS[2], id) or '0')
  redis.call('HDEL', KEYS[2], id)
end
if #expired > 0 then redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1]) end
local used = math.max(0, tonumber(redis.call('GET', KEYS[3]) or '0') - released)
redis.call('SET', KEYS[3], used)
redis.call('EXPIRE', KEYS[3], tonumber(ARGV[7]) + 3600)
if redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[3]) or used + tonumber(ARGV[6]) > tonumber(ARGV[4]) then return 0 end
redis.call('ZADD', KEYS[1], ARGV[2], ARGV[5])
redis.call('HSET', KEYS[2], ARGV[5], ARGV[6])
redis.call('INCRBY', KEYS[3], ARGV[6])
for i=1,3 do redis.call('EXPIRE', KEYS[i], tonumber(ARGV[7]) + 3600) end
return 1
`

const releasePayloadScript = `
local released = 0
for i=1,#ARGV do
  released = released + tonumber(redis.call('HGET', KEYS[2], ARGV[i]) or '0')
  redis.call('HDEL', KEYS[2], ARGV[i])
  redis.call('ZREM', KEYS[1], ARGV[i])
end
local used = math.max(0, tonumber(redis.call('GET', KEYS[3]) or '0') - released)
if used == 0 then
  redis.call('DEL', KEYS[3])
else
  local ttl = redis.call('TTL', KEYS[1])
  redis.call('SET', KEYS[3], used)
  if ttl > 0 then redis.call('EXPIRE', KEYS[3], ttl) end
end
return used
`

const releaseConversationLockScript = `
if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end
return 0
`

const (
	conversationStateTTL = 72 * time.Hour
)

type Service struct {
	repo               *Repository
	redis              *redis.Client
	settings           service.SettingRepository
	encryptor          service.SecretEncryptor
	accounts           service.AccountRepository
	ops                service.OpsRepository
	openAIGateway      *service.OpenAIGatewayService
	queue              chan queuedRequest
	config             atomic.Pointer[storedConfig]
	queuedCount        atomic.Int64
	queuedBytes        atomic.Int64
	dropped            atomic.Int64
	processed          atomic.Int64
	failed             atomic.Int64
	analyzerPauseUntil atomic.Int64
	ingressSessions    sync.Map
	batchCancels       sync.Map
	cancel             context.CancelFunc
	wg                 sync.WaitGroup
}

type queuedRequest struct {
	request securityaudit.Request
	bytes   int64
}

func NewService(repo *Repository, rdb *redis.Client, settings service.SettingRepository, encryptor service.SecretEncryptor, accounts service.AccountRepository, ops service.OpsRepository, openAIGateway *service.OpenAIGatewayService) *Service {
	s := &Service{repo: repo, redis: rdb, settings: settings, encryptor: encryptor, accounts: accounts, ops: ops, openAIGateway: openAIGateway, queue: make(chan queuedRequest, DefaultIngressLimit)}
	defaults := storedConfig{Config: DefaultConfig()}
	s.config.Store(&defaults)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = s.reload(ctx)
	cancel()
	runCtx, stop := context.WithCancel(context.Background())
	s.cancel = stop
	s.wg.Add(35 + ingressWorkerCount)
	for range ingressWorkerCount {
		go s.consume(runCtx)
	}
	go s.scheduler(runCtx)
	go s.maintenance(runCtx)
	go s.configRefresh(runCtx)
	for workerID := 0; workerID < 32; workerID++ {
		go s.batchWorker(runCtx, workerID)
	}
	return s
}

// TrySubmit is the only gateway-side entry. It performs no I/O and never waits.
func (s *Service) TrySubmit(req securityaudit.Request) {
	if s == nil {
		return
	}
	cfg := s.config.Load()
	if cfg == nil || !cfg.Enabled || cfg.excludes(req.UserID, req.UserEmail) || len(req.Body) == 0 || len(req.Body) > maxCapturedRequestBytes {
		return
	}
	if !s.ingressSelected(*cfg, req, time.Now()) {
		return
	}
	queueCapacity := min(cfg.QueueCapacity, cap(s.queue))
	if s.queuedCount.Add(1) > int64(queueCapacity) {
		s.queuedCount.Add(-1)
		s.dropped.Add(1)
		return
	}
	bodyBytes := int64(len(req.Body))
	if s.queuedBytes.Add(bodyBytes) > maxIngressQueueBytes {
		s.queuedCount.Add(-1)
		s.queuedBytes.Add(-bodyBytes)
		s.dropped.Add(1)
		return
	}
	request := req.Clone()
	select {
	case s.queue <- queuedRequest{request: request, bytes: bodyBytes}:
	default:
		s.queuedCount.Add(-1)
		s.queuedBytes.Add(-bodyBytes)
		s.dropped.Add(1)
	}
}

func (s *Service) consume(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case queued := <-s.queue:
			workCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := s.processCandidate(workCtx, queued.request); err != nil {
				s.failed.Add(1)
			} else {
				s.processed.Add(1)
			}
			cancel()
			s.queuedCount.Add(-1)
			s.queuedBytes.Add(-queued.bytes)
		}
	}
}

func (s *Service) processCandidate(ctx context.Context, req securityaudit.Request) error {
	cfg := s.config.Load()
	if cfg == nil || !cfg.Enabled || cfg.excludes(req.UserID, req.UserEmail) {
		return nil
	}
	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return err
	}
	now := time.Now()
	localDate := now.In(location).Format("2006-01-02")
	entropy := req.RequestID
	if entropy == "" {
		digest := sha256.Sum256(req.Body)
		entropy = hex.EncodeToString(digest[:])
	}
	compact := isCompactRequest(req)
	decision, err := s.sample(ctx, *cfg, req.UserID, localDate, now, entropy, compact)
	if err != nil || !decision.selected {
		return err
	}
	snapshot, err := securityaudit.ExtractLatestWorkInsightSnapshot(req, maxCapturedRequestBytes)
	if err != nil {
		_ = s.finishSample(ctx, req.UserID, localDate, decision, false)
		if errors.Is(err, securityaudit.ErrNoPromptText) {
			return nil
		}
		return err
	}
	conversationKey := s.conversationStateKey(req.UserID, req.ClientSessionID)
	lockToken := req.RequestID + ":" + strconv.FormatInt(now.UnixNano(), 10)
	if conversationKey != "" {
		locked, lockErr := s.redis.SetNX(ctx, conversationKey+":lock", lockToken, 10*time.Second).Result()
		if lockErr != nil || !locked {
			_ = s.finishSample(ctx, req.UserID, localDate, decision, false)
			return lockErr
		}
		defer func() {
			_ = s.redis.Eval(ctx, releaseConversationLockScript, []string{conversationKey + ":lock"}, lockToken).Err()
		}()
	}
	fullSegmentCount, fullPromptHash := snapshot.MessageCount, snapshot.PromptHash
	if conversationKey != "" {
		if state, stateErr := s.redis.Get(ctx, conversationKey).Result(); stateErr == nil {
			if separator := strings.IndexByte(state, ':'); separator > 0 {
				count, parseErr := strconv.Atoi(state[:separator])
				if parseErr == nil && snapshot.RetainAfterPrefix(count, state[separator+1:]) && snapshot.MessageCount == 0 {
					_ = s.finishSample(ctx, req.UserID, localDate, decision, false)
					return nil
				}
			}
		} else if !errors.Is(stateErr, redis.Nil) {
			_ = s.finishSample(ctx, req.UserID, localDate, decision, false)
			return stateErr
		}
	}
	fingerprintRaw := fmt.Sprintf("%d\x00%s\x00%s\x00%s", req.UserID, req.RequestID, decision.sessionID, snapshot.PromptHash)
	digest := sha256.Sum256([]byte(fingerprintRaw))
	date, _ := time.Parse("2006-01-02", localDate)
	preview := ""
	if cfg.StoreRedactedPreview {
		preview = securityaudit.RedactPreview(snapshot.Text, 96)
	}
	id, created, err := s.repo.CreateStaging(ctx, SampleInput{
		Fingerprint: hex.EncodeToString(digest[:]), RequestID: req.RequestID, UserID: req.UserID,
		Username: req.Username, UserEmail: req.UserEmail, APIKeyID: req.APIKeyID, GroupID: req.GroupID, Provider: req.Provider,
		Protocol: req.Protocol, Endpoint: req.Endpoint, Model: req.Model, LocalDate: date, SessionID: decision.sessionID,
		Reason: decision.reason, PromptHash: snapshot.PromptHash,
		EstimatedTokens: (snapshot.AnalyzedChars + 2) / 3, PromptChars: snapshot.PromptChars, AnalyzedChars: snapshot.AnalyzedChars,
		Truncated: snapshot.Truncated, RedactedPreview: preview,
	})
	if err != nil || !created {
		_ = s.finishSample(ctx, req.UserID, localDate, decision, false)
		return err
	}
	payloadTTL := time.Duration(cfg.PayloadTTLMinutes) * time.Minute
	payload, err := encodePayload(snapshot.Segments)
	if err != nil {
		_ = s.repo.Drop(ctx, id, "payload_encode_failed")
		_ = s.finishSample(ctx, req.UserID, localDate, decision, false)
		return err
	}
	if len(payload) > maxPayloadBytes {
		_ = s.repo.Drop(ctx, id, "payload_too_large")
		_ = s.finishSample(ctx, req.UserID, localDate, decision, false)
		return errors.New("work insight payload exceeds size limit")
	}
	reserved, err := s.reservePayloadSlot(ctx, id, len(payload), payloadTTL)
	if err != nil || !reserved {
		_ = s.repo.Drop(ctx, id, "payload_capacity")
		_ = s.finishSample(ctx, req.UserID, localDate, decision, false)
		if err != nil {
			return err
		}
		return errors.New("work insight payload capacity reached")
	}
	if err := s.redis.Set(ctx, PayloadKeyPrefix+strconv.FormatInt(id, 10), payload, payloadTTL).Err(); err != nil {
		s.releasePayloadSlots(ctx, []string{strconv.FormatInt(id, 10)})
		_ = s.repo.Drop(ctx, id, "payload_store_failed")
		_ = s.finishSample(ctx, req.UserID, localDate, decision, false)
		return err
	}
	if err := s.repo.Publish(ctx, id); err != nil {
		s.deletePayload(ctx, id)
		_ = s.repo.Drop(ctx, id, "queue_publish_failed")
		_ = s.finishSample(ctx, req.UserID, localDate, decision, false)
		return err
	}
	if duplicateIDs, cleanupErr := s.repo.DeleteOlderPendingDuplicates(ctx, id, req.UserID, date, snapshot.PromptHash); cleanupErr == nil {
		duplicates := make([]BatchSample, len(duplicateIDs))
		for index, duplicateID := range duplicateIDs {
			duplicates[index].ID = duplicateID
		}
		s.deletePayloads(ctx, duplicates)
	}
	if conversationKey != "" {
		_ = s.redis.Set(ctx, conversationKey, strconv.Itoa(fullSegmentCount)+":"+fullPromptHash, conversationStateTTL).Err()
	}
	return s.finishSample(ctx, req.UserID, localDate, decision, true)
}

func (s *Service) conversationStateKey(userID int64, clientSessionID string) string {
	clientSessionID = strings.TrimSpace(clientSessionID)
	if clientSessionID == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(strconv.FormatInt(userID, 10) + "\x00" + clientSessionID))
	return "sub2api:ai_work_insight:conversation:" + hex.EncodeToString(digest[:])
}

func (s *Service) reservePayloadSlot(ctx context.Context, id int64, bytes int, ttl time.Duration) (bool, error) {
	now := time.Now()
	result, err := s.redis.Eval(ctx, reservePayloadScript, []string{PayloadSlotsKey, PayloadSizesKey, PayloadBytesKey},
		now.Unix(), now.Add(ttl).Unix(), maxOutstandingPayloads, maxOutstandingPayloadBytes,
		strconv.FormatInt(id, 10), bytes, int(ttl/time.Second)).Int()
	return result == 1, err
}

func (s *Service) releasePayloadSlots(ctx context.Context, ids []string) {
	if len(ids) > 0 {
		_ = s.redis.Eval(ctx, releasePayloadScript, []string{PayloadSlotsKey, PayloadSizesKey, PayloadBytesKey}, stringSliceToAny(ids)...).Err()
	}
}

func (s *Service) deletePayload(ctx context.Context, id int64) {
	value := strconv.FormatInt(id, 10)
	if s.redis.Del(ctx, PayloadKeyPrefix+value).Err() == nil {
		s.releasePayloadSlots(ctx, []string{value})
	}
}

func stringSliceToAny(values []string) []any {
	result := make([]any, len(values))
	for i := range values {
		result[i] = values[i]
	}
	return result
}

type ingressSessionKey struct {
	userID int64
	date   string
}

func (s *Service) ingressSelected(cfg storedConfig, req securityaudit.Request, now time.Time) bool {
	if isCompactRequest(req) {
		return true
	}
	if cfg.SampleRate >= 100 {
		return true
	}
	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return false
	}
	date := now.In(location).Format("2006-01-02")
	state, _ := s.ingressSessions.LoadOrStore(ingressSessionKey{userID: req.UserID, date: date}, &atomic.Int64{})
	counter, ok := state.(*atomic.Int64)
	if !ok {
		return false
	}
	last := counter.Swap(now.UnixMilli())
	if last == 0 || now.UnixMilli()-last > int64(time.Duration(cfg.SessionIdleMinutes)*time.Minute/time.Millisecond) {
		return true
	}
	if cfg.SampleRate <= 0 {
		return false
	}
	entropy := req.RequestID
	if entropy == "" {
		digest := sha256.Sum256(req.Body)
		entropy = hex.EncodeToString(digest[:])
	}
	return selectedByRate(req.UserID, date, entropy, cfg.SampleRate)
}

func (s *Service) pruneIngressSessions(cutoff time.Time) {
	cutoffMS := cutoff.UnixMilli()
	s.ingressSessions.Range(func(key, value any) bool {
		counter, ok := value.(*atomic.Int64)
		if ok && counter.Load() < cutoffMS {
			s.ingressSessions.Delete(key)
		}
		return true
	})
}

func selectedByRate(userID int64, date, entropy string, rate int) bool {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", userID, date, entropy)))
	return int(uint16(digest[0])<<8|uint16(digest[1]))%100 < rate
}

type sampleDecision struct {
	sessionID     string
	selected      bool
	reason        string
	claimToken    string
	coverageClaim bool
}

func (s *Service) sample(ctx context.Context, cfg storedConfig, userID int64, date string, now time.Time, entropy string, compact bool) (sampleDecision, error) {
	prefix := "sub2api:ai_work_insight:" + date
	uid := strconv.FormatInt(userID, 10)
	keys := []string{
		prefix + ":session:" + uid,
		prefix + ":eligible:" + uid,
		prefix + ":sampled:" + uid,
		prefix + ":sampled:global",
		prefix + ":session_seq:" + uid,
	}
	claimRaw := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%d\x00%s", userID, date, now.UnixNano(), entropy)))
	claimToken := hex.EncodeToString(claimRaw[:16])
	rateSelected := "0"
	if selectedByRate(userID, date, entropy, cfg.SampleRate) {
		rateSelected = "1"
	}
	forcedReason := ""
	if compact {
		forcedReason = "compact"
	}
	result, err := s.redis.Eval(ctx, samplingScript, keys,
		now.UnixMilli(), int64(time.Duration(cfg.SessionIdleMinutes)*time.Minute/time.Millisecond),
		uid+":"+date, rateSelected, cfg.UserDailyLimit, cfg.GlobalDailyLimit,
		cfg.SampleRate, claimToken, int((72*time.Hour)/time.Second), forcedReason).Slice()
	if err != nil {
		return sampleDecision{}, err
	}
	if len(result) != 4 {
		return sampleDecision{}, errors.New("invalid sampling response")
	}
	reason := fmt.Sprint(result[3])
	return sampleDecision{sessionID: fmt.Sprint(result[0]), selected: fmt.Sprint(result[2]) == "1", reason: reason, claimToken: claimToken, coverageClaim: reason == "session_coverage"}, nil
}

func isCompactRequest(req securityaudit.Request) bool {
	endpoint := strings.TrimRight(strings.ToLower(strings.TrimSpace(req.Endpoint)), "/")
	return strings.HasSuffix(endpoint, "/responses/compact") || service.HasCompactionTriggerInInput(req.Body)
}

func (s *Service) finishSample(ctx context.Context, userID int64, date string, decision sampleDecision, success bool) error {
	if !decision.selected {
		return nil
	}
	prefix := "sub2api:ai_work_insight:" + date
	uid := strconv.FormatInt(userID, 10)
	_, err := s.redis.Eval(ctx, finishSamplingScript, []string{
		prefix + ":session:" + uid,
		prefix + ":sampled:" + uid,
		prefix + ":sampled:global",
	}, decision.claimToken, boolString(success), boolString(decision.coverageClaim)).Result()
	return err
}

func boolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func (s *Service) PublicConfig() Config {
	stored := s.config.Load()
	if stored == nil {
		return DefaultConfig()
	}
	cfg := stored.Config
	cfg.AnalyzerToken, cfg.AnalyzerTokenSet = "", stored.AnalyzerTokenCiphertext != ""
	return cfg
}

func (s *Service) AnalyzeNow(ctx context.Context) (int, error) {
	cfg := s.config.Load()
	if cfg == nil || !cfg.Enabled {
		return 0, infraerrors.BadRequest("ai_work_insight_disabled", "请先启用并保存 AI 使用洞察配置")
	}
	return s.repo.CreateDueBatches(ctx, time.Now(), cfg.Config, "manual")
}

func (s *Service) RetryBatchNow(ctx context.Context, batchID int64) error {
	cfg := s.config.Load()
	if cfg == nil || !cfg.Enabled {
		return infraerrors.BadRequest("ai_work_insight_disabled", "请先启用并保存 AI 使用洞察配置")
	}
	samples, err := s.repo.LoadRetryableBatchSamples(ctx, batchID)
	if err != nil {
		return err
	}
	if len(samples) == 0 {
		return infraerrors.BadRequest("ai_work_insight_batch_not_retryable", "该分析批次不是已停止状态，或关联样本已被清理")
	}
	keys := make([]string, len(samples))
	for index, sample := range samples {
		keys[index] = PayloadKeyPrefix + strconv.FormatInt(sample.ID, 10)
	}
	available, err := s.redis.Exists(ctx, keys...).Result()
	if err != nil {
		return err
	}
	if available != int64(len(keys)) {
		return infraerrors.BadRequest("ai_work_insight_payload_expired", "该批次的脱敏分析文本已过期，无法重新分析")
	}
	return s.repo.RequeueBatch(ctx, batchID, time.Now())
}

type RetryAllResult struct {
	Retried int `json:"retried"`
	Skipped int `json:"skipped"`
}

func (s *Service) RetryAllBatchesNow(ctx context.Context) (RetryAllResult, error) {
	cfg := s.config.Load()
	if cfg == nil || !cfg.Enabled {
		return RetryAllResult{}, infraerrors.BadRequest("ai_work_insight_disabled", "请先启用并保存 AI 使用洞察配置")
	}
	candidates, err := s.repo.ListRetryBatchCandidates(ctx)
	if err != nil || len(candidates) == 0 {
		return RetryAllResult{}, err
	}
	pipe := s.redis.Pipeline()
	checks := make([]*redis.IntCmd, len(candidates))
	for index, candidate := range candidates {
		keys := make([]string, len(candidate.SampleIDs))
		for sampleIndex, sampleID := range candidate.SampleIDs {
			keys[sampleIndex] = PayloadKeyPrefix + strconv.FormatInt(sampleID, 10)
		}
		checks[index] = pipe.Exists(ctx, keys...)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return RetryAllResult{}, err
	}
	ids := make([]int64, 0, len(candidates))
	for index, candidate := range candidates {
		if checks[index].Val() == int64(len(candidate.SampleIDs)) {
			ids = append(ids, candidate.ID)
		}
	}
	retried, err := s.repo.RequeueBatches(ctx, ids, time.Now())
	if err != nil {
		return RetryAllResult{}, err
	}
	return RetryAllResult{Retried: int(retried), Skipped: len(candidates) - int(retried)}, nil
}

func (s *Service) StopBatchNow(ctx context.Context, batchID int64) error {
	stopped, err := s.repo.StopBatch(ctx, batchID, time.Now())
	if err != nil {
		return err
	}
	if !stopped {
		return infraerrors.BadRequest("ai_work_insight_batch_not_stoppable", "该分析任务已经结束，无法停止")
	}
	if value, ok := s.batchCancels.Load(batchID); ok {
		if cancel, valid := value.(context.CancelFunc); valid {
			cancel()
		}
	}
	return nil
}

func (s *Service) DeleteFailedBatchNow(ctx context.Context, batchID int64) error {
	samples, err := s.repo.DeleteFailedBatch(ctx, batchID)
	if err != nil {
		return err
	}
	s.deletePayloads(ctx, samples)
	return nil
}

func (s *Service) DeleteAllFailedBatchesNow(ctx context.Context) (int64, error) {
	samples, deleted, err := s.repo.DeleteAllFailedBatches(ctx)
	if err != nil {
		return 0, err
	}
	s.deletePayloads(ctx, samples)
	return deleted, nil
}

func (s *Service) SaveConfig(ctx context.Context, next Config, actorID int64) (Config, error) {
	current := s.config.Load()
	if current == nil || next.ConfigVersion != current.ConfigVersion {
		return Config{}, infraerrors.Conflict("ai_work_insight_config_conflict", "AI 使用洞察配置已被其他管理员更新")
	}
	next.normalize()
	next.ConfigVersion++
	next.UpdatedAt, next.UpdatedBy = time.Now().UTC(), actorID
	next.AnalyzerTokenSet = current.AnalyzerTokenCiphertext != "" || next.AnalyzerToken != ""
	if err := next.validate(); err != nil {
		return Config{}, infraerrors.BadRequest("ai_work_insight_invalid_config", err.Error())
	}
	if next.AnalyzerSource == "account" && next.AnalyzerAccountID > 0 {
		if _, err := s.openAIAnalyzerAccount(ctx, next.AnalyzerAccountID); err != nil {
			return Config{}, infraerrors.BadRequest("ai_work_insight_invalid_analyzer_account", "分析账号必须是账号管理中可用的 OpenAI 平台账号")
		}
	}
	stored := storedConfig{Config: next, AnalyzerTokenCiphertext: current.AnalyzerTokenCiphertext}
	if next.AnalyzerToken != "" {
		if s.encryptor == nil {
			return Config{}, errors.New("secret encryptor unavailable")
		}
		ciphertext, err := s.encryptor.Encrypt(next.AnalyzerToken)
		if err != nil {
			return Config{}, err
		}
		stored.AnalyzerTokenCiphertext = ciphertext
	}
	stored.AnalyzerToken, stored.AnalyzerTokenSet = "", stored.AnalyzerTokenCiphertext != ""
	raw, err := json.Marshal(stored)
	if err != nil {
		return Config{}, err
	}
	if s.repo == nil {
		return Config{}, errors.New("work insight database unavailable")
	}
	if err := s.repo.SaveConfigCAS(ctx, string(raw), current.ConfigVersion); err != nil {
		if errors.Is(err, ErrConfigConflict) {
			return Config{}, infraerrors.Conflict("ai_work_insight_config_conflict", "AI 使用洞察配置已被其他管理员更新")
		}
		return Config{}, err
	}
	s.config.Store(&stored)
	if s.redis != nil {
		_ = s.redis.Publish(ctx, ConfigInvalidationChannel, next.ConfigVersion).Err()
	}
	return s.PublicConfig(), nil
}

func (s *Service) configRefresh(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	if s.redis == nil {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				reloadCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				_ = s.reload(reloadCtx)
				cancel()
			}
		}
	}
	pubsub := s.redis.Subscribe(ctx, ConfigInvalidationChannel)
	defer func() { _ = pubsub.Close() }()
	channel := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case _, ok := <-channel:
			if !ok {
				channel = nil
			}
		}
		reloadCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		_ = s.reload(reloadCtx)
		cancel()
	}
}

func (s *Service) reload(ctx context.Context) error {
	if s.settings == nil {
		return errors.New("settings unavailable")
	}
	raw, err := s.settings.GetValue(ctx, SettingKey)
	if err != nil {
		return nil // absent settings keep safe default-off configuration
	}
	var stored storedConfig
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return err
	}
	stored.normalize()
	stored.AnalyzerTokenSet = stored.AnalyzerTokenCiphertext != ""
	if err := stored.validate(); err != nil {
		return err
	}
	s.config.Store(&stored)
	return nil
}

func (s *Service) Runtime(ctx context.Context) (Runtime, error) {
	cfg := s.config.Load()
	runtime := Runtime{QueueDepth: int(s.queuedCount.Load()), QueueCapacity: cap(s.queue), QueueBytes: s.queuedBytes.Load(), QueueByteCapacity: maxIngressQueueBytes, Dropped: s.dropped.Load(), Processed: s.processed.Load(), Failed: s.failed.Load()}
	if cfg != nil {
		runtime.Enabled, runtime.QueueCapacity = cfg.Enabled, min(cfg.QueueCapacity, cap(s.queue))
		location, err := time.LoadLocation(cfg.Timezone)
		if err != nil {
			return runtime, err
		}
		date, _ := time.Parse("2006-01-02", time.Now().In(location).Format("2006-01-02"))
		stats, err := s.repo.RuntimeStats(ctx, date)
		if err != nil {
			return runtime, err
		}
		stats.Enabled, stats.QueueDepth, stats.QueueCapacity = runtime.Enabled, runtime.QueueDepth, runtime.QueueCapacity
		stats.QueueBytes, stats.QueueByteCapacity = runtime.QueueBytes, runtime.QueueByteCapacity
		stats.Dropped, stats.Processed, stats.Failed = runtime.Dropped, runtime.Processed, runtime.Failed
		runtime = stats
	}
	return runtime, nil
}

func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil || s.cancel == nil {
		return nil
	}
	s.cancel()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) AnalyzerToken() (string, error) {
	cfg := s.config.Load()
	if cfg == nil || cfg.AnalyzerTokenCiphertext == "" {
		return "", nil
	}
	if s.encryptor == nil {
		return "", errors.New("secret encryptor unavailable")
	}
	return s.encryptor.Decrypt(strings.TrimSpace(cfg.AnalyzerTokenCiphertext))
}
