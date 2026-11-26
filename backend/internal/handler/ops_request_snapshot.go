package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// attachOpsRequestSnapshot stores a size-capped, gzip-compressed copy of the
// failed request body. The capture policy (enable flag, byte cap, minimum
// severity) comes from ops_advanced_settings.data_retention.
func attachOpsRequestSnapshot(c *gin.Context, entry *service.OpsInsertErrorLogInput) {
	if c == nil || entry == nil || !c.GetBool(service.OpsCaptureRequestSnapshotKey) {
		return
	}
	policy := service.OpsRequestSnapshotPolicyFromContext(c)
	maxBytes := policy.MaxBytesForSeverity(entry.Severity)
	if maxBytes <= 0 {
		return
	}
	var body []byte
	if c.Request != nil && c.Request.GetBody != nil {
		if reader, err := c.Request.GetBody(); err == nil {
			if buffered, ok := reader.(*httputil.PrereadBody); ok {
				body = buffered.Bytes()
			}
			_ = reader.Close()
		}
	}
	if len(body) == 0 {
		if value, ok := c.Get(service.OpsRequestSnapshotBodyKey); ok {
			body, _ = value.([]byte)
		}
	}
	capture, err := service.BuildOpsRequestSnapshot(body, maxBytes)
	if err != nil {
		return
	}
	entry.RequestSnapshotBytes = capture.OriginalBytes
	entry.RequestSnapshotGzip = capture.Gzip
	entry.RequestSnapshotTruncated = capture.Truncated
}
