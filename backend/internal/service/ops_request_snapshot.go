package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
)

const (
	OpsRequestSnapshotBodyKey    = "ops_request_snapshot_body"
	OpsCaptureRequestSnapshotKey = "ops_capture_request_snapshot"
	// OpsRequestSnapshotPolicyKey carries the per-request capture policy resolved
	// from ops_advanced_settings.data_retention at middleware entry.
	OpsRequestSnapshotPolicyKey = "ops_request_snapshot_policy"

	// OpsRequestSnapshotMaxBytes is the default raw cap for one request snapshot (1MB).
	OpsRequestSnapshotMaxBytes = 1 << 20
	// OpsRequestSnapshotMaxCompressedBytes is the storage cap after gzip (256KB).
	OpsRequestSnapshotMaxCompressedBytes = 256 << 10
	// Bounds for the runtime-configurable raw cap.
	OpsRequestSnapshotMinMaxBytes  = 16 << 10
	OpsRequestSnapshotHardMaxBytes = 8 << 20
	// OpsRequestSnapshotHeadBytes keeps only the body head for severities below
	// the configured minimum (diagnostics still need the request prefix).
	OpsRequestSnapshotHeadBytes = 16 << 10

	// Read-side ceilings tolerate snapshots written before the caps were tightened.
	OpsRequestSnapshotLegacyMaxCompressedBytes = 16 << 20
	OpsRequestSnapshotLegacyMaxDecodedBytes    = 64 << 20

	// OpsRequestSnapshotDefaultRetentionHours keeps snapshot payloads for a day by default.
	OpsRequestSnapshotDefaultRetentionHours = 24
	// OpsRequestSnapshotMaxRetentionHours bounds the runtime knob (30 days).
	OpsRequestSnapshotMaxRetentionHours = 720

	OpsSnapshotSeverityP1 = "P1"
	OpsSnapshotSeverityP2 = "P2"
	OpsSnapshotSeverityP3 = "P3"
)

var errOpsSnapshotTooLarge = errors.New("error request snapshot exceeds storage limit")

// OpsRequestSnapshotCapture is the result of capturing one request body.
type OpsRequestSnapshotCapture struct {
	Gzip          []byte
	OriginalBytes int64
	Truncated     bool
}

// OpsRequestSnapshotPolicy decides whether and how much of a request body is kept.
type OpsRequestSnapshotPolicy struct {
	Enabled bool
	// MaxBytes is the raw cap applied to severities at or above MinSeverity.
	MaxBytes int
	// MinSeverity is the loosest severity that still keeps a full body ("P3" = all).
	MinSeverity string
}

func DefaultOpsRequestSnapshotPolicy() OpsRequestSnapshotPolicy {
	return OpsRequestSnapshotPolicy{
		Enabled:     true,
		MaxBytes:    OpsRequestSnapshotMaxBytes,
		MinSeverity: OpsSnapshotSeverityP3,
	}
}

// OpsRequestSnapshotPolicy resolves the runtime policy from the cached advanced
// settings; request hot paths never touch the settings repository.
func (s *OpsService) OpsRequestSnapshotPolicy() OpsRequestSnapshotPolicy {
	if s == nil {
		return DefaultOpsRequestSnapshotPolicy()
	}
	snapshot := s.runtimeSettings.Load()
	if snapshot == nil {
		return DefaultOpsRequestSnapshotPolicy()
	}
	return opsRequestSnapshotPolicyFromRetention(snapshot.advanced.DataRetention)
}

func opsRequestSnapshotPolicyFromRetention(dr OpsDataRetentionSettings) OpsRequestSnapshotPolicy {
	policy := DefaultOpsRequestSnapshotPolicy()
	if dr.ErrorLogSnapshotMaxBytes != nil {
		policy.MaxBytes = *dr.ErrorLogSnapshotMaxBytes
	}
	if policy.MaxBytes < 0 {
		policy.MaxBytes = 0
	}
	if policy.MaxBytes > OpsRequestSnapshotHardMaxBytes {
		policy.MaxBytes = OpsRequestSnapshotHardMaxBytes
	}
	// max_bytes == 0 turns snapshot capture off entirely.
	policy.Enabled = policy.MaxBytes > 0
	policy.MinSeverity = normalizeOpsSnapshotSeverity(dr.ErrorLogSnapshotMinSeverity)
	return policy
}

// MaxBytesForSeverity returns the raw cap for one error severity; 0 means "do not capture".
func (p OpsRequestSnapshotPolicy) MaxBytesForSeverity(severity string) int {
	if !p.Enabled || p.MaxBytes <= 0 {
		return 0
	}
	if opsSnapshotSeverityRank(severity) > opsSnapshotSeverityRank(p.MinSeverity) {
		return min(p.MaxBytes, OpsRequestSnapshotHeadBytes)
	}
	return p.MaxBytes
}

