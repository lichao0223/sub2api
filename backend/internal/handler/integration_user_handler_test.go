package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIntegrationUserHandler_CreateValidation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantReason string
		wantField  string
	}{
		{
			name:       "invalid json",
			body:       `{`,
			wantReason: "INVALID_JSON",
		},
		{
			name:       "missing external user id",
			body:       `{"external_user_id":" ","username":"张三"}`,
			wantReason: "INVALID_ARGUMENT",
			wantField:  "external_user_id",
		},
		{
			name:       "missing username",
			body:       `{"external_user_id":"u-1","external_organization_id":"org-1","username":" "}`,
			wantReason: "INVALID_ARGUMENT",
			wantField:  "username",
		},
		{
			name:       "missing external organization id",
			body:       `{"external_user_id":"u-1","external_organization_id":" ","username":"张三"}`,
			wantReason: "INVALID_ARGUMENT",
			wantField:  "external_organization_id",
		},
		{
			name:       "external user id too long",
			body:       `{"external_user_id":"` + strings.Repeat("a", 256) + `","external_organization_id":"org-1","username":"张三"}`,
			wantReason: "INVALID_ARGUMENT",
			wantField:  "external_user_id",
		},
		{
			name:       "external organization id too long",
			body:       `{"external_user_id":"u-1","external_organization_id":"` + strings.Repeat("a", 256) + `","username":"张三"}`,
			wantReason: "INVALID_ARGUMENT",
			wantField:  "external_organization_id",
		},
		{
			name:       "username too long",
			body:       `{"external_user_id":"u-1","external_organization_id":"org-1","username":"` + strings.Repeat("a", 101) + `"}`,
			wantReason: "INVALID_ARGUMENT",
			wantField:  "username",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := newIntegrationUserTestRouter(&integrationUserServiceStub{})

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/integrations/users", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			var got map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			require.Equal(t, tt.wantReason, got["reason"])
			if tt.wantField != "" {
				metadata, ok := got["metadata"].(map[string]any)
				require.True(t, ok)
				require.Equal(t, tt.wantField, metadata["field"])
			}
		})
	}
}

func TestIntegrationUserHandler_CreateSuccessAndExisting(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		wantStatus int
	}{
		{name: "created", status: service.ExternalUserStatusCreated, wantStatus: http.StatusCreated},
		{name: "skipped", status: service.ExternalUserStatusSkipped, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &integrationUserServiceStub{
				createResult: &service.ExternalUserResult{
					Status:         tt.status,
					ExternalUserID: "u-1",
					User:           &service.ExternalUserUserInfo{ID: 10, Username: "张三"},
					APIKeys:        []service.ExternalUserAPIKeyInfo{{ID: 20, Key: "sk-test", Status: service.StatusAPIKeyActive}},
				},
			}
			router, _ := newIntegrationUserTestRouter(svc)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/integrations/users", bytes.NewBufferString(`{"external_user_id":" u-1 ","external_organization_id":" org-1 ","username":" 张三 "}`))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			require.Equal(t, service.ExternalUserInput{ExternalUserID: "u-1", ExternalOrganizationID: "org-1", Username: "张三"}, svc.createInput)
			require.Contains(t, rec.Body.String(), `"api_keys"`)
			require.NotContains(t, rec.Body.String(), `"api_key":`)
		})
	}
}

func TestIntegrationUserHandler_Delete(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		router, _ := newIntegrationUserTestRouter(&integrationUserServiceStub{
			deleteErr: service.ErrExternalUserMappingNotFound,
		})

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/integrations/users/u-missing", nil)
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
		require.Contains(t, rec.Body.String(), "EXTERNAL_USER_NOT_FOUND")
	})

	t.Run("success", func(t *testing.T) {
		svc := &integrationUserServiceStub{
			deleteResult: &service.ExternalUserDeleteResult{
				Status:         service.ExternalUserStatusDeleted,
				ExternalUserID: "u-1",
				UserID:         10,
			},
		}
		router, _ := newIntegrationUserTestRouter(svc)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/integrations/users/u-1", nil)
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "u-1", svc.deleteExternalUserID)
		require.Contains(t, rec.Body.String(), `"user_id":10`)
	})
}

