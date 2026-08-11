package workinsight

import (
	"errors"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestCleanupHasMoreWhenAnyTableFillsBatch(t *testing.T) {
	require.False(t, cleanupHasMore(CleanupResult{Samples: 4999, Batches: 2, Daily: 1}, 5000))
	require.True(t, cleanupHasMore(CleanupResult{Samples: 5000}, 5000))
}

func TestJSONValueKeepsNilSlicesAsArrays(t *testing.T) {
	require.JSONEq(t, `[]`, string(jsonValue([]string(nil))))
	require.JSONEq(t, `{}`, string(jsonValue(map[string]int{})))
}

func TestSummaryWriteErrorCodeExplainsKnownFailures(t *testing.T) {
	require.Equal(t, "summary_write_conflict", summaryWriteErrorCode(errors.New("work insight claim lost")))
	require.Equal(t, "summary_json_invalid", summaryWriteErrorCode(&pq.Error{Constraint: "chk_ai_work_insight_batches_json"}))
	require.Equal(t, "summary_write_failed", summaryWriteErrorCode(errors.New("disk unavailable")))
}

func TestAnalyzerPauseExpires(t *testing.T) {
	service := &Service{}
	now := time.Now()
	service.pauseAnalyzer(now)
	require.True(t, service.analyzerPaused(now))
	require.False(t, service.analyzerPaused(now.Add(31*time.Second)))
}

func TestBatchTriggerModesAndBoundaries(t *testing.T) {
	cfg := DefaultConfig()
	now := time.Date(2026, 8, 11, 2, 0, 0, 0, time.UTC)
	samples := []Sample{{ID: 1, EstimatedTokens: 100, CreatedAt: now.Add(-15 * time.Minute)}}
	require.Equal(t, "idle", batchTrigger(samples, now, cfg, ""))

	samples = []Sample{{ID: 1, EstimatedTokens: 100, CreatedAt: now.Add(-60 * time.Minute)}, {ID: 2, EstimatedTokens: 100, CreatedAt: now}}
	require.Equal(t, "max_wait", batchTrigger(samples, now, cfg, ""))

	cfg.AnalysisTriggerMode = "fixed_interval"
	cfg.FixedIntervalMinutes = 10
	require.Equal(t, "fixed", batchTrigger([]Sample{{CreatedAt: now.Add(-10 * time.Minute)}}, now, cfg, ""))
	cfg.AnalysisTriggerMode = "fixed_time"
	require.Equal(t, "fixed", batchTrigger([]Sample{{CreatedAt: now}}, now, cfg, "fixed"))
	cfg.MaxSamplesPerBatch = 1
	require.Equal(t, "manual", batchTrigger([]Sample{{CreatedAt: now}}, now, cfg, "manual"))
}

func TestSelectBatchSamplesKeepsOneOversizeAndPrefix(t *testing.T) {
	selected := selectBatchSamples([]Sample{{ID: 1, EstimatedTokens: 20}, {ID: 2, EstimatedTokens: 5}}, 20, 10)
	require.Equal(t, []int64{1}, []int64{selected[0].ID})
	selected = selectBatchSamples([]Sample{{ID: 1, EstimatedTokens: 4}, {ID: 2, EstimatedTokens: 4}, {ID: 3, EstimatedTokens: 4}}, 20, 9)
	require.Equal(t, []int64{1, 2}, []int64{selected[0].ID, selected[1].ID})
}
