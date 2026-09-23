package workinsight

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type authCacheInvalidatorStub struct {
	keys    []string
	userIDs []int64
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByKey(_ context.Context, key string) {
	s.keys = append(s.keys, key)
}
func (s *authCacheInvalidatorStub) InvalidateAuthCacheByUserID(_ context.Context, id int64) {
	s.userIDs = append(s.userIDs, id)
}
func (s *authCacheInvalidatorStub) InvalidateAuthCacheByGroupID(context.Context, int64) {}

func TestUsageAlertAutoDisableUpdatesQualifiedKeysAndInvalidatesCache(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	dayStart := time.Date(2026, 9, 7, 0, 0, 0, 0, location).UTC()
	mock.ExpectQuery(`(?s)WITH qualified_users AS .*UPDATE users SET status=\$8`).
		WithArgs(dayStart, dayStart.AddDate(0, 0, 1), 300000, 3, "active", "admin", sqlmock.AnyArg(), "disabled").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(8))
	invalidator := &authCacheInvalidatorStub{}
	svc := &Service{repo: NewRepository(db), authCache: invalidator}
	cfg := storedConfig{Config: DefaultConfig()}
	cfg.UsageAlertEnabled = true
	cfg.UsageAlertAutoDisableEnabled = true
	cfg.UsageAlertInputTokens = 300000
	cfg.UsageAlertConsecutiveCount = 3
	cfg.UsageAlertExemptUserIDs = []int64{5}

	svc.runUsageAlertAutoDisable(context.Background(), now, cfg)

	require.Equal(t, []int64{8}, invalidator.userIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageAlertAutoDisableSkipsWhenDisabled(t *testing.T) {
	svc := &Service{}
	cfg := storedConfig{Config: DefaultConfig()}
	cfg.UsageAlertEnabled = true

	svc.runUsageAlertAutoDisable(context.Background(), time.Now(), cfg)
}
