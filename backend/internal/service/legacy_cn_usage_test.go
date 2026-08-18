package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLegacyCNUsageParsersRestoreWindowsAndBalance(t *testing.T) {
	glm, err := parseLegacyGLMUsage([]byte(`{"code":0,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"percentage":25},{"type":"TOKENS_LIMIT","unit":6,"percentage":75}]}}`))
	require.NoError(t, err)
	require.Equal(t, 25.0, glm.FiveHour.Utilization)
	require.Equal(t, 75.0, glm.SevenDay.Utilization)

	kimi, err := parseLegacyKimiUsage([]byte(`{"usage":{"limit":100,"remaining":20},"limits":[{"window":{"duration":5,"timeUnit":"HOUR"},"detail":{"limit":50,"remaining":40}}]}`))
	require.NoError(t, err)
	require.Equal(t, 20.0, kimi.FiveHour.Utilization)
	require.Equal(t, 80.0, kimi.SevenDay.Utilization)

	require.True(t, isLegacyCNUsageAccount(&Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: map[string]any{"model_provider": "deepseek"}}))
}
