package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGatewayInputTokenLimitScope(t *testing.T) {
	groupID, otherID := int64(7), int64(8)
	for _, tc := range []struct {
		name    string
		enabled string
		groups  string
		groupID *int64
		want    int
	}{
		{name: "default enabled for ungrouped", want: 12345},
		{name: "default enabled for groups", groupID: &groupID, want: 12345},
		{name: "disabled", enabled: "false", groups: "[7]", groupID: &groupID},
		{name: "selected", enabled: "true", groups: "[7]", groupID: &groupID, want: 12345},
		{name: "other group", groups: "[7]", groupID: &otherID},
		{name: "ungrouped excluded", groups: "[7]"},
		{name: "empty selects all", groups: "[]", groupID: &otherID, want: 12345},
		{name: "malformed scope fails closed", groups: "[7,", groupID: &otherID, want: 12345},
		{name: "invalid IDs fail closed", groups: "[-1]", groupID: &otherID, want: 12345},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{
				SettingKeyGatewayMaxInputTokens:          "12345",
				SettingKeyGatewayInputTokenLimitEnabled:  tc.enabled,
				SettingKeyGatewayInputTokenLimitGroupIDs: tc.groups,
			}
			svc := NewSettingService(&contentModerationTestSettingRepo{values: values}, &config.Config{})
			for range 2 {
				require.Equal(t, tc.want, svc.GetGatewayInputTokenLimit(context.Background(), tc.groupID))
			}
			view := svc.parseSettings(values)
			require.Equal(t, tc.enabled != "false", view.GatewayInputTokenLimitEnabled)
			require.NotNil(t, view.GatewayInputTokenLimitGroupIDs)
		})
	}
}

type failingInputTokenLimitRepo struct{ SettingRepository }

func (failingInputTokenLimitRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("database unavailable")
}

func TestGatewayInputTokenLimitDatabaseFailureUsesConfiguredDefault(t *testing.T) {
	svc := NewSettingService(failingInputTokenLimitRepo{}, &config.Config{
		Gateway: config.GatewayConfig{MaxInputTokens: 12345},
	})
	require.Equal(t, 12345, svc.GetGatewayInputTokenLimit(context.Background(), nil))
}
