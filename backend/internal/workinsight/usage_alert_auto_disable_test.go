package workinsight

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type authCacheInvalidatorStub struct{ keys []string }

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByKey(_ context.Context, key string) {
	s.keys = append(s.keys, key)
}
func (s *authCacheInvalidatorStub) InvalidateAuthCacheByUserID(context.Context, int64)  {}
func (s *authCacheInvalidatorStub) InvalidateAuthCacheByGroupID(context.Context, int64) {}

func TestUsageAlertAutoDisableUpdatesQualifiedKeysAndInvalidatesCache(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)WITH candidate_keys AS .*UPDATE api_keys SET status=\$8`).
		WithArgs(now.Add(-usageAlertLookback), 300000, 3, sqlmock.AnyArg(), "active", "active", "admin", "disabled").
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "key"}).AddRow(12, 8, "sk-secret"))
	invalidator := &authCacheInvalidatorStub{}
	svc := &Service{repo: NewRepository(db), authCache: invalidator}
	cfg := storedConfig{Config: DefaultConfig()}
	cfg.UsageAlertEnabled = true
	cfg.UsageAlertAutoDisableEnabled = true
	cfg.UsageAlertInputTokens = 300000
	cfg.UsageAlertConsecutiveCount = 3
	cfg.UsageAlertExemptUserIDs = []int64{5}

	svc.runUsageAlertAutoDisable(context.Background(), now, cfg)

	require.Equal(t, []string{"sk-secret"}, invalidator.keys)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageAlertAutoDisableSkipsWhenDisabled(t *testing.T) {
	svc := &Service{}
	cfg := storedConfig{Config: DefaultConfig()}
	cfg.UsageAlertEnabled = true

	svc.runUsageAlertAutoDisable(context.Background(), time.Now(), cfg)
}