func TestIntegrationUserHandler_DeleteAll(t *testing.T) {
	svc := &integrationUserServiceStub{
		deleteAllResult: &service.ExternalUserDeleteAllResult{
			Summary: service.ExternalUserDeleteAllSummary{
				Total:   2,
				Deleted: 2,
			},
			Items: []service.ExternalUserDeleteResult{
				{Status: service.ExternalUserStatusDeleted, ExternalUserID: "u-1", UserID: 10},
				{Status: service.ExternalUserStatusDeleted, ExternalUserID: "u-2", UserID: 11},
			},
		},
	}
	router, _ := newIntegrationUserTestRouter(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/integrations/users", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, svc.deleteAllCalled)
	require.Contains(t, rec.Body.String(), `"deleted":2`)
}

func TestIntegrationUserHandler_SyncValidation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantReason string
	}{
		{name: "invalid json", body: `{`, wantReason: "INVALID_JSON"},
		{name: "empty users", body: `{"users":[]}`, wantReason: "INVALID_ARGUMENT"},
		{name: "too many users", body: syncUsersPayload(501), wantReason: "BATCH_TOO_LARGE"},
		{name: "duplicate external user id", body: `{"users":[{"external_user_id":"u-1","external_organization_id":"org-1","username":"a"},{"external_user_id":" u-1 ","external_organization_id":"org-1","username":"b"}]}`, wantReason: "DUPLICATE_EXTERNAL_USER_ID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := newIntegrationUserTestRouter(&integrationUserServiceStub{})

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/integrations/users/sync", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Contains(t, rec.Body.String(), tt.wantReason)
		})
	}
}

func TestIntegrationUserHandler_SyncSuccess(t *testing.T) {
	svc := &integrationUserServiceStub{
		syncResult: &service.ExternalUserSyncResult{
			BatchID: "batch-1",
			Summary: service.ExternalUserSyncSummary{
				Total:   2,
				Created: 1,
				Skipped: 1,
			},
			Items: []service.ExternalUserResult{
				{Status: service.ExternalUserStatusCreated, ExternalUserID: "u-1", APIKeys: []service.ExternalUserAPIKeyInfo{{ID: 1, Key: "sk-1"}}},
				{Status: service.ExternalUserStatusSkipped, ExternalUserID: "u-2", APIKeys: []service.ExternalUserAPIKeyInfo{{ID: 2, Key: "sk-2"}}},
			},
		},
	}
	router, _ := newIntegrationUserTestRouter(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/integrations/users/sync", bytes.NewBufferString(`{"batch_id":" batch-1 ","users":[{"external_user_id":"u-1","external_organization_id":"org-1","username":"张三"},{"external_user_id":"u-2","external_organization_id":"org-1","username":"李四"}]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "batch-1", svc.syncInput.BatchID)
	require.Len(t, svc.syncInput.Users, 2)
	require.Contains(t, rec.Body.String(), `"api_keys"`)
	require.NotContains(t, rec.Body.String(), `"api_key":`)
}

func newIntegrationUserTestRouter(svc *integrationUserServiceStub) (*gin.Engine, *IntegrationUserHandler) {
	gin.SetMode(gin.TestMode)
	h := NewIntegrationUserHandler(svc)
	router := gin.New()
	router.POST("/integrations/users", h.Create)
	router.DELETE("/integrations/users", h.DeleteAll)
	router.DELETE("/integrations/users/:external_user_id", h.DeleteByExternalID)
	router.POST("/integrations/users/sync", h.Sync)
	return router, h
}

func syncUsersPayload(n int) string {
	users := make([]externalUserRequest, 0, n)
	for i := 0; i < n; i++ {
		users = append(users, externalUserRequest{
			ExternalUserID:         "u-" + strconv.Itoa(i),
			ExternalOrganizationID: "org-1",
			Username:               "用户",
		})
	}
	payload, _ := json.Marshal(externalUserSyncRequest{Users: users})
	return string(payload)
}

type integrationUserServiceStub struct {
	createResult *service.ExternalUserResult
	createErr    error
	createInput  service.ExternalUserInput

	deleteResult         *service.ExternalUserDeleteResult
	deleteErr            error
	deleteExternalUserID string
	deleteAllResult      *service.ExternalUserDeleteAllResult
	deleteAllErr         error
	deleteAllCalled      bool

	syncResult *service.ExternalUserSyncResult
	syncErr    error
	syncInput  service.ExternalUserSyncInput
}

func (s *integrationUserServiceStub) Create(_ context.Context, input service.ExternalUserInput) (*service.ExternalUserResult, error) {
	s.createInput = input
	if s.createErr != nil {
		return nil, s.createErr
	}
	return s.createResult, nil
}

func (s *integrationUserServiceStub) DeleteByExternalID(_ context.Context, externalUserID string) (*service.ExternalUserDeleteResult, error) {
	s.deleteExternalUserID = externalUserID
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}
	return s.deleteResult, nil
}

func (s *integrationUserServiceStub) DeleteAll(_ context.Context) (*service.ExternalUserDeleteAllResult, error) {
	s.deleteAllCalled = true
	if s.deleteAllErr != nil {
		return nil, s.deleteAllErr
	}
	return s.deleteAllResult, nil
}

func (s *integrationUserServiceStub) Sync(_ context.Context, input service.ExternalUserSyncInput) (*service.ExternalUserSyncResult, error) {
	s.syncInput = input
	if s.syncErr != nil {
		return nil, s.syncErr
	}
	return s.syncResult, nil
}