// OpsRequestSnapshotPolicyFromContext reads the policy resolved by the ops error
// logger middleware, falling back to the default policy for direct callers/tests.
func OpsRequestSnapshotPolicyFromContext(c *gin.Context) OpsRequestSnapshotPolicy {
	if c == nil {
		return DefaultOpsRequestSnapshotPolicy()
	}
	if value, ok := c.Get(OpsRequestSnapshotPolicyKey); ok {
		if policy, ok := value.(OpsRequestSnapshotPolicy); ok {
			return policy
		}
	}
	return DefaultOpsRequestSnapshotPolicy()
}

func normalizeOpsSnapshotSeverity(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case OpsSnapshotSeverityP1:
		return OpsSnapshotSeverityP1
	case OpsSnapshotSeverityP2:
		return OpsSnapshotSeverityP2
	default:
		return OpsSnapshotSeverityP3
	}
}

// IsValidOpsSnapshotSeverity reports whether value is an accepted (possibly empty) severity.
func IsValidOpsSnapshotSeverity(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", OpsSnapshotSeverityP1, OpsSnapshotSeverityP2, OpsSnapshotSeverityP3:
		return true
	}
	return false
}

func opsSnapshotSeverityRank(severity string) int {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case OpsSnapshotSeverityP1:
		return 1
	case OpsSnapshotSeverityP2:
		return 2
	default:
		return 3
	}
}

// BuildOpsRequestSnapshot gzips a request body for storage under a raw byte cap.
// Bodies over the cap are truncated on a UTF-8 boundary and flagged; the stored
// payload additionally never exceeds OpsRequestSnapshotMaxCompressedBytes, so a
// high-entropy body is shrunk further instead of being dropped or rejected.
func BuildOpsRequestSnapshot(body []byte, maxRawBytes int) (OpsRequestSnapshotCapture, error) {
	capture := OpsRequestSnapshotCapture{OriginalBytes: int64(len(body))}
	if len(body) == 0 {
		return capture, nil
	}
	if maxRawBytes <= 0 {
		maxRawBytes = OpsRequestSnapshotMaxBytes
	}
	if maxRawBytes > OpsRequestSnapshotHardMaxBytes {
		maxRawBytes = OpsRequestSnapshotHardMaxBytes
	}
	limit := len(body)
	if limit > maxRawBytes {
		limit = maxRawBytes
		capture.Truncated = true
	}
	for {
		compressed, err := compressOpsSnapshot(trimOpsSnapshotUTF8(body[:limit]))
		if err != nil {
			return OpsRequestSnapshotCapture{}, err
		}
		if len(compressed) <= OpsRequestSnapshotMaxCompressedBytes {
			capture.Gzip = compressed
			return capture, nil
		}
		// Poorly compressible payload: keep shrinking the raw head until the
		// compressed representation fits the storage budget.
		capture.Truncated = true
		next := limit / 2
		if next < OpsRequestSnapshotMinMaxBytes/4 {
			return OpsRequestSnapshotCapture{}, errOpsSnapshotTooLarge
		}
		limit = next
	}
}

func compressOpsSnapshot(body []byte) ([]byte, error) {
	var out bytes.Buffer
	w, err := gzip.NewWriterLevel(&out, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(body); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// trimOpsSnapshotUTF8 drops at most one partial UTF-8 sequence at the cut point.
// It must not reject a payload that is not valid UTF-8 as a whole: request bodies
// can be compressed or otherwise binary, and they still deserve a snapshot.
func trimOpsSnapshotUTF8(body []byte) []byte {
	if len(body) == 0 || body[len(body)-1] < utf8.RuneSelf {
		return body
	}
	start := len(body) - 1
	for start > 0 && !utf8.RuneStart(body[start]) {
		start--
	}
	if utf8.FullRune(body[start:]) {
		return body
	}
	return body[:start]
}

// GetErrorRequestSnapshot is used only by the admin-only, on-demand download endpoint.
func (s *OpsService) GetErrorRequestSnapshot(ctx context.Context, id int64) ([]byte, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	repo, ok := s.opsRepo.(interface {
		GetErrorRequestSnapshot(context.Context, int64) ([]byte, error)
	})
	if !ok {
		return nil, infraerrors.NotFound("OPS_REQUEST_SNAPSHOT_NOT_FOUND", "Error request snapshot not available")
	}
	compressed, err := repo.GetErrorRequestSnapshot(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && len(compressed) == 0) {
		return nil, infraerrors.NotFound("OPS_REQUEST_SNAPSHOT_NOT_FOUND", "Error request snapshot not available")
	}
	if err != nil {
		return nil, err
	}
	if len(compressed) > OpsRequestSnapshotLegacyMaxCompressedBytes {
		return nil, errOpsSnapshotTooLarge
	}
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("open error request snapshot: %w", err)
	}
	defer func() { _ = r.Close() }()
	body, err := io.ReadAll(io.LimitReader(r, OpsRequestSnapshotLegacyMaxDecodedBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > OpsRequestSnapshotLegacyMaxDecodedBytes {
		return nil, errOpsSnapshotTooLarge
	}
	return body, nil
}
