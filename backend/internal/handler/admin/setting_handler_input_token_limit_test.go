//go:build unit

package admin

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateSettingsInputTokenLimitSwitchAndGroups(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyGatewayMaxInputTokens:          "10",
		service.SettingKeyGatewayInputTokenLimitEnabled:  "true",
		service.SettingKeyGatewayInputTokenLimitGroupIDs: "[7]",
	})
	ctx := context.Background()
	selected, other := int64(7), int64(8)
	require.Equal(t, 10, h.settingService.GetGatewayInputTokenLimit(ctx, &selected))
	require.Zero(t, h.settingService.GetGatewayInputTokenLimit(ctx, &other))

	rec := doUpdateSettings(t, h, map[string]any{"gateway_input_token_limit_enabled": false}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"gateway_input_token_limit_enabled":false`)
	require.Equal(t, "false", repo.values[service.SettingKeyGatewayInputTokenLimitEnabled])
	require.JSONEq(t, "[7]", repo.values[service.SettingKeyGatewayInputTokenLimitGroupIDs])
	require.Zero(t, h.settingService.GetGatewayInputTokenLimit(ctx, &selected), "local saves refresh the cache immediately")

	rec = doUpdateSettings(t, h, map[string]any{"site_name": "Example"}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyGatewayInputTokenLimitEnabled])
	require.Zero(t, h.settingService.GetGatewayInputTokenLimit(ctx, &selected))

	rec = doUpdateSettings(t, h, map[string]any{
		"gateway_input_token_limit_enabled": true, "gateway_input_token_limit_group_ids": []int64{8},
	}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"gateway_input_token_limit_group_ids":[8]`)
	require.Zero(t, h.settingService.GetGatewayInputTokenLimit(ctx, &selected))
	require.Equal(t, 10, h.settingService.GetGatewayInputTokenLimit(ctx, &other))

	rec = doUpdateSettings(t, h, map[string]any{"gateway_input_token_limit_group_ids": []int64{}}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, "[]", repo.values[service.SettingKeyGatewayInputTokenLimitGroupIDs])
	require.Equal(t, 10, h.settingService.GetGatewayInputTokenLimit(ctx, nil))
	require.Equal(t, 10, h.settingService.GetGatewayInputTokenLimit(ctx, &selected))
}

func TestUpdateSettingsRejectsInvalidInputTokenLimitGroups(t *testing.T) {
	for _, id := range []int64{0, -1} {
		h, repo := newStepUpSwitchTestHandler(t, map[string]string{})
		rec := doUpdateSettings(t, h, map[string]any{"gateway_input_token_limit_group_ids": []int64{id}}, nil)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Contains(t, rec.Body.String(), "must contain positive IDs")
		require.NotContains(t, repo.values, service.SettingKeyGatewayInputTokenLimitGroupIDs)
	}
}
