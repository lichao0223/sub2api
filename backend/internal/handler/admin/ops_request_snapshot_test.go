package admin

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"strings"
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
	body := []byte(`{"messages":[{"role":"user","content":"full request"}],"tools":[],"id":9007199254740993}`)
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
	require.Equal(t, `{
  "messages": [
    {
      "role": "user",
      "content": "full request"
    }
  ],
  "tools": [],
  "id": 9007199254740993
}`, w.Body.String())
	require.JSONEq(t, string(body), w.Body.String())
	require.Contains(t, w.Header().Get("Content-Disposition"), "error-42-request.json")
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, 400, request("invalid").Code)
	require.Equal(t, 400, request("0").Code)
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{"large JSON", `{"content":"` + strings.Repeat("x", 70*1024) + `","tail":true}`, "{\n  \"content\": \"" + strings.Repeat("x", 70*1024) + "\",\n  \"tail\": true\n}"},
		{"invalid JSON", `{"messages":[`, `{"messages":[`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo.data, err = service.CompressOpsRequestSnapshot([]byte(tt.body))
			require.NoError(t, err)
			w := request("42")
			require.Equal(t, 200, w.Code)
			require.Equal(t, tt.want, w.Body.String())
		})
	}
	repo.err = sql.ErrNoRows
	require.Equal(t, 404, request("42").Code)
	ops.SetMonitoringEnabled(false)
	require.Equal(t, 404, request("42").Code)
}
