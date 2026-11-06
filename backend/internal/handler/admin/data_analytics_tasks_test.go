package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type analyticsHandlerRepo struct {
	control service.AnalyticsTaskControl
}

func (r *analyticsHandlerRepo) GetAnalyticsTaskControl(context.Context) (service.AnalyticsTaskControl, error) {
	return r.control, nil
}
func (r *analyticsHandlerRepo) UpdateAnalyticsTaskControl(_ context.Context, enabled bool, version, actorID int64) (service.AnalyticsTaskControl, error) {
	if version != r.control.Version {
		return r.control, service.ErrAnalyticsControlConflict
	}
	r.control.Enabled, r.control.Version, r.control.UpdatedBy = enabled, version+1, actorID
	return r.control, nil
}
func (*analyticsHandlerRepo) RecordAnalyticsTaskWorker(context.Context, service.AnalyticsTaskWorker) error {
	return nil
}
func (*analyticsHandlerRepo) ListAnalyticsTaskWorkers(context.Context) ([]service.AnalyticsTaskWorker, error) {
	return nil, nil
}

func TestAnalyticsTaskSettingsValidateAndHandleConflicts(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"enabled":true,"version":1}`, http.StatusOK},
		{`{"enabled":false,"version":1}`, http.StatusOK},
		{`{"version":1}`, http.StatusBadRequest},
		{`{"enabled":true,"version":0}`, http.StatusBadRequest},
		{`{"enabled":"true","version":1}`, http.StatusBadRequest},
		{`{"enabled":true,"version":9}`, http.StatusConflict},
	} {
		t.Run(tc.body, func(t *testing.T) {
			repo := &analyticsHandlerRepo{control: service.AnalyticsTaskControl{Version: 1}}
			h := NewCostManagementHandler(nil, service.NewAnalyticsTaskService(repo, "test"))
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/data-analytics", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
			h.UpdateAnalyticsTaskSettings(c)
			require.Equal(t, tc.status, rec.Code)
			if tc.status == http.StatusOK {
				require.EqualValues(t, 7, repo.control.UpdatedBy)
				require.EqualValues(t, 2, repo.control.Version)
			} else {
				require.EqualValues(t, 1, repo.control.Version)
			}
		})
	}
}

func TestAnalyticsTaskSettingsRequireAuthenticatedActor(t *testing.T) {
	repo := &analyticsHandlerRepo{control: service.AnalyticsTaskControl{Version: 1}}
	h := NewCostManagementHandler(nil, service.NewAnalyticsTaskService(repo, "test"))
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"enabled":true,"version":1}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateAnalyticsTaskSettings(c)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.False(t, repo.control.Enabled)
}
