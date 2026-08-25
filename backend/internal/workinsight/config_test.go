package workinsight

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultConfigMatchesProductContract(t *testing.T) {
	cfg := DefaultConfig()
	require.NoError(t, cfg.validate())
	require.Equal(t, 20, cfg.SampleRate)
	require.Equal(t, 5000, cfg.UserDailyLimit)
	require.Equal(t, 200000, cfg.GlobalDailyLimit)
	require.Equal(t, 50, cfg.MaxSamplesPerBatch)
	require.Equal(t, 64000, cfg.MaxInputTokens)
	require.Equal(t, 60, cfg.AnalysisTimeoutSeconds)
	require.Equal(t, 15, cfg.AnalysisIdleMinutes)
	require.Equal(t, 60, cfg.AnalysisMaxWaitMinutes)
	require.Equal(t, 10000, cfg.QueueCapacity)
	require.Equal(t, 30, cfg.SampleRetentionDays)
	require.Equal(t, 180, cfg.InsightRetentionDays)
	require.Equal(t, "hybrid", cfg.AnalysisTriggerMode)
	require.Equal(t, []string{
		"代码开发", "问题排查", "测试用例", "接口文档", "需求分析", "方案设计", "数据分析", "SQL/报表",
		"运维部署", "日志分析", "文档写作", "翻译润色", "会议纪要", "客服支持", "培训学习", "咨询", "其他",
	}, TaskCategories)
}

func TestConfigRejectsUnsafeContextAndLifecycle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.GlobalDailyLimit = 500001
	require.ErrorContains(t, cfg.validate(), "sampling limits")

	cfg = DefaultConfig()
	cfg.MaxInputTokens = cfg.ContextWindowTokens
	require.ErrorContains(t, cfg.validate(), "analysis lifecycle")

	cfg = DefaultConfig()
	cfg.PayloadTTLMinutes = cfg.MaxJobAgeMinutes + 29
	require.ErrorContains(t, cfg.validate(), "analysis lifecycle")

	cfg = DefaultConfig()
	cfg.InsightRetentionDays = cfg.SampleRetentionDays - 1
	require.ErrorContains(t, cfg.validate(), "retention")
}

func TestConfigNormalizeBackfillsNewFields(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AnalysisTriggerMode = ""
	cfg.ContextWindowTokens = 0
	cfg.ReservedOutputTokens = 0
	cfg.DailyFinalizeTime = ""
	cfg.CleanupBatchSize = 0
	cfg.normalize()
	require.NoError(t, cfg.validate())
	require.Equal(t, 128000, cfg.ContextWindowTokens)
	require.Equal(t, "00:15", cfg.DailyFinalizeTime)
}
