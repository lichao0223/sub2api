package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIntegrationRoutesUseAdminAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	svc := &integrationRoutesUserServiceStub{
		createResult: &service.ExternalUserResult{
			Status:         service.ExternalUserStatusCreated,
			ExternalUserID: "u-1",
			APIKey:         &service.ExternalUserAPIKeyInfo{ID: 1, Key: "sk-test"},
		},
	}
	h := &handler.Handlers{
		Integration: &handler.IntegrationHandlers{
			User: handler.NewIntegrationUserHandler(svc),
		},
	}

	v1 := router.Group("/api/v1")
	RegisterIntegrationRoutes(v1, h, servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("x-test-admin") != "ok" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/users", strings.NewReader(`{"external_user_id":"u-1","external_organization_id":"org-1","username":"张三"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/integrations/users", strings.NewReader(`{"external_user_id":"u-1","external_organization_id":"org-1","username":"张三"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-test-admin", "ok")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, service.ExternalUserInput{ExternalUserID: "u-1", ExternalOrganizationID: "org-1", Username: "张三"}, svc.createInput)
}

type integrationRoutesUserServiceStub struct {
	createResult *service.ExternalUserResult
	createInput  service.ExternalUserInput
}

func (s *integrationRoutesUserServiceStub) Create(_ context.Context, input service.ExternalUserInput) (*service.ExternalUserResult, error) {
	s.createInput = input
	return s.createResult, nil
}

func (s *integrationRoutesUserServiceStub) DeleteByExternalID(_ context.Context, externalUserID string) (*service.ExternalUserDeleteResult, error) {
	return &service.ExternalUserDeleteResult{
		Status:         service.ExternalUserStatusDeleted,
		ExternalUserID: externalUserID,
		UserID:         1,
	}, nil
}

func (s *integrationRoutesUserServiceStub) DeleteAll(_ context.Context) (*service.ExternalUserDeleteAllResult, error) {
	return &service.ExternalUserDeleteAllResult{
		Summary: service.ExternalUserDeleteAllSummary{Total: 1, Deleted: 1},
	}, nil
}

func (s *integrationRoutesUserServiceStub) Sync(_ context.Context, input service.ExternalUserSyncInput) (*service.ExternalUserSyncResult, error) {
	return &service.ExternalUserSyncResult{
		BatchID: input.BatchID,
		Summary: service.ExternalUserSyncSummary{
			Total: len(input.Users),
		},
	}, nil
}
