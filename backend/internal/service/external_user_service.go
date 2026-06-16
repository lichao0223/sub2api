package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	ExternalUserStatusCreated = "created"
	ExternalUserStatusSkipped = "skipped"
	ExternalUserStatusDeleted = "deleted"
	ExternalUserStatusFailed  = "failed"

	externalUserDefaultBalance     = 100000
	externalUserDefaultConcurrency = 2
	externalUserMaxBatchSize       = 500
)

var (
	ErrExternalUserInvalidArgument = infraerrors.BadRequest("INVALID_ARGUMENT", "invalid argument")
	ErrExternalUserInvalidJSON     = infraerrors.BadRequest("INVALID_JSON", "invalid json")
	ErrExternalUserBatchTooLarge   = infraerrors.BadRequest("BATCH_TOO_LARGE", "batch size exceeds limit")
	ErrExternalUserDuplicateID     = infraerrors.BadRequest("DUPLICATE_EXTERNAL_USER_ID", "duplicate external_user_id in request")

	ErrExternalUserMappingNotFound = infraerrors.NotFound("EXTERNAL_USER_NOT_FOUND", "external user mapping not found")
	ErrExternalUserMappingExists   = infraerrors.Conflict("EXTERNAL_USER_EXISTS", "external user mapping already exists")

	ErrExternalUserNoActiveGroup       = infraerrors.InternalServer("NO_ACTIVE_GROUP", "no active group available")
	ErrExternalUserCreateUserFailed    = infraerrors.InternalServer("CREATE_USER_FAILED", "create user failed")
	ErrExternalUserCreateAPIKeyFailed  = infraerrors.InternalServer("CREATE_API_KEY_FAILED", "create api key failed")
	ErrExternalUserCreateMappingFailed = infraerrors.InternalServer("CREATE_MAPPING_FAILED", "create mapping failed")
	ErrExternalUserDeleteFailed        = infraerrors.InternalServer("DELETE_USER_FAILED", "delete user failed")
	ErrExternalUserInternal            = infraerrors.InternalServer("INTERNAL_ERROR", "internal error")
)

type ExternalUserMapping struct {
	ID               int64
	ExternalUserID   string
	UserID           int64
	APIKeyID         int64
	UsernameSnapshot string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

type ExternalUserMappingRepository interface {
	GetByExternalUserID(ctx context.Context, externalUserID string) (*ExternalUserMapping, error)
	Create(ctx context.Context, mapping *ExternalUserMapping) error
	UpdateAPIKeyID(ctx context.Context, id int64, apiKeyID int64) error
	SoftDeleteByExternalUserID(ctx context.Context, externalUserID string) error
}

type externalUserAdminPort interface {
	CreateUser(ctx context.Context, input *CreateUserInput) (*User, error)
	DeleteUser(ctx context.Context, id int64) error
	GetAllGroups(ctx context.Context) ([]Group, error)
	GetUser(ctx context.Context, id int64) (*User, error)
}

type externalUserAPIKeyPort interface {
	Create(ctx context.Context, userID int64, req CreateAPIKeyRequest) (*APIKey, error)
	GetByID(ctx context.Context, id int64) (*APIKey, error)
}

type ExternalUserService struct {
	adminService  externalUserAdminPort
	apiKeyService externalUserAPIKeyPort
	mappingRepo   ExternalUserMappingRepository
}

func NewExternalUserService(
	adminService AdminService,
	apiKeyService *APIKeyService,
	mappingRepo ExternalUserMappingRepository,
) *ExternalUserService {
	return &ExternalUserService{
		adminService:  adminService,
		apiKeyService: apiKeyService,
		mappingRepo:   mappingRepo,
	}
}

type ExternalUserInput struct {
	ExternalUserID string
	Username       string
}

type ExternalUserSyncInput struct {
	BatchID string
	Users   []ExternalUserInput
}

type ExternalUserResult struct {
	Status         string                  `json:"status"`
	ExternalUserID string                  `json:"external_user_id"`
	User           *ExternalUserUserInfo   `json:"user,omitempty"`
	APIKey         *ExternalUserAPIKeyInfo `json:"api_key,omitempty"`
	Error          *ExternalUserItemError  `json:"error,omitempty"`
}

type ExternalUserUserInfo struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

type ExternalUserAPIKeyInfo struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Key     string `json:"key"`
	GroupID *int64 `json:"group_id,omitempty"`
	Status  string `json:"status"`
}

