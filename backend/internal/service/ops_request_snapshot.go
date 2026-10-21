package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	OpsRequestSnapshotBodyKey            = "ops_request_snapshot_body"
	OpsCaptureRequestSnapshotKey         = "ops_capture_request_snapshot"
	OpsRequestSnapshotMaxBytes           = 256 << 20
	OpsRequestSnapshotMaxCompressedBytes = 8 << 20
)

var errOpsSnapshotTooLarge = errors.New("error request snapshot exceeds storage limit")

type opsSnapshotBuffer struct{ bytes.Buffer }

func (b *opsSnapshotBuffer) Write(p []byte) (int, error) {
	if len(p) > OpsRequestSnapshotMaxCompressedBytes-b.Len() {
		return 0, errOpsSnapshotTooLarge
	}
	return b.Buffer.Write(p)
}

// CompressOpsRequestSnapshot never truncates: an oversized body keeps its byte
// count but no payload, so the UI cannot mistake a partial capture for a full one.
func CompressOpsRequestSnapshot(body []byte) ([]byte, error) {
	if len(body) == 0 {
		return nil, nil
	}
	if len(body) > OpsRequestSnapshotMaxBytes {
		return nil, errOpsSnapshotTooLarge
	}
	var out opsSnapshotBuffer
	w, err := gzip.NewWriterLevel(&out, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err = w.Write(body); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
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
	if len(compressed) > OpsRequestSnapshotMaxCompressedBytes {
		return nil, errOpsSnapshotTooLarge
	}
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("open error request snapshot: %w", err)
	}
	defer func() { _ = r.Close() }()
	body, err := io.ReadAll(io.LimitReader(r, OpsRequestSnapshotMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > OpsRequestSnapshotMaxBytes {
		return nil, errOpsSnapshotTooLarge
	}
	return body, nil
}
