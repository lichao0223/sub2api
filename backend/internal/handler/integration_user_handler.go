package handler

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	externalUserIDMaxLen         = 255
	externalOrganizationIDMaxLen = 255
	externalUsernameMaxLen       = 100
	externalBatchIDMaxLen        = 128
)

type IntegrationHandlers struct {
	User *IntegrationUserHandler
}

type externalUserServicePort interface {
	Create(c context.Context, input service.ExternalUserInput) (*service.ExternalUserResult, error)
	DeleteByExternalID(c context.Context, externalUserID string) (*service.ExternalUserDeleteResult, error)
	DeleteAll(c context.Context) (*service.ExternalUserDeleteAllResult, error)
	Sync(c context.Context, input service.ExternalUserSyncInput) (*service.ExternalUserSyncResult, error)
	RotateAPIKeysByExternalID(c context.Context, externalUserID string) (*service.ExternalUserRotateAPIKeysResult, error)
}

type externalUserQueryPort interface {
	ListSubscriptionsByExternalID(context.Context, string) ([]service.UserSubscription, error)
	ListUsageByExternalID(context.Context, string, pagination.PaginationParams, usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error)
}

func (h *IntegrationUserHandler) ListSubscriptions(c *gin.Context) {
	queryService, ok := h.externalUserService.(externalUserQueryPort)
	if !ok {
		response.Error(c, 503, "integration query service not available")
		return
	}
	externalUserID, ok := validateExternalUserIDParam(c)
	if !ok {
		return
	}
	subs, err := queryService.ListSubscriptionsByExternalID(c.Request.Context(), externalUserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.AdminUserSubscription, 0, len(subs))
	for i := range subs {
		out = append(out, *dto.UserSubscriptionFromServiceAdmin(&subs[i]))
	}
	response.Success(c, out)
}

func (h *IntegrationUserHandler) ListUsage(c *gin.Context) {
	queryService, ok := h.externalUserService.(externalUserQueryPort)
	if !ok {
		response.Error(c, 503, "integration query service not available")
		return
	}
	externalUserID, ok := validateExternalUserIDParam(c)
	if !ok {
		return
	}
	filters, ok := parseIntegrationUsageFilters(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: c.DefaultQuery("sort_by", "created_at"), SortOrder: c.DefaultQuery("sort_order", "desc")}
	logs, result, err := queryService.ListUsageByExternalID(c.Request.Context(), externalUserID, params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.UsageLog, 0, len(logs))
	for i := range logs {
		out = append(out, *dto.UsageLogFromService(&logs[i]))
	}
	response.Paginated(c, out, result.Total, page, pageSize)
}

func validateExternalUserIDParam(c *gin.Context) (string, bool) {
	id := strings.TrimSpace(c.Param("external_user_id"))
	if id == "" || len(id) > externalUserIDMaxLen {
		response.ErrorFrom(c, invalidExternalUserArgument("external_user_id", "invalid external_user_id"))
		return "", false
	}
	return id, true
}

func parseIntegrationUsageFilters(c *gin.Context) (usagestats.UsageLogFilters, bool) {
	var f usagestats.UsageLogFilters
	f.Model = strings.TrimSpace(c.Query("model"))
	f.ModelFilterSource = usagestats.ModelSourceRequested
	f.BillingMode = strings.TrimSpace(c.Query("billing_mode"))
	if f.BillingMode != "" && !service.BillingMode(f.BillingMode).IsValidUsageFilter() {
		response.BadRequest(c, "Invalid billing_mode")
		return f, false
	}
	if v := strings.TrimSpace(c.Query("api_key_id")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid api_key_id")
			return f, false
		}
		f.APIKeyID = n
	}
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid group_id")
			return f, false
		}
		f.GroupID = n
	}
	if v := strings.TrimSpace(c.Query("request_type")); v != "" {
		n, err := service.ParseUsageRequestType(v)
		if err != nil {
			response.BadRequest(c, err.Error())
			return f, false
		}
		x := int16(n)
		f.RequestType = &x
	} else if v := strings.TrimSpace(c.Query("stream")); v != "" {
		x, err := strconv.ParseBool(v)
		if err != nil {
			response.BadRequest(c, "Invalid stream value, use true or false")
			return f, false
		}
		f.Stream = &x
	}
	if v := strings.TrimSpace(c.Query("billing_type")); v != "" {
		n, err := strconv.ParseInt(v, 10, 8)
		if err != nil {
			response.BadRequest(c, "Invalid billing_type")
			return f, false
		}
		x := int8(n)
		f.BillingType = &x
	}
	userTZ := c.Query("timezone")
	if v := strings.TrimSpace(c.Query("start_date")); v != "" {
		t, err := timezone.ParseInUserLocation("2006-01-02", v, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid start_date format, use YYYY-MM-DD")
			return f, false
		}
		f.StartTime = &t
	}
	if v := strings.TrimSpace(c.Query("end_date")); v != "" {
		t, err := timezone.ParseInUserLocation("2006-01-02", v, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid end_date format, use YYYY-MM-DD")
			return f, false
		}
		t = t.AddDate(0, 0, 1)
		f.EndTime = &t
	}
	return f, true
}

type IntegrationUserHandler struct {
	externalUserService externalUserServicePort
}

func NewIntegrationUserHandler(externalUserService externalUserServicePort) *IntegrationUserHandler {
	return &IntegrationUserHandler{externalUserService: externalUserService}
}

func ProvideIntegrationUserHandler(externalUserService *service.ExternalUserService) *IntegrationUserHandler {
	return NewIntegrationUserHandler(externalUserService)
}