type ExternalUserDeleteResult struct {
	Status         string `json:"status"`
	ExternalUserID string `json:"external_user_id"`
	UserID         int64  `json:"user_id"`
}

type ExternalUserSyncSummary struct {
	Total   int `json:"total"`
	Created int `json:"created"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

type ExternalUserSyncResult struct {
	BatchID string                  `json:"batch_id,omitempty"`
	Summary ExternalUserSyncSummary `json:"summary"`
	Items   []ExternalUserResult    `json:"items"`
}

type ExternalUserItemError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (s *ExternalUserService) Create(ctx context.Context, input ExternalUserInput) (*ExternalUserResult, error) {
	input.ExternalUserID = strings.TrimSpace(input.ExternalUserID)
	input.Username = strings.TrimSpace(input.Username)

	if mapping, err := s.mappingRepo.GetByExternalUserID(ctx, input.ExternalUserID); err == nil {
		return s.buildExistingResult(ctx, input.ExternalUserID, input.Username, mapping)
	} else if !errors.Is(err, ErrExternalUserMappingNotFound) {
		return nil, ErrExternalUserInternal.WithCause(err)
	}

	group, err := s.firstActiveGroup(ctx)
	if err != nil {
		return nil, err
	}

	balance := float64(externalUserDefaultBalance)
	user, err := s.adminService.CreateUser(ctx, &CreateUserInput{
		Email:         generatedExternalEmail(input.ExternalUserID),
		Password:      generatedExternalPassword(),
		Username:      input.Username,
		Balance:       &balance,
		Concurrency:   externalUserDefaultConcurrency,
		AllowedGroups: []int64{group.ID},
	})
	if err != nil {
		return nil, ErrExternalUserCreateUserFailed.WithCause(err)
	}

	apiKey, err := s.createDefaultAPIKey(ctx, user.ID, input.Username, group.ID)
	if err != nil {
		_ = s.adminService.DeleteUser(ctx, user.ID)
		return nil, ErrExternalUserCreateAPIKeyFailed.WithCause(err)
	}

	mapping := &ExternalUserMapping{
		ExternalUserID:   input.ExternalUserID,
		UserID:           user.ID,
		APIKeyID:         apiKey.ID,
		UsernameSnapshot: input.Username,
	}
	if err := s.mappingRepo.Create(ctx, mapping); err != nil {
		_ = s.adminService.DeleteUser(ctx, user.ID)
		if errors.Is(err, ErrExternalUserMappingExists) {
			existing, getErr := s.mappingRepo.GetByExternalUserID(ctx, input.ExternalUserID)
			if getErr == nil {
				return s.buildExistingResult(ctx, input.ExternalUserID, input.Username, existing)
			}
			return nil, ErrExternalUserMappingExists.WithCause(err)
		}
		return nil, ErrExternalUserCreateMappingFailed.WithCause(err)
	}

	return &ExternalUserResult{
		Status:         ExternalUserStatusCreated,
		ExternalUserID: input.ExternalUserID,
		User:           externalUserInfoFromService(user),
		APIKey:         externalAPIKeyInfoFromService(apiKey),
	}, nil
}

func (s *ExternalUserService) DeleteByExternalID(ctx context.Context, externalUserID string) (*ExternalUserDeleteResult, error) {
	externalUserID = strings.TrimSpace(externalUserID)
	mapping, err := s.mappingRepo.GetByExternalUserID(ctx, externalUserID)
	if err != nil {
		if errors.Is(err, ErrExternalUserMappingNotFound) {
			return nil, err
		}
		return nil, ErrExternalUserInternal.WithCause(err)
	}

	if err := s.adminService.DeleteUser(ctx, mapping.UserID); err != nil {
		return nil, ErrExternalUserDeleteFailed.WithCause(err)
	}
	if err := s.mappingRepo.SoftDeleteByExternalUserID(ctx, externalUserID); err != nil {
		return nil, ErrExternalUserDeleteFailed.WithCause(err)
	}

	return &ExternalUserDeleteResult{
		Status:         ExternalUserStatusDeleted,
		ExternalUserID: externalUserID,
		UserID:         mapping.UserID,
	}, nil
}

func (s *ExternalUserService) Sync(ctx context.Context, input ExternalUserSyncInput) (*ExternalUserSyncResult, error) {
	result := &ExternalUserSyncResult{
		BatchID: strings.TrimSpace(input.BatchID),
		Items:   make([]ExternalUserResult, 0, len(input.Users)),
	}
	result.Summary.Total = len(input.Users)

	for _, user := range input.Users {
		item, err := s.Create(ctx, user)
		if err != nil {
			item = &ExternalUserResult{
				Status:         ExternalUserStatusFailed,
				ExternalUserID: strings.TrimSpace(user.ExternalUserID),
				Error:          externalUserErrorItem(err),
			}
		}
		result.Items = append(result.Items, *item)
		switch item.Status {
		case ExternalUserStatusCreated:
			result.Summary.Created++
		case ExternalUserStatusSkipped:
			result.Summary.Skipped++
		default:
			result.Summary.Failed++
		}
	}
	return result, nil
}

func (s *ExternalUserService) buildExistingResult(ctx context.Context, externalUserID, username string, mapping *ExternalUserMapping) (*ExternalUserResult, error) {
	user, err := s.adminService.GetUser(ctx, mapping.UserID)
	if err != nil {
		return nil, ErrExternalUserInternal.WithCause(err)
	}

	apiKey, err := s.apiKeyService.GetByID(ctx, mapping.APIKeyID)
	if err != nil {
		if !errors.Is(err, ErrAPIKeyNotFound) {
			return nil, ErrExternalUserInternal.WithCause(err)
		}
	}
	if err != nil || !apiKey.IsActive() {
		group, groupErr := s.firstActiveGroup(ctx)
		if groupErr != nil {
			return nil, groupErr
		}
		apiKey, err = s.createDefaultAPIKey(ctx, mapping.UserID, username, group.ID)
		if err != nil {
			return nil, ErrExternalUserCreateAPIKeyFailed.WithCause(err)
		}
		if err := s.mappingRepo.UpdateAPIKeyID(ctx, mapping.ID, apiKey.ID); err != nil {
			return nil, ErrExternalUserInternal.WithCause(err)
		}
	}

	return &ExternalUserResult{
		Status:         ExternalUserStatusSkipped,
		ExternalUserID: externalUserID,
		User:           externalUserInfoFromService(user),
		APIKey:         externalAPIKeyInfoFromService(apiKey),
	}, nil
}

func (s *ExternalUserService) firstActiveGroup(ctx context.Context) (*Group, error) {
	groups, err := s.adminService.GetAllGroups(ctx)
	if err != nil {
		return nil, ErrExternalUserInternal.WithCause(err)
	}
	if len(groups) == 0 {
		return nil, ErrExternalUserNoActiveGroup
	}
	return &groups[0], nil
}

func (s *ExternalUserService) createDefaultAPIKey(ctx context.Context, userID int64, username string, groupID int64) (*APIKey, error) {
	return s.apiKeyService.Create(ctx, userID, CreateAPIKeyRequest{
		Name:    username,
		GroupID: &groupID,
	})
}

func externalUserInfoFromService(user *User) *ExternalUserUserInfo {
	if user == nil {
		return nil
	}
	return &ExternalUserUserInfo{
		ID:       user.ID,
		Email:    user.Email,
		Username: user.Username,
	}
}

func externalAPIKeyInfoFromService(key *APIKey) *ExternalUserAPIKeyInfo {
	if key == nil {
		return nil
	}
	return &ExternalUserAPIKeyInfo{
		ID:      key.ID,
		Name:    key.Name,
		Key:     key.Key,
		GroupID: key.GroupID,
		Status:  key.Status,
	}
}

func externalUserErrorItem(err error) *ExternalUserItemError {
	if err == nil {
		return nil
	}
	appErr := infraerrors.FromError(err)
	return &ExternalUserItemError{
		Code:    appErr.Reason,
		Message: appErr.Message,
	}
}

func generatedExternalEmail(externalUserID string) string {
	sum := sha256.Sum256([]byte(externalUserID))
	return fmt.Sprintf("ext+%s@external.local", hex.EncodeToString(sum[:])[:16])
}

func generatedExternalPassword() string {
	var buf [24]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(buf[:])
}

func ExternalUserMaxBatchSize() int {
	return externalUserMaxBatchSize
}
