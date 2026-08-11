package workinsight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCleanupHasMoreWhenAnyTableFillsBatch(t *testing.T) {
	require.False(t, cleanupHasMore(CleanupResult{Samples: 4999, Batches: 2, Daily: 1}, 5000))
	require.True(t, cleanupHasMore(CleanupResult{Samples: 5000}, 5000))
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
	require.Equal(t, "idle", batchTrigger(samples, now, cfg, false))

	samples = []Sample{{ID: 1, EstimatedTokens: 100, CreatedAt: now.Add(-60 * time.Minute)}, {ID: 2, EstimatedTokens: 100, CreatedAt: now}}
	require.Equal(t, "max_wait", batchTrigger(samples, now, cfg, false))

	cfg.AnalysisTriggerMode = "fixed_interval"
	cfg.FixedIntervalMinutes = 10
	require.Equal(t, "fixed", batchTrigger([]Sample{{CreatedAt: now.Add(-10 * time.Minute)}}, now, cfg, false))
	cfg.AnalysisTriggerMode = "fixed_time"
	require.Equal(t, "fixed", batchTrigger([]Sample{{CreatedAt: now}}, now, cfg, true))
}

func TestSelectBatchSamplesKeepsOneOversizeAndPrefix(t *testing.T) {
	selected := selectBatchSamples([]Sample{{ID: 1, EstimatedTokens: 20}, {ID: 2, EstimatedTokens: 5}}, 20, 10)
	require.Equal(t, []int64{1}, []int64{selected[0].ID})
	selected = selectBatchSamples([]Sample{{ID: 1, EstimatedTokens: 4}, {ID: 2, EstimatedTokens: 4}, {ID: 3, EstimatedTokens: 4}}, 20, 9)
	require.Equal(t, []int64{1, 2}, []int64{selected[0].ID, selected[1].ID})
}
