//go:build integration

package workinsight

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func TestWorkInsightCandidateToDailyInsight(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("docker is required for integration tests: %v", err)
		}
		t.Skip("docker is unavailable")
	}
	ctx := context.Background()
	postgres, err := tcpostgres.Run(ctx, "postgres:18.1-alpine3.23", tcpostgres.WithDatabase("sub2api_test"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = postgres.Terminate(ctx) })
	dsn, err := postgres.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.Eventually(t, func() bool { return db.PingContext(ctx) == nil }, 20*time.Second, 200*time.Millisecond)
	require.NoError(t, repository.ApplyMigrations(ctx, db))

	redisContainer, err := tcredis.Run(ctx, "redis:8.4-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = redisContainer.Terminate(ctx) })
	host, err := redisContainer.Host(ctx)
	require.NoError(t, err)
	port, err := redisContainer.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)
	rdb := redis.NewClient(&redis.Options{Addr: fmt.Sprintf("%s:%d", host, port.Int())})
	t.Cleanup(func() { _ = rdb.Close() })
	require.NoError(t, rdb.Ping(ctx).Err())

	analyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `{"work_summary":"使用 sub2api 排查网关问题。","task_categories":["问题排查"],"explicit_projects":["sub2api"],"explicit_modules":["网关"],"change_types":["Bug 修复"],"business_topics":["路由"],"representative_items":[{"source_sample_ids":[1],"summary":"排查网关问题。","task_categories":["问题排查"],"explicit_projects":["sub2api"],"explicit_modules":["网关"]}],"evidence_level":"explicit"}`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}, "usage": map[string]int{"prompt_tokens": 30, "completion_tokens": 20}})
	}))
	defer analyzer.Close()

	var userID int64
	require.NoError(t, db.QueryRow(`INSERT INTO users(email,password_hash,role) VALUES ('work-insight-canary@example.test','x','user') RETURNING id`).Scan(&userID))
	repo := NewRepository(db)
	configV2 := DefaultConfig()
	configV2.ConfigVersion = 2
	configRaw, err := json.Marshal(storedConfig{Config: configV2})
	require.NoError(t, err)
	require.NoError(t, repo.SaveConfigCAS(ctx, string(configRaw), 1))
	require.ErrorIs(t, repo.SaveConfigCAS(ctx, string(configRaw), 1), ErrConfigConflict)
	cfg := DefaultConfig()
	cfg.Enabled, cfg.AnalyzerSource, cfg.AnalyzerBaseURL, cfg.AnalyzerToken, cfg.AnalyzerModel = true, "custom", analyzer.URL, "token-canary", "model-canary"
	cfg.MaxSamplesPerBatch = 1
	stored := storedConfig{Config: cfg}
	service := &Service{repo: repo, redis: rdb}
	service.config.Store(&stored)
	require.NoError(t, service.processCandidate(ctx, securityaudit.Request{
		RequestID: "request-canary", UserID: userID, Username: "测试用户", UserEmail: "work-insight-canary@example.test",
		Provider: "openai", Protocol: "openai_chat", Endpoint: "/v1/chat/completions", Model: "model-canary",
		Body: []byte(`{"messages":[{"role":"user","content":"请使用 sub2api 排查网关问题"}]}`),
	}))
	created, err := repo.CreateDueBatches(ctx, time.Now(), cfg, "")
	require.NoError(t, err)
	require.Equal(t, 1, created)
	var sampleID int64
	require.NoError(t, db.QueryRow(`SELECT id FROM ai_work_insight_samples WHERE request_id='request-canary'`).Scan(&sampleID))
	service.processNextBatch(ctx, stored)

	var summary string
	var retained int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM ai_work_insight_samples WHERE request_id='request-canary'`).Scan(&retained))
	require.Zero(t, retained)
	require.Equal(t, int64(0), rdb.Exists(ctx, PayloadKeyPrefix+fmt.Sprint(sampleID)).Val())
	require.NoError(t, db.QueryRow(`SELECT daily_summary FROM ai_user_daily_work_insights WHERE user_id=$1`, userID).Scan(&summary))
	require.Contains(t, summary, "sub2api")
	var insightDate time.Time
	require.NoError(t, db.QueryRow(`SELECT insight_date FROM ai_user_daily_work_insights WHERE user_id=$1`, userID).Scan(&insightDate))
	coverageKey := "sub2api:ai_work_insight:" + insightDate.Format("2006-01-02") + ":eligible:" + fmt.Sprint(userID)
	require.NoError(t, rdb.Set(ctx, coverageKey, 2, time.Hour).Err())
	service.runFinalize(ctx, insightDate, stored)
	service.runFinalize(ctx, insightDate, stored)
	var eligible, covered int
	require.NoError(t, db.QueryRow(`SELECT eligible_active_session_count,covered_active_session_count FROM ai_user_daily_work_insights WHERE user_id=$1 AND insight_date=$2`, userID, insightDate).Scan(&eligible, &covered))
	require.Equal(t, 2, eligible)
	require.Equal(t, 1, covered)

	var staleBatchID int64
	require.NoError(t, db.QueryRow(`INSERT INTO ai_work_insight_batches
		(user_id,username_snapshot,local_date,active_session_id,first_sample_id,last_sample_id,sample_count,trigger_reason,status,claim_version,updated_at)
		VALUES ($1,'测试用户',$2,'stale-session',900001,900001,1,'manual','processing',1,NOW()-INTERVAL '2 minutes') RETURNING id`, userID, insightDate).Scan(&staleBatchID))
	reclaimed, ok, err := repo.ClaimBatch(ctx, time.Now(), time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, staleBatchID, reclaimed.ID)
	require.Equal(t, int64(2), reclaimed.ClaimVersion)
	require.Error(t, repo.RetryBatch(ctx, Batch{ID: staleBatchID, ClaimVersion: 1}, time.Now(), "stale-worker"), "old worker must be fenced after lease recovery")
	require.NoError(t, db.QueryRow(`UPDATE ai_user_daily_work_insights SET finalized_at=NULL WHERE user_id=$1 AND insight_date=$2 RETURNING id`, userID, insightDate).Scan(new(int64)))
	finalized, err := repo.Finalize(ctx, insightDate)
	require.NoError(t, err)
	require.Zero(t, finalized, "a processing batch must keep the day open")
	pending, err := repo.HasOpenBatches(ctx, insightDate)
	require.NoError(t, err)
	require.True(t, pending)
	_, err = repo.DropBatch(ctx, *reclaimed, "test_cleanup")
	require.NoError(t, err)
	finalized, err = repo.Finalize(ctx, insightDate)
	require.NoError(t, err)
	require.Equal(t, int64(1), finalized)
	pending, err = repo.HasOpenBatches(ctx, insightDate)
	require.NoError(t, err)
	require.False(t, pending)
	require.NoError(t, db.QueryRow(`INSERT INTO ai_work_insight_samples
		(request_fingerprint,user_id,local_date,active_session_id,sample_reason,prompt_hash,status)
		VALUES ('pending-finalize-canary',$1,$2,'pending-finalize-session','rate','pending-finalize-hash','pending_batch') RETURNING id`, userID, insightDate).Scan(new(int64)))
	pending, err = repo.HasOpenBatches(ctx, insightDate)
	require.NoError(t, err)
	require.True(t, pending, "an unbatched sample must keep the day open")
	require.NoError(t, db.QueryRow(`DELETE FROM ai_work_insight_samples WHERE request_fingerprint='pending-finalize-canary' RETURNING id`).Scan(new(int64)))

	var earlierID, laterID int64
	require.NoError(t, db.QueryRow(`INSERT INTO ai_work_insight_batches
		(user_id,username_snapshot,local_date,active_session_id,first_sample_id,last_sample_id,sample_count,trigger_reason,status,next_attempt_at)
		VALUES ($1,'测试用户',$2,'ordered-1',910001,910001,1,'manual','retry',NOW()+INTERVAL '1 hour') RETURNING id`, userID, insightDate).Scan(&earlierID))
	require.NoError(t, db.QueryRow(`INSERT INTO ai_work_insight_batches
		(user_id,username_snapshot,local_date,active_session_id,first_sample_id,last_sample_id,sample_count,trigger_reason,status)
		VALUES ($1,'测试用户',$2,'ordered-2',910002,910002,1,'manual','queued') RETURNING id`, userID, insightDate).Scan(&laterID))
	_, ok, err = repo.ClaimBatch(ctx, time.Now(), time.Minute)
	require.NoError(t, err)
	require.False(t, ok, "a later watermark must wait while an earlier batch is retrying")
	require.NoError(t, db.QueryRow(`UPDATE ai_work_insight_batches SET next_attempt_at=NOW() WHERE id=$1 RETURNING id`, earlierID).Scan(new(int64)))
	ordered, ok, err := repo.ClaimBatch(ctx, time.Now(), time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, earlierID, ordered.ID)
	_, err = repo.DropBatch(ctx, *ordered, "test_cleanup")
	require.NoError(t, err)
	ordered, ok, err = repo.ClaimBatch(ctx, time.Now(), time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, laterID, ordered.ID)
	_, err = repo.DropBatch(ctx, *ordered, "test_cleanup")
	require.NoError(t, err)

	var expiredSampleID int64
	require.NoError(t, db.QueryRow(`INSERT INTO ai_work_insight_samples
		(request_fingerprint,request_id,user_id,local_date,active_session_id,sample_reason,prompt_hash,status,created_at)
		VALUES ('expired-canary','expired-request',$1,$2,'expired-session','rate','expired-hash','pending_batch',NOW()-INTERVAL '2 hours') RETURNING id`, userID, insightDate).Scan(&expiredSampleID))
	require.NoError(t, rdb.Set(ctx, PayloadKeyPrefix+fmt.Sprint(expiredSampleID), "expired canary", time.Hour).Err())
	expired, err := repo.DropExpiredSamples(ctx, time.Now().Add(-90*time.Minute))
	require.NoError(t, err)
	require.Len(t, expired, 1)
	service.deletePayloads(ctx, expired)
	require.Zero(t, rdb.Exists(ctx, PayloadKeyPrefix+fmt.Sprint(expiredSampleID)).Val())

	runtime, err := repo.RuntimeStats(ctx, insightDate)
	require.NoError(t, err)
	require.GreaterOrEqual(t, runtime.DoneBatches, int64(1))
	require.Greater(t, runtime.AnalyzerInputTokens, int64(0))
	var rawColumns int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name LIKE 'ai_work_insight%' AND column_name IN ('prompt_raw','response_raw','full_prompt','raw_request')`).Scan(&rawColumns))
	require.Zero(t, rawColumns)
}
