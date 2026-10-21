package admin

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type adminOpsSnapshotRepo struct {
	service.OpsRepository
	data []byte
	err  error
}

func (r *adminOpsSnapshotRepo) GetErrorRequestSnapshot(context.Context, int64) ([]byte, error) {
	return r.data, r.err
}

func TestDownloadErrorRequestSnapshot(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"full request"}],"tools":[]}`)
	compressed, err := service.CompressOpsRequestSnapshot(body)
	require.NoError(t, err)
	repo := &adminOpsSnapshotRepo{data: compressed}
	ops := service.NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/errors/:id/request-body", NewOpsHandler(ops).DownloadErrorRequestSnapshot)
	request := func(id string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/errors/"+id+"/request-body", nil))
		return w
	}
	w := request("42")
	require.Equal(t, 200, w.Code)
	require.Equal(t, string(body), w.Body.String())
	require.Contains(t, w.Header().Get("Content-Disposition"), "error-42-request.json")
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, 400, request("invalid").Code)
	require.Equal(t, 400, request("0").Code)
	repo.err = sql.ErrNoRows
	require.Equal(t, 404, request("42").Code)
	ops.SetMonitoringEnabled(false)
	require.Equal(t, 404, request("42").Code)
}
