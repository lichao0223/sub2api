package handler

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
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
