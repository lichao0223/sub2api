package service

import (
	"context"
	"errors"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
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
	ID                     int64
	ExternalUserID         string
	ExternalOrganizationID string
	UserID                 int64
	APIKeyID               int64
	UsernameSnapshot       string
	CreatedAt              time.Time
	UpdatedAt              time.Time
	DeletedAt              *time.Time
}

type ExternalUserMappingRepository interface {
	GetByExternalUserID(ctx context.Context, externalUserID string) (*ExternalUserMapping, error)
	ListActive(ctx context.Context) ([]ExternalUserMapping, error)
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
	List(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error)
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
	ExternalUserID         string
	ExternalOrganizationID string
	Username               string
}

type ExternalUserSyncInput struct {
	BatchID string
	Users   []ExternalUserInput
}

type ExternalUserResult struct {
	Status                 string                   `json:"status"`
	ExternalUserID         string                   `json:"external_user_id"`
	ExternalOrganizationID string                   `json:"external_organization_id,omitempty"`
	User                   *ExternalUserUserInfo    `json:"user,omitempty"`
	APIKeys                []ExternalUserAPIKeyInfo `json:"api_keys,omitempty"`
	Error                  *ExternalUserItemError   `json:"error,omitempty"`
}

type ExternalUserUserInfo struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

type ExternalUserAPIKeyInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	GroupID   *int64 `json:"group_id,omitempty"`
	GroupName string `json:"group_name,omitempty"`
	Platform  string `json:"platform,omitempty"`
	Status    string `json:"status"`
}

type ExternalUserDeleteResult struct {
	Status         string `json:"status"`
	ExternalUserID string `json:"external_user_id"`
	UserID         int64  `json:"user_id"`
}

type ExternalUserDeleteAllResult struct {
	Summary ExternalUserDeleteAllSummary `json:"summary"`
	Items   []ExternalUserDeleteResult   `json:"items"`
	Errors  []ExternalUserDeleteError    `json:"errors,omitempty"`
}

type ExternalUserDeleteAllSummary struct {
	Total   int `json:"total"`
	Deleted int `json:"deleted"`
	Failed  int `json:"failed"`
}

