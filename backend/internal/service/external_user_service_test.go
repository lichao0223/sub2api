//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestExternalUserService_Create_CreatesUserWithDefaultsAndAPIKey(t *testing.T) {
	admin := &externalUserAdminStub{
		groups: []Group{
			{ID: 7, Name: "公共一", Status: StatusActive},
			{ID: 8, Name: "专属", Status: StatusActive, IsExclusive: true},
			{ID: 9, Name: "公共二", Status: StatusActive},
		},
		nextUser: &User{
			ID:       101,
			Email:    "ext@example.local",
			Username: "张三",
		},
	}
	apiKeys := &externalUserAPIKeyStub{
		nextKeys: []APIKey{
			{ID: 201, Key: "sk-created-1", Name: "张三", Status: StatusAPIKeyActive},
			{ID: 202, Key: "sk-created-2", Name: "张三", Status: StatusAPIKeyActive},
		},
	}
	mappings := newExternalUserMappingStub()
	svc := &ExternalUserService{
		adminService:  admin,
		apiKeyService: apiKeys,
		mappingRepo:   mappings,
	}

	result, err := svc.Create(context.Background(), ExternalUserInput{
		ExternalUserID:         " u-1 ",
		ExternalOrganizationID: " org-1 ",
		Username:               " 张三 ",
	})

	require.NoError(t, err)
	require.Equal(t, ExternalUserStatusCreated, result.Status)
	require.Equal(t, "u-1", result.ExternalUserID)
	require.NotNil(t, result.APIKey)
	require.Equal(t, int64(201), result.APIKey.ID)
	require.Equal(t, "sk-created-1", result.APIKey.Key)
	require.Len(t, result.APIKeys, 2)
	require.Equal(t, int64(7), *result.APIKeys[0].GroupID)
	require.Equal(t, int64(9), *result.APIKeys[1].GroupID)
	require.Equal(t, "公共一", result.APIKeys[0].Group.Name)

	require.Len(t, admin.createInputs, 1)
	input := admin.createInputs[0]
	require.Equal(t, "张三", input.Username)
	require.NotNil(t, input.Balance)
	require.Equal(t, float64(externalUserDefaultBalance), *input.Balance)
	require.Equal(t, externalUserDefaultConcurrency, input.Concurrency)
	require.Equal(t, []int64{7, 9}, input.AllowedGroups)
	require.Equal(t, "u-1@sub.com", input.Email)
	require.Equal(t, "u-1", input.Password)

	require.Len(t, apiKeys.createCalls, 2)
	require.Equal(t, int64(101), apiKeys.createCalls[0].userID)
	require.Equal(t, "张三", apiKeys.createCalls[0].req.Name)
	require.NotNil(t, apiKeys.createCalls[0].req.GroupID)
	require.Equal(t, int64(7), *apiKeys.createCalls[0].req.GroupID)
	require.NotNil(t, apiKeys.createCalls[1].req.GroupID)
	require.Equal(t, int64(9), *apiKeys.createCalls[1].req.GroupID)

	mapping, err := mappings.GetByExternalUserID(context.Background(), "u-1")
	require.NoError(t, err)
	require.Equal(t, int64(101), mapping.UserID)
	require.Equal(t, int64(201), mapping.APIKeyID)
	require.Equal(t, "org-1", mapping.ExternalOrganizationID)
	require.Equal(t, "张三", mapping.UsernameSnapshot)
}

