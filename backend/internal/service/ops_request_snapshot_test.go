package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type opsSnapshotTestRepo struct {
	OpsRepository
	data []byte
	err  error
}

func (r *opsSnapshotTestRepo) GetErrorRequestSnapshot(context.Context, int64) ([]byte, error) {
	return r.data, r.err
}

func TestOpsRequestSnapshotRoundTripAndMissing(t *testing.T) {
	body := `{"messages":[{"role":"tool","content":"` + strings.Repeat("complete tool result 中文 ", 10000) + `"}],"tools":[{"name":"test"}]}`
	compressed, err := CompressOpsRequestSnapshot([]byte(body))
	require.NoError(t, err)
	repo := &opsSnapshotTestRepo{data: compressed}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	got, err := svc.GetErrorRequestSnapshot(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, body, string(got))
	repo.err = sql.ErrNoRows
	_, err = svc.GetErrorRequestSnapshot(context.Background(), 1)
	require.Error(t, err)
	repo.err, repo.data = nil, nil
	_, err = svc.GetErrorRequestSnapshot(context.Background(), 1)
	require.Error(t, err)
	repo.data = []byte("not gzip")
	_, err = svc.GetErrorRequestSnapshot(context.Background(), 1)
	require.Error(t, err)
	svc.SetMonitoringEnabled(false)
	_, err = svc.GetErrorRequestSnapshot(context.Background(), 1)
	require.ErrorIs(t, err, ErrOpsDisabled)
}

func TestOpsRequestSnapshotStorageLimitDoesNotReturnPartialBody(t *testing.T) {
	input := make([]byte, OpsRequestSnapshotMaxCompressedBytes+1024)
	_, err := rand.Read(input)
	require.NoError(t, err)
	compressed, err := CompressOpsRequestSnapshot(input)
	require.ErrorIs(t, err, errOpsSnapshotTooLarge)
	require.Nil(t, compressed)
}

func TestOpsRequestSnapshotWebSocketRetentionBudget(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportWS)
	c.Set(OpsCaptureRequestSnapshotKey, true)
	c.Set(OpsStreamErrorsKey, []OpsStreamError{
		{Turn: 1, RequestSnapshotGzip: make([]byte, OpsRequestSnapshotMaxCompressedBytes/2), RequestSnapshotBytes: 100},
		{Turn: 2, RequestSnapshotGzip: make([]byte, OpsRequestSnapshotMaxCompressedBytes/2), RequestSnapshotBytes: 100},
	})
	BeginOpsStreamTurn(c, 3)
	c.Set(OpsRequestSnapshotBodyKey, []byte(`{"input":"new failure"}`))
	MarkOpsStreamErrorValue(c, OpsStreamError{RequestScoped: true, IntendedStatus: 400})
	errors := GetOpsStreamErrors(c)
	require.Len(t, errors, 3)
	require.Empty(t, errors[0].RequestSnapshotGzip)
	require.EqualValues(t, 100, errors[0].RequestSnapshotBytes)
	require.NotEmpty(t, errors[1].RequestSnapshotGzip)
	require.NotEmpty(t, errors[2].RequestSnapshotGzip)
}

func TestOpsRequestSnapshotRecoveredRequestIsNotPersisted(t *testing.T) {
	svc := NewOpsService(&opsSnapshotTestRepo{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	entry, keep, err := svc.prepareErrorLogInput(context.Background(), &OpsInsertErrorLogInput{
		StatusCode: 200, ErrorPhase: "upstream",
		RequestSnapshotGzip: []byte("must not persist"), RequestSnapshotBytes: 100,
	})
	require.NoError(t, err)
	require.True(t, keep)
	require.Empty(t, entry.RequestSnapshotGzip)
	require.Zero(t, entry.RequestSnapshotBytes)
}
