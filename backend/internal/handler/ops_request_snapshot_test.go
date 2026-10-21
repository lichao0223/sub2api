package handler

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func decodeRequestSnapshot(t *testing.T, compressed []byte) string {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	require.NoError(t, err)
	defer func() { require.NoError(t, r.Close()) }()
	body, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(body)
}

func TestOpsRequestSnapshotOnlyPersistsFailures(t *testing.T) {
	for _, status := range []int{200, 400, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			setupOpsErrorLogTestQueue(t, 1)
			ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			router := gin.New()
			router.Use(OpsErrorLoggerMiddleware(ops))
			router.POST("/v1/messages", func(c *gin.Context) {
				_, err := readLenientJSONRequestBodyWithPrealloc(c.Request, nil)
				require.NoError(t, err)
				c.JSON(status, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "test"}})
			})
			body := `{"messages":[{"role":"tool","content":"` + strings.Repeat("tool result ", 10000) + `"}],"tools":[{"name":"test"}]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer must-not-be-captured")
			router.ServeHTTP(httptest.NewRecorder(), req)
			if status == 200 {
				require.Zero(t, OpsErrorLogQueueLength())
				return
			}
			require.EqualValues(t, 1, OpsErrorLogQueueLength())
			entry := (<-opsErrorLogQueue).entry
			require.EqualValues(t, len(body), entry.RequestSnapshotBytes)
			require.Equal(t, body, decodeRequestSnapshot(t, entry.RequestSnapshotGzip))
			require.NotContains(t, decodeRequestSnapshot(t, entry.RequestSnapshotGzip), "must-not-be-captured")
		})
	}
}

func TestOpsRequestSnapshotMalformedJSON(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 1)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1/messages", func(c *gin.Context) {
		_, _ = readLenientJSONRequestBodyWithPrealloc(c.Request, nil)
		c.JSON(400, gin.H{"error": gin.H{"message": "Invalid JSON"}})
	})
	body := `{"messages":[`
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body)))
	entry := (<-opsErrorLogQueue).entry
	require.Equal(t, body, decodeRequestSnapshot(t, entry.RequestSnapshotGzip))
}

func TestOpsRequestSnapshotWebSocketKeepsFailingTurn(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 2)
	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.GET("/v1/responses", func(c *gin.Context) {
		c.Status(http.StatusSwitchingProtocols)
		setOpenAIClientTransportWS(c)
		for turn, body := range []string{`{"input":"first failed turn"}`, `{"input":"second failed turn"}`} {
			service.BeginOpsStreamTurn(c, turn+1)
			c.Set(service.OpsRequestSnapshotBodyKey, []byte(body))
			service.MarkOpsStreamErrorValue(c, service.OpsStreamError{IntendedStatus: 400, ErrType: "invalid_request_error", Message: "test", RequestScoped: true})
		}
		service.BeginOpsStreamTurn(c, 3)
		c.Set(service.OpsRequestSnapshotBodyKey, []byte(`{"input":"later successful turn"}`))
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/responses", nil))
	require.EqualValues(t, 2, OpsErrorLogQueueLength())
	require.Equal(t, `{"input":"first failed turn"}`, decodeRequestSnapshot(t, (<-opsErrorLogQueue).entry.RequestSnapshotGzip))
	require.Equal(t, `{"input":"second failed turn"}`, decodeRequestSnapshot(t, (<-opsErrorLogQueue).entry.RequestSnapshotGzip))
}

func TestOpsRequestSnapshotQueueBudgetRetainsErrorWithoutPayload(t *testing.T) {
	setupOpsErrorLogTestQueue(t, 1)
	opsErrorLogQueueBytes.Store(opsErrorLogMaxQueueBytes - 2048)
	entry := &service.OpsInsertErrorLogInput{StatusCode: 400, RequestSnapshotBytes: 4096, RequestSnapshotGzip: make([]byte, 4096)}
	enqueueOpsErrorLog(service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil), entry)
	require.EqualValues(t, 1, OpsErrorLogQueueLength())
	queued := (<-opsErrorLogQueue).entry
	require.Empty(t, queued.RequestSnapshotGzip)
	require.EqualValues(t, 4096, queued.RequestSnapshotBytes)
}