func TestExternalUserService_Create_ExistingReturnsSkippedWithAPIKey(t *testing.T) {
	groupID := int64(7)
	admin := &externalUserAdminStub{
		groups: []Group{{ID: 7, Name: "公共", Status: StatusActive}},
		users: map[int64]*User{
			101: {ID: 101, Email: "ext@example.local", Username: "李四"},
		},
	}
	apiKeys := &externalUserAPIKeyStub{
		keys: map[int64]*APIKey{
			201: {ID: 201, Key: "sk-existing", Name: "李四", GroupID: &groupID, Status: StatusAPIKeyActive},
		},
	}
	mappings := newExternalUserMappingStub()
	require.NoError(t, mappings.Create(context.Background(), &ExternalUserMapping{
		ID:                     1,
		ExternalUserID:         "u-2",
		ExternalOrganizationID: "org-1",
		UserID:                 101,
		APIKeyID:               201,
	}))
	svc := &ExternalUserService{
		adminService:  admin,
		apiKeyService: apiKeys,
		mappingRepo:   mappings,
	}

	result, err := svc.Create(context.Background(), ExternalUserInput{
		ExternalUserID:         "u-2",
		ExternalOrganizationID: "org-1",
		Username:               "李四",
	})

	require.NoError(t, err)
	require.Equal(t, ExternalUserStatusSkipped, result.Status)
	require.Equal(t, int64(101), result.User.ID)
	require.Equal(t, int64(201), result.APIKey.ID)
	require.Equal(t, "sk-existing", result.APIKey.Key)
	require.Len(t, result.APIKeys, 1)
	require.Equal(t, "org-1", result.ExternalOrganizationID)
	require.Empty(t, admin.createInputs)
	require.Empty(t, apiKeys.createCalls)
}

func TestExternalUserService_Create_ExistingMissingAPIKeyCreatesReplacement(t *testing.T) {
	admin := &externalUserAdminStub{
		groups: []Group{{ID: 7, Status: StatusActive}},
		users: map[int64]*User{
			101: {ID: 101, Email: "ext@example.local", Username: "王五"},
		},
	}
	apiKeys := &externalUserAPIKeyStub{
		nextKeys: []APIKey{{ID: 202, Key: "sk-replacement", Name: "王五", Status: StatusAPIKeyActive}},
	}
	mappings := newExternalUserMappingStub()
	require.NoError(t, mappings.Create(context.Background(), &ExternalUserMapping{
		ID:                     1,
		ExternalUserID:         "u-3",
		ExternalOrganizationID: "org-1",
		UserID:                 101,
		APIKeyID:               201,
	}))
	svc := &ExternalUserService{
		adminService:  admin,
		apiKeyService: apiKeys,
		mappingRepo:   mappings,
	}

	result, err := svc.Create(context.Background(), ExternalUserInput{
		ExternalUserID:         "u-3",
		ExternalOrganizationID: "org-1",
		Username:               "王五",
	})

	require.NoError(t, err)
	require.Equal(t, ExternalUserStatusSkipped, result.Status)
	require.Equal(t, int64(202), result.APIKey.ID)
	require.Len(t, result.APIKeys, 1)
	mapping, err := mappings.GetByExternalUserID(context.Background(), "u-3")
	require.NoError(t, err)
	require.Equal(t, int64(202), mapping.APIKeyID)
}