func ProvideIntegrationHandlers(userHandler *IntegrationUserHandler) *IntegrationHandlers {
	return &IntegrationHandlers{User: userHandler}
}

type externalUserRequest struct {
	ExternalUserID         string `json:"external_user_id"`
	ExternalOrganizationID string `json:"external_organization_id"`
	Username               string `json:"username"`
}

type externalUserSyncRequest struct {
	BatchID string                `json:"batch_id"`
	Users   []externalUserRequest `json:"users"`
}

func (h *IntegrationUserHandler) Create(c *gin.Context) {
	var req externalUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeExternalUserJSONError(c, err)
		return
	}

	input, err := validateExternalUserRequest(req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result, err := h.externalUserService.Create(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if result.Status == service.ExternalUserStatusCreated {
		response.Created(c, result)
		return
	}
	response.Success(c, result)
}

func (h *IntegrationUserHandler) DeleteByExternalID(c *gin.Context) {
	externalUserID := strings.TrimSpace(c.Param("external_user_id"))
	if externalUserID == "" {
		response.ErrorFrom(c, invalidExternalUserArgument("external_user_id", "external_user_id is required"))
		return
	}
	if len(externalUserID) > externalUserIDMaxLen {
		response.ErrorFrom(c, invalidExternalUserArgument("external_user_id", "external_user_id is too long"))
		return
	}

	result, err := h.externalUserService.DeleteByExternalID(c.Request.Context(), externalUserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *IntegrationUserHandler) DeleteAll(c *gin.Context) {
	result, err := h.externalUserService.DeleteAll(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *IntegrationUserHandler) RotateAPIKeys(c *gin.Context) {
	externalUserID := strings.TrimSpace(c.Param("external_user_id"))
	if externalUserID == "" || len(externalUserID) > externalUserIDMaxLen {
		response.ErrorFrom(c, invalidExternalUserArgument("external_user_id", "invalid external_user_id"))
		return
	}
	result, err := h.externalUserService.RotateAPIKeysByExternalID(c.Request.Context(), externalUserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *IntegrationUserHandler) Sync(c *gin.Context) {
	var req externalUserSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeExternalUserJSONError(c, err)
		return
	}

	input, err := validateExternalUserSyncRequest(req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result, err := h.externalUserService.Sync(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func validateExternalUserRequest(req externalUserRequest) (service.ExternalUserInput, error) {
	externalUserID := strings.TrimSpace(req.ExternalUserID)
	externalOrganizationID := strings.TrimSpace(req.ExternalOrganizationID)
	username := strings.TrimSpace(req.Username)

	if externalUserID == "" {
		return service.ExternalUserInput{}, invalidExternalUserArgument("external_user_id", "external_user_id is required")
	}
	if len(externalUserID) > externalUserIDMaxLen {
		return service.ExternalUserInput{}, invalidExternalUserArgument("external_user_id", "external_user_id is too long")
	}
	if externalOrganizationID == "" {
		return service.ExternalUserInput{}, invalidExternalUserArgument("external_organization_id", "external_organization_id is required")
	}
	if len(externalOrganizationID) > externalOrganizationIDMaxLen {
		return service.ExternalUserInput{}, invalidExternalUserArgument("external_organization_id", "external_organization_id is too long")
	}
	if username == "" {
		return service.ExternalUserInput{}, invalidExternalUserArgument("username", "username is required")
	}
	if len(username) > externalUsernameMaxLen {
		return service.ExternalUserInput{}, invalidExternalUserArgument("username", "username is too long")
	}

	return service.ExternalUserInput{
		ExternalUserID:         externalUserID,
		ExternalOrganizationID: externalOrganizationID,
		Username:               username,
	}, nil
}

func validateExternalUserSyncRequest(req externalUserSyncRequest) (service.ExternalUserSyncInput, error) {
	batchID := strings.TrimSpace(req.BatchID)
	if len(batchID) > externalBatchIDMaxLen {
		return service.ExternalUserSyncInput{}, invalidExternalUserArgument("batch_id", "batch_id is too long")
	}
	if len(req.Users) == 0 {
		return service.ExternalUserSyncInput{}, invalidExternalUserArgument("users", "users is required")
	}
	if len(req.Users) > service.ExternalUserMaxBatchSize() {
		return service.ExternalUserSyncInput{}, service.ErrExternalUserBatchTooLarge.WithMetadata(map[string]string{
			"limit": "500",
		})
	}

	users := make([]service.ExternalUserInput, 0, len(req.Users))
	seen := make(map[string]struct{}, len(req.Users))
	for _, userReq := range req.Users {
		user, err := validateExternalUserRequest(userReq)
		if err != nil {
			return service.ExternalUserSyncInput{}, err
		}
		if _, ok := seen[user.ExternalUserID]; ok {
			return service.ExternalUserSyncInput{}, service.ErrExternalUserDuplicateID.WithMetadata(map[string]string{
				"external_user_id": user.ExternalUserID,
			})
		}
		seen[user.ExternalUserID] = struct{}{}
		users = append(users, user)
	}

	return service.ExternalUserSyncInput{
		BatchID: batchID,
		Users:   users,
	}, nil
}

func writeExternalUserJSONError(c *gin.Context, err error) {
	message := "invalid json"
	if errors.Is(err, io.EOF) {
		message = "request body is required"
	}
	response.ErrorFrom(c, service.ErrExternalUserInvalidJSON.WithCause(err).WithMetadata(map[string]string{
		"message": message,
	}))
}

func invalidExternalUserArgument(field, message string) error {
	return service.ErrExternalUserInvalidArgument.WithMetadata(map[string]string{
		"field":   field,
		"message": message,
	})
}
