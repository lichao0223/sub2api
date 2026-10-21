//go:build unit

package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsInsertErrorLogArgsPreservesExplicitZeroUpstreamStatus(t *testing.T) {
	zero := 0
	args := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{UpstreamStatusCode: &zero})

	require.Len(t, args, 40)
	encoded, ok := args[27].(sql.NullInt64)
	require.True(t, ok)
	require.True(t, encoded.Valid)
	require.Zero(t, encoded.Int64)
}

func TestOpsNullableIntPointerDistinguishesNilZeroAndStatus(t *testing.T) {
	missing := opsNullableIntPointer(nil).(sql.NullInt64)
	require.False(t, missing.Valid)

	zeroValue := 0
	zero := opsNullableIntPointer(&zeroValue).(sql.NullInt64)
	require.True(t, zero.Valid)
	require.Zero(t, zero.Int64)

	statusValue := 503
	status := opsNullableIntPointer(&statusValue).(sql.NullInt64)
	require.True(t, status.Valid)
	require.EqualValues(t, 503, status.Int64)
}

func TestOpsInsertErrorLogArgsIncludesCompressedSnapshot(t *testing.T) {
	body := []byte("gzip snapshot")
	args := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{RequestSnapshotGzip: body, RequestSnapshotBytes: 50000})
	require.Equal(t, body, args[38])
	require.EqualValues(t, 50000, args[39])
}

func TestOpsRequestSnapshotRepositoryRead(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	query := regexp.QuoteMeta(`SELECT request_snapshot_gzip FROM ops_error_logs WHERE id = $1`)
	body := []byte("compressed request")
	mock.ExpectQuery(query).WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"request_snapshot_gzip"}).AddRow(body))
	got, err := repo.GetErrorRequestSnapshot(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, body, got)
	mock.ExpectQuery(query).WithArgs(int64(43)).WillReturnError(sql.ErrNoRows)
	_, err = repo.GetErrorRequestSnapshot(context.Background(), 43)
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.NoError(t, mock.ExpectationsWereMet())
}