func TestExternalUserService_Sync_ReturnsSummary(t *testing.T) {
	admin := &externalUserAdminStub{
		groups: []Group{{ID: 7, Status: StatusActive}},
		users: map[int64]*User{
			101: {ID: 101, Email: "old@example.local", Username: "已有"},
		},
		nextUser: &User{ID: 102, Email: "new@example.local", Username: "新增"},
	}
	apiKeys := &externalUserAPIKeyStub{
		keys: map[int64]*APIKey{
			201: {ID: 201, Key: "sk-existing", Name: "已有", Status: StatusAPIKeyActive},
		},
		nextKeys: []APIKey{{ID: 202, Key: "sk-new", Name: "新增", Status: StatusAPIKeyActive}},
	}
	mappings := newExternalUserMappingStub()
	require.NoError(t, mappings.Create(context.Background(), &ExternalUserMapping{
		ID:                     1,
		ExternalUserID:         "existing",
		ExternalOrganizationID: "org-1",
		UserID:                 101,
		APIKeyID:               201,
	}))
	svc := &ExternalUserService{
		adminService:  admin,
		apiKeyService: apiKeys,
		mappingRepo:   mappings,
	}

	result, err := svc.Sync(context.Background(), ExternalUserSyncInput{
		BatchID: "batch-1",
		Users: []ExternalUserInput{
			{ExternalUserID: "existing", ExternalOrganizationID: "org-1", Username: "已有"},
			{ExternalUserID: "new", ExternalOrganizationID: "org-1", Username: "新增"},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "batch-1", result.BatchID)
	require.Equal(t, 2, result.Summary.Total)
	require.Equal(t, 1, result.Summary.Created)
	require.Equal(t, 1, result.Summary.Skipped)
	require.Equal(t, 0, result.Summary.Failed)
	require.Len(t, result.Items, 2)
	require.Equal(t, ExternalUserStatusSkipped, result.Items[0].Status)
	require.Equal(t, ExternalUserStatusCreated, result.Items[1].Status)
	require.NotNil(t, result.Items[0].APIKey)
	require.NotNil(t, result.Items[1].APIKey)
	require.Len(t, result.Items[0].APIKeys, 1)
	require.Len(t, result.Items[1].APIKeys, 1)
}

func TestExternalUserService_DeleteAll_DeletesMappedUsers(t *testing.T) {
	admin := &externalUserAdminStub{
		users: map[int64]*User{
			101: {ID: 101, Email: "u-1@sub.com", Username: "张三"},
			102: {ID: 102, Email: "u-2@sub.com", Username: "李四"},
		},
	}
	apiKeys := &externalUserAPIKeyStub{}
	mappings := newExternalUserMappingStub()
	require.NoError(t, mappings.Create(context.Background(), &ExternalUserMapping{
		ID:                     1,
		ExternalUserID:         "u-1",
		ExternalOrganizationID: "org-1",
		UserID:                 101,
		APIKeyID:               201,
	}))
	require.NoError(t, mappings.Create(context.Background(), &ExternalUserMapping{
		ID:                     2,
		ExternalUserID:         "u-2",
		ExternalOrganizationID: "org-1",
		UserID:                 102,
		APIKeyID:               202,
	}))
	svc := &ExternalUserService{
		adminService:  admin,
		apiKeyService: apiKeys,
		mappingRepo:   mappings,
	}

	result, err := svc.DeleteAll(context.Background())

	require.NoError(t, err)
	require.Equal(t, 2, result.Summary.Total)
	require.Equal(t, 2, result.Summary.Deleted)
	require.Equal(t, 0, result.Summary.Failed)
	require.ElementsMatch(t, []int64{101, 102}, admin.deletedIDs)
	require.Empty(t, mappings.byExternal)
}

func TestExternalUserService_Create_MappingConflictRequeryFailureReturnsConflict(t *testing.T) {
	admin := &externalUserAdminStub{
		groups:   []Group{{ID: 7, Status: StatusActive}},
		nextUser: &User{ID: 101, Email: "ext@example.local", Username: "赵六"},
	}
	apiKeys := &externalUserAPIKeyStub{
		nextKeys: []APIKey{{ID: 201, Key: "sk-created", Name: "赵六", Status: StatusAPIKeyActive}},
	}
	mappings := newExternalUserMappingStub()
	mappings.createErr = ErrExternalUserMappingExists
	svc := &ExternalUserService{
		adminService:  admin,
		apiKeyService: apiKeys,
		mappingRepo:   mappings,
	}

	_, err := svc.Create(context.Background(), ExternalUserInput{
		ExternalUserID:         "u-conflict",
		ExternalOrganizationID: "org-1",
		Username:               "赵六",
	})

	require.ErrorIs(t, err, ErrExternalUserMappingExists)
	require.Equal(t, http.StatusConflict, infraerrors.Code(err))
	require.Equal(t, []int64{101}, admin.deletedIDs)
}

type externalUserAdminStub struct {
	groups       []Group
	users        map[int64]*User
	nextUser     *User
	createErr    error
	deleteErr    error
	createInputs []*CreateUserInput
	deletedIDs   []int64
}

func (s *externalUserAdminStub) CreateUser(_ context.Context, input *CreateUserInput) (*User, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	s.createInputs = append(s.createInputs, input)
	if s.nextUser == nil {
		return nil, errors.New("next user missing")
	}
	user := *s.nextUser
	if input != nil {
		user.Email = input.Email
		user.Username = input.Username
		user.Balance = *input.Balance
		user.Concurrency = input.Concurrency
		user.AllowedGroups = append([]int64(nil), input.AllowedGroups...)
	}
	if s.users == nil {
		s.users = make(map[int64]*User)
	}
	s.users[user.ID] = &user
	return &user, nil
}

func (s *externalUserAdminStub) DeleteUser(_ context.Context, id int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deletedIDs = append(s.deletedIDs, id)
	return nil
}

func (s *externalUserAdminStub) GetAllGroups(_ context.Context) ([]Group, error) {
	return append([]Group(nil), s.groups...), nil
}

func (s *externalUserAdminStub) GetUser(_ context.Context, id int64) (*User, error) {
	if user := s.users[id]; user != nil {
		out := *user
		return &out, nil
	}
	return nil, ErrUserNotFound
}

type externalUserAPIKeyCreateCall struct {
	userID int64
	req    CreateAPIKeyRequest
}

type externalUserAPIKeyStub struct {
	keys        map[int64]*APIKey
	nextKey     *APIKey
	nextKeys    []APIKey
	getErr      error
	createErr   error
	createCalls []externalUserAPIKeyCreateCall
}

func (s *externalUserAPIKeyStub) Create(_ context.Context, userID int64, req CreateAPIKeyRequest) (*APIKey, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	s.createCalls = append(s.createCalls, externalUserAPIKeyCreateCall{userID: userID, req: req})
	var key APIKey
	if len(s.nextKeys) > 0 {
		key = s.nextKeys[0]
		s.nextKeys = s.nextKeys[1:]
	} else if s.nextKey != nil {
		key = *s.nextKey
	} else {
		return nil, errors.New("next api key missing")
	}
	key.UserID = userID
	key.Name = req.Name
	key.GroupID = req.GroupID
	if s.keys == nil {
		s.keys = make(map[int64]*APIKey)
	}
	s.keys[key.ID] = &key
	return &key, nil
}

func (s *externalUserAPIKeyStub) GetByID(_ context.Context, id int64) (*APIKey, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if key := s.keys[id]; key != nil {
		out := *key
		return &out, nil
	}
	return nil, ErrAPIKeyNotFound
}

func (s *externalUserAPIKeyStub) List(_ context.Context, userID int64, _ pagination.PaginationParams, _ APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	out := make([]APIKey, 0, len(s.keys))
	for _, key := range s.keys {
		if key != nil && key.UserID == userID {
			out = append(out, *key)
		}
	}
	return out, &pagination.PaginationResult{Total: int64(len(out))}, nil
}

type externalUserMappingStub struct {
	byExternal map[string]*ExternalUserMapping
	nextID     int64
	createErr  error
}

func newExternalUserMappingStub() *externalUserMappingStub {
	return &externalUserMappingStub{
		byExternal: make(map[string]*ExternalUserMapping),
		nextID:     1,
	}
}

func (s *externalUserMappingStub) GetByExternalUserID(_ context.Context, externalUserID string) (*ExternalUserMapping, error) {
	if mapping := s.byExternal[externalUserID]; mapping != nil {
		out := *mapping
		return &out, nil
	}
	return nil, ErrExternalUserMappingNotFound
}

func (s *externalUserMappingStub) ListActive(_ context.Context) ([]ExternalUserMapping, error) {
	out := make([]ExternalUserMapping, 0, len(s.byExternal))
	for _, mapping := range s.byExternal {
		if mapping == nil {
			continue
		}
		item := *mapping
		out = append(out, item)
	}
	return out, nil
}

func (s *externalUserMappingStub) Create(_ context.Context, mapping *ExternalUserMapping) error {
	if s.createErr != nil {
		return s.createErr
	}
	if _, ok := s.byExternal[mapping.ExternalUserID]; ok {
		return ErrExternalUserMappingExists
	}
	out := *mapping
	if out.ID == 0 {
		out.ID = s.nextID
		s.nextID++
	}
	s.byExternal[out.ExternalUserID] = &out
	mapping.ID = out.ID
	return nil
}

func (s *externalUserMappingStub) UpdateAPIKeyID(_ context.Context, id int64, apiKeyID int64) error {
	for _, mapping := range s.byExternal {
		if mapping.ID == id {
			mapping.APIKeyID = apiKeyID
			return nil
		}
	}
	return ErrExternalUserMappingNotFound
}

func (s *externalUserMappingStub) SoftDeleteByExternalUserID(_ context.Context, externalUserID string) error {
	if _, ok := s.byExternal[externalUserID]; !ok {
		return ErrExternalUserMappingNotFound
	}
	delete(s.byExternal, externalUserID)
	return nil
}