type ExternalUserDeleteError struct {
	ExternalUserID string                 `json:"external_user_id"`
	UserID         int64                  `json:"user_id,omitempty"`
	Error          *ExternalUserItemError `json:"error"`
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
	input.ExternalOrganizationID = strings.TrimSpace(input.ExternalOrganizationID)
	input.Username = strings.TrimSpace(input.Username)

	if mapping, err := s.mappingRepo.GetByExternalUserID(ctx, input.ExternalUserID); err == nil {
		return s.buildExistingResult(ctx, input.ExternalUserID, input.ExternalOrganizationID, input.Username, mapping)
	} else if !errors.Is(err, ErrExternalUserMappingNotFound) {
		return nil, ErrExternalUserInternal.WithCause(err)
	}

	groups, err := s.activeNonExclusiveGroups(ctx)
	if err != nil {
		return nil, err
	}
	allowedGroups := externalUserGroupIDs(groups)

	balance := float64(externalUserDefaultBalance)
	user, err := s.adminService.CreateUser(ctx, &CreateUserInput{
		Email:         generatedExternalEmail(input.ExternalUserID),
		Password:      generatedExternalPassword(input.ExternalUserID),
		Username:      input.Username,
		Balance:       &balance,
		Concurrency:   externalUserDefaultConcurrency,
		AllowedGroups: allowedGroups,
	})
	if err != nil {
		return nil, ErrExternalUserCreateUserFailed.WithCause(err)
	}

	apiKeys, err := s.createDefaultAPIKeys(ctx, user.ID, input.Username, groups, nil)
	if err != nil {
		_ = s.adminService.DeleteUser(ctx, user.ID)
		return nil, ErrExternalUserCreateAPIKeyFailed.WithCause(err)
	}
	firstKey := externalUserFirstAPIKey(apiKeys)

	mapping := &ExternalUserMapping{
		ExternalUserID:         input.ExternalUserID,
		ExternalOrganizationID: input.ExternalOrganizationID,
		UserID:                 user.ID,
		APIKeyID:               firstKey.ID,
		UsernameSnapshot:       input.Username,
	}
	if err := s.mappingRepo.Create(ctx, mapping); err != nil {
		_ = s.adminService.DeleteUser(ctx, user.ID)
		if errors.Is(err, ErrExternalUserMappingExists) {
			existing, getErr := s.mappingRepo.GetByExternalUserID(ctx, input.ExternalUserID)
			if getErr == nil {
				return s.buildExistingResult(ctx, input.ExternalUserID, input.ExternalOrganizationID, input.Username, existing)
			}
			return nil, ErrExternalUserMappingExists.WithCause(err)
		}
		return nil, ErrExternalUserCreateMappingFailed.WithCause(err)
	}

	return &ExternalUserResult{
		Status:                 ExternalUserStatusCreated,
		ExternalUserID:         input.ExternalUserID,
		ExternalOrganizationID: input.ExternalOrganizationID,
		User:                   externalUserInfoFromService(user),
		APIKeys:                externalAPIKeyInfosFromService(apiKeys),
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

	if err := s.adminService.DeleteUser(ctx, mapping.UserID); err != nil && !errors.Is(err, ErrUserNotFound) {
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

func (s *ExternalUserService) DeleteAll(ctx context.Context) (*ExternalUserDeleteAllResult, error) {
	mappings, err := s.mappingRepo.ListActive(ctx)
	if err != nil {
		return nil, ErrExternalUserInternal.WithCause(err)
	}

	result := &ExternalUserDeleteAllResult{
		Items:  make([]ExternalUserDeleteResult, 0, len(mappings)),
		Errors: make([]ExternalUserDeleteError, 0),
	}
	result.Summary.Total = len(mappings)

	for _, mapping := range mappings {
		item, err := s.DeleteByExternalID(ctx, mapping.ExternalUserID)
		if err != nil {
			result.Summary.Failed++
			result.Errors = append(result.Errors, ExternalUserDeleteError{
				ExternalUserID: mapping.ExternalUserID,
				UserID:         mapping.UserID,
				Error:          externalUserErrorItem(err),
			})
			continue
		}
		result.Summary.Deleted++
		result.Items = append(result.Items, *item)
	}

	return result, nil
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

func (s *ExternalUserService) buildExistingResult(ctx context.Context, externalUserID, externalOrganizationID, username string, mapping *ExternalUserMapping) (*ExternalUserResult, error) {
	user, err := s.adminService.GetUser(ctx, mapping.UserID)
	if err != nil {
		return nil, ErrExternalUserInternal.WithCause(err)
	}

	groups, err := s.activeNonExclusiveGroups(ctx)
	if err != nil {
		return nil, err
	}
	existingKeys, err := s.listUserAPIKeys(ctx, mapping.UserID)
	if err != nil {
		return nil, err
	}
	apiKeys, err := s.createDefaultAPIKeys(ctx, mapping.UserID, firstExternalUserNonEmpty(username, mapping.UsernameSnapshot, user.Username), groups, existingKeys)
	if err != nil {
		return nil, ErrExternalUserCreateAPIKeyFailed.WithCause(err)
	}
	firstKey := externalUserFirstAPIKey(apiKeys)
	if firstKey != nil && firstKey.ID != mapping.APIKeyID {
		if err := s.mappingRepo.UpdateAPIKeyID(ctx, mapping.ID, firstKey.ID); err != nil {
			return nil, ErrExternalUserInternal.WithCause(err)
		}
	}

	return &ExternalUserResult{
		Status:                 ExternalUserStatusSkipped,
		ExternalUserID:         externalUserID,
		ExternalOrganizationID: firstExternalUserNonEmpty(mapping.ExternalOrganizationID, externalOrganizationID),
		User:                   externalUserInfoFromService(user),
		APIKeys:                externalAPIKeyInfosFromService(apiKeys),
	}, nil
}

func (s *ExternalUserService) activeNonExclusiveGroups(ctx context.Context) ([]Group, error) {
	groups, err := s.adminService.GetAllGroups(ctx)
	if err != nil {
		return nil, ErrExternalUserInternal.WithCause(err)
	}
	out := make([]Group, 0, len(groups))
	for _, group := range groups {
		if group.IsActive() && !group.IsExclusive {
			out = append(out, group)
		}
	}
	if len(out) == 0 {
		return nil, ErrExternalUserNoActiveGroup
	}
	return out, nil
}

func (s *ExternalUserService) createDefaultAPIKey(ctx context.Context, userID int64, name string, groupID int64) (*APIKey, error) {
	return s.apiKeyService.Create(ctx, userID, CreateAPIKeyRequest{
		Name:    name,
		GroupID: &groupID,
	})
}

func (s *ExternalUserService) listUserAPIKeys(ctx context.Context, userID int64) ([]APIKey, error) {
	keys, _, err := s.apiKeyService.List(ctx, userID, pagination.PaginationParams{
		Page:      1,
		PageSize:  1000,
		SortBy:    "created_at",
		SortOrder: "asc",
	}, APIKeyListFilters{})
	if err != nil {
		return nil, ErrExternalUserInternal.WithCause(err)
	}
	return keys, nil
}

func (s *ExternalUserService) createDefaultAPIKeys(ctx context.Context, userID int64, username string, groups []Group, existing []APIKey) ([]*APIKey, error) {
	byGroup := make(map[int64]*APIKey, len(existing))
	defaultGroupIDs := make(map[int64]struct{}, len(groups))
	for i := range existing {
		key := existing[i]
		if key.GroupID == nil || !key.IsActive() {
			continue
		}
		byGroup[*key.GroupID] = &key
	}

	keys := make([]*APIKey, 0, len(groups))
	for i := range groups {
		group := groups[i]
		defaultGroupIDs[group.ID] = struct{}{}
		if key := byGroup[group.ID]; key != nil {
			attachExternalUserGroup(key, &group)
			keys = append(keys, key)
			continue
		}
		key, err := s.createDefaultAPIKey(ctx, userID, externalUserAPIKeyName(username, group.Name), group.ID)
		if err != nil {
			return nil, err
		}
		attachExternalUserGroup(key, &group)
		keys = append(keys, key)
	}
	keys = appendExistingNonDefaultAPIKeys(keys, existing, defaultGroupIDs)
	return keys, nil
}

func appendExistingNonDefaultAPIKeys(keys []*APIKey, existing []APIKey, defaultGroupIDs map[int64]struct{}) []*APIKey {
	seen := make(map[int64]struct{}, len(keys))
	for _, key := range keys {
		if key != nil {
			seen[key.ID] = struct{}{}
		}
	}
	for i := range existing {
		key := existing[i]
		if !key.IsActive() {
			continue
		}
		if _, ok := seen[key.ID]; ok {
			continue
		}
		if key.GroupID != nil {
			if _, ok := defaultGroupIDs[*key.GroupID]; ok {
				continue
			}
		}
		keys = append(keys, &key)
		seen[key.ID] = struct{}{}
	}
	return keys
}

func externalUserAPIKeyName(username, groupName string) string {
	return strings.TrimSpace(username) + strings.TrimSpace(groupName)
}

func externalUserGroupIDs(groups []Group) []int64 {
	ids := make([]int64, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	return ids
}

func attachExternalUserGroup(key *APIKey, group *Group) {
	if key == nil || group == nil {
		return
	}
	g := *group
	key.Group = &g
	if key.GroupID == nil {
		key.GroupID = &g.ID
	}
}

func externalUserFirstAPIKey(keys []*APIKey) *APIKey {
	if len(keys) == 0 {
		return nil
	}
	return keys[0]
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
	info := &ExternalUserAPIKeyInfo{
		ID:      key.ID,
		Name:    key.Name,
		Key:     key.Key,
		GroupID: key.GroupID,
		Status:  key.Status,
	}
	if key.Group != nil {
		info.GroupName = key.Group.Name
		info.Platform = key.Group.Platform
	}
	return info
}

func externalAPIKeyInfosFromService(keys []*APIKey) []ExternalUserAPIKeyInfo {
	out := make([]ExternalUserAPIKeyInfo, 0, len(keys))
	for _, key := range keys {
		if item := externalAPIKeyInfoFromService(key); item != nil {
			out = append(out, *item)
		}
	}
	return out
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
	return strings.TrimSpace(externalUserID) + "@sub.com"
}

func generatedExternalPassword(externalUserID string) string {
	return strings.TrimSpace(externalUserID)
}

func firstExternalUserNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func ExternalUserMaxBatchSize() int {
	return externalUserMaxBatchSize
}
