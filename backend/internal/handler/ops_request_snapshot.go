package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func attachOpsRequestSnapshot(c *gin.Context, entry *service.OpsInsertErrorLogInput) {
	if c == nil || entry == nil || !c.GetBool(service.OpsCaptureRequestSnapshotKey) {
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
	entry.RequestSnapshotBytes = int64(len(body))
	entry.RequestSnapshotGzip, _ = service.CompressOpsRequestSnapshot(body)
}
