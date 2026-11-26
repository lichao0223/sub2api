package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"database/sql"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

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

func gunzipSnapshot(t *testing.T, compressed []byte) string {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	require.NoError(t, err)
	defer func() { require.NoError(t, r.Close()) }()
	body, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(body)
}

func TestOpsRequestSnapshotRoundTripAndMissing(t *testing.T) {
	body := `{"messages":[{"role":"tool","content":"` + strings.Repeat("complete tool result 中文 ", 10000) + `"}],"tools":[{"name":"test"}]}`
	capture, err := BuildOpsRequestSnapshot([]byte(body), OpsRequestSnapshotMaxBytes)
	require.NoError(t, err)
	require.False(t, capture.Truncated)
	require.EqualValues(t, len(body), capture.OriginalBytes)
	require.Equal(t, body, gunzipSnapshot(t, capture.Gzip))

	repo := &opsSnapshotTestRepo{data: capture.Gzip}
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

func TestOpsRequestSnapshotTruncatesOversizedBody(t *testing.T) {
	full := strings.Repeat(`{"messages":[{"role":"user","content":"diagnostic payload"}],}`, 40000)
	require.Greater(t, len(full), OpsRequestSnapshotMaxBytes)

	capture, err := BuildOpsRequestSnapshot([]byte(full), OpsRequestSnapshotMaxBytes)
	require.NoError(t, err)
	require.True(t, capture.Truncated)
	require.EqualValues(t, len(full), capture.OriginalBytes)
	require.LessOrEqual(t, len(capture.Gzip), OpsRequestSnapshotMaxCompressedBytes)

	stored := gunzipSnapshot(t, capture.Gzip)
	require.Len(t, stored, OpsRequestSnapshotMaxBytes)
	require.True(t, strings.HasPrefix(full, stored))
}

func TestOpsRequestSnapshotHighEntropyBodyStaysUnderCompressedCap(t *testing.T) {
	input := make([]byte, OpsRequestSnapshotMaxBytes)
	_, err := rand.Read(input)
	require.NoError(t, err)

	capture, err := BuildOpsRequestSnapshot(input, OpsRequestSnapshotMaxBytes)
	require.NoError(t, err)
	require.True(t, capture.Truncated)
	require.EqualValues(t, len(input), capture.OriginalBytes)
	require.NotEmpty(t, capture.Gzip)
	require.LessOrEqual(t, len(capture.Gzip), OpsRequestSnapshotMaxCompressedBytes)
}

func TestBuildOpsRequestSnapshotKeepsBinaryBody(t *testing.T) {
	input := make([]byte, 4096)
	_, err := rand.Read(input)
	require.NoError(t, err)

	capture, err := BuildOpsRequestSnapshot(input, 4096)
	require.NoError(t, err)
	require.False(t, capture.Truncated)
	r, err := gzip.NewReader(bytes.NewReader(capture.Gzip))
	require.NoError(t, err)
	stored, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	require.True(t, bytes.Equal(input, stored))
}

func TestBuildOpsRequestSnapshotTruncatesOnUTF8Boundary(t *testing.T) {
	full := strings.Repeat("中", 10000) // 3 bytes each
	rawCap := 3001                     // deliberately inside a rune
	capture, err := BuildOpsRequestSnapshot([]byte(full), rawCap)
	require.NoError(t, err)
	require.True(t, capture.Truncated)

	stored := gunzipSnapshot(t, capture.Gzip)
	require.True(t, utf8.ValidString(stored))
	require.LessOrEqual(t, len(stored), rawCap)
	require.True(t, strings.HasPrefix(full, stored))
}

func TestOpsRequestSnapshotCustomRawCap(t *testing.T) {
	body := strings.Repeat("a", 64<<10)
	capture, err := BuildOpsRequestSnapshot([]byte(body), 16<<10)
	require.NoError(t, err)
	require.True(t, capture.Truncated)
	require.EqualValues(t, len(body), capture.OriginalBytes)
	require.LessOrEqual(t, len(gunzipSnapshot(t, capture.Gzip)), 16<<10)
}

func TestOpsRequestSnapshotPolicyDefaults(t *testing.T) {
	policy := opsRequestSnapshotPolicyFromRetention(OpsDataRetentionSettings{})
	require.True(t, policy.Enabled)
	require.Equal(t, OpsRequestSnapshotMaxBytes, policy.MaxBytes)
	require.Equal(t, OpsSnapshotSeverityP3, policy.MinSeverity)
	require.Equal(t, OpsRequestSnapshotMaxBytes, policy.MaxBytesForSeverity("P3"))
}

func TestOpsRequestSnapshotPolicyDisabledWhenMaxBytesZero(t *testing.T) {
	zero := 0
	policy := opsRequestSnapshotPolicyFromRetention(OpsDataRetentionSettings{ErrorLogSnapshotMaxBytes: &zero})
	require.False(t, policy.Enabled)
	require.Zero(t, policy.MaxBytesForSeverity("P1"))
}

func TestOpsRequestSnapshotPolicySeverityHeadOnly(t *testing.T) {
	maxBytes := 512 << 10
	policy := opsRequestSnapshotPolicyFromRetention(OpsDataRetentionSettings{
		ErrorLogSnapshotMaxBytes:    &maxBytes,
		ErrorLogSnapshotMinSeverity: "P2",
	})
	require.Equal(t, 512<<10, policy.MaxBytesForSeverity("P1"))
	require.Equal(t, 512<<10, policy.MaxBytesForSeverity("P2"))
	require.Equal(t, OpsRequestSnapshotHeadBytes, policy.MaxBytesForSeverity("P3"))
	require.False(t, IsValidOpsSnapshotSeverity("P4"))
}

func TestOpsRequestSnapshotPolicyFromContextFallsBackToDefault(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	require.Equal(t, DefaultOpsRequestSnapshotPolicy(), OpsRequestSnapshotPolicyFromContext(c))
	c.Set(OpsRequestSnapshotPolicyKey, OpsRequestSnapshotPolicy{Enabled: false})
	require.False(t, OpsRequestSnapshotPolicyFromContext(c).Enabled)
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
		RequestSnapshotGzip: []byte("must not persist"), RequestSnapshotBytes: 100, RequestSnapshotTruncated: true,
	})
	require.NoError(t, err)
	require.True(t, keep)
	require.Empty(t, entry.RequestSnapshotGzip)
	require.Zero(t, entry.RequestSnapshotBytes)
	require.False(t, entry.RequestSnapshotTruncated)
}
