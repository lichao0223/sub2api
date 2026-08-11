package workinsight

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigrationStoresMetadataAndStructuredResultsOnly(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/913_ai_work_insights.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(raw))
	for _, forbidden := range []string{"prompt_raw", "response_raw", "full_prompt", "raw_request", "analyzer_response"} {
		require.NotContains(t, sql, forbidden)
	}
	for _, required := range []string{"request_fingerprint", "active_session_id", "representative_items", "eligible_active_session_count", "covered_active_session_count", "group_id", "protocol", "model_usage", "business_input_tokens", "business_error_count", "p95_duration_ms", "business_topics"} {
		require.Contains(t, sql, required)
	}
	for _, unusedIndex := range []string{"idx_ai_work_insight_samples_schedule", "idx_ai_work_insight_samples_date", "idx_ai_work_insight_samples_group", "idx_ai_work_insight_batches_categories", "idx_ai_work_insight_batches_projects"} {
		require.NotContains(t, sql, unusedIndex)
	}
}

func TestPerformanceMigrationUpgradesExistingCapacity(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/914_ai_work_insight_performance.sql")
	require.NoError(t, err)
	sql := string(raw)
	for _, required := range []string{"'user_daily_limit'", "5000", "'global_daily_limit'", "200000", "'analysis_idle_minutes'", "15", "'analysis_max_wait_minutes'", "60", "'max_input_tokens'", "64000"} {
		require.Contains(t, sql, required)
	}
}
