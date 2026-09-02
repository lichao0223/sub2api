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
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
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

func TestIntegrationUserHandler_RotateAPIKeys(t *testing.T) {
	svc := &integrationUserServiceStub{}
	router, _ := newIntegrationUserTestRouter(svc)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/integrations/users/u-1/api-keys/rotate", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "u-1", svc.rotateExternalUserID)
}

func TestIntegrationUserHandler_ListUsageReturnsDisplayFields(t *testing.T) {
	effort := "medium"
	endpoint := "/v1/responses"
	ipAddress := "192.168.51.27"
	firstTokenMs, durationMs := 4520, 6700
	createdAt := time.Date(2026, 8, 28, 15, 51, 41, 0, time.FixedZone("CST", 8*60*60))
	svc := &integrationUserServiceStub{
		usageLogs: []service.UsageLog{{
			RequestedModel:  "gpt-5.6-sol",
			ReasoningEffort: &effort,
			InboundEndpoint: &endpoint,
			IPAddress:       &ipAddress,
			InputTokens:     2495,
			OutputTokens:    103,
			CacheReadTokens: 78600,
			ActualCost:      0.053337,
			BillingType:     service.BillingTypeBalance,
			RequestType:     service.RequestTypeStream,
			FirstTokenMs:    &firstTokenMs,
			DurationMs:      &durationMs,
			CreatedAt:       createdAt,
			APIKey:          &service.APIKey{Name: "CHATGPT"},
			Group:           &service.Group{Name: "CHATGPT"},
		}},
		usagePage: &pagination.PaginationResult{Total: 1},
	}
	router, handler := newIntegrationUserTestRouter(svc)
	handler.usageUSDToCNYRate = func(context.Context) float64 { return 7.2 }

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/integrations/users/u-1/usage", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data.Items, 1)
	item := body.Data.Items[0]
	require.Equal(t, "CHATGPT", item["api_key_name"])
	require.Equal(t, "CHATGPT", item["group_name"])
	require.Equal(t, "Medium", item["reasoning_effort"])
	require.Equal(t, "流式", item["request_type"])
	require.Equal(t, "按量", item["billing_type"])
	require.Equal(t, 0.384026, item["actual_cost_cny"])
	require.NotContains(t, item, "group_id")
	require.NotContains(t, item, "api_key_id")
}

func TestIntegrationUserHandler_ListSubscriptionsIncludesProgress(t *testing.T) {
	svc := &integrationUserServiceStub{subscriptionDetails: []service.ExternalUserSubscriptionDetail{{
		Subscription: service.UserSubscription{ID: 88, UserID: 10, GroupID: 15, Status: service.SubscriptionStatusActive},
		Progress: &service.SubscriptionProgress{
			Daily:   &service.UsageWindowProgress{LimitUSD: 10, UsedUSD: 3},
			Weekly:  &service.UsageWindowProgress{LimitUSD: 50, UsedUSD: 12},
			Monthly: &service.UsageWindowProgress{LimitUSD: 200, UsedUSD: 31},
		},
	}}}
	router, _ := newIntegrationUserTestRouter(svc)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/integrations/users/u-1/subscriptions", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	require.Equal(t, float64(10), body.Data[0]["daily"].(map[string]any)["limit_usd"])
	require.Equal(t, float64(12), body.Data[0]["weekly"].(map[string]any)["used_usd"])
	require.Equal(t, float64(31), body.Data[0]["monthly"].(map[string]any)["used_usd"])
	require.NotContains(t, body.Data[0], "progress")
}

func newIntegrationUserTestRouter(svc *integrationUserServiceStub) (*gin.Engine, *IntegrationUserHandler) {
	gin.SetMode(gin.TestMode)
	h := NewIntegrationUserHandler(svc)
	router := gin.New()
	router.POST("/integrations/users", h.Create)
	router.DELETE("/integrations/users", h.DeleteAll)
	router.DELETE("/integrations/users/:external_user_id", h.DeleteByExternalID)
	router.POST("/integrations/users/:external_user_id/api-keys/rotate", h.RotateAPIKeys)
	router.GET("/integrations/users/:external_user_id/usage", h.ListUsage)
	router.GET("/integrations/users/:external_user_id/subscriptions", h.ListSubscriptions)
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

	rotateExternalUserID string

	usageLogs           []service.UsageLog
	usagePage           *pagination.PaginationResult
	subscriptionDetails []service.ExternalUserSubscriptionDetail
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

func (s *integrationUserServiceStub) RotateAPIKeysByExternalID(_ context.Context, externalUserID string) (*service.ExternalUserRotateAPIKeysResult, error) {
	s.rotateExternalUserID = externalUserID
	return &service.ExternalUserRotateAPIKeysResult{ExternalUserID: externalUserID}, nil
}

func (s *integrationUserServiceStub) ListSubscriptionsByExternalID(context.Context, string) ([]service.UserSubscription, error) {
	return nil, nil
}

func (s *integrationUserServiceStub) ListSubscriptionDetailsByExternalID(context.Context, string) ([]service.ExternalUserSubscriptionDetail, error) {
	return s.subscriptionDetails, nil
}

func (s *integrationUserServiceStub) ListUsageByExternalID(context.Context, string, pagination.PaginationParams, usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	return s.usageLogs, s.usagePage, nil
}
