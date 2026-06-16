//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestExternalUserService_Create_CreatesUserWithDefaultsAndAPIKey(t *testing.T) {
	admin := &externalUserAdminStub{
		groups: []Group{{ID: 7, Status: StatusActive}},
		nextUser: &User{
			ID:       101,
			Email:    "ext@example.local",
			Username: "张三",
		},
	}
	apiKeys := &externalUserAPIKeyStub{
		nextKey: &APIKey{
			ID:     201,
			Key:    "sk-created",
			Name:   "张三",
			Status: StatusAPIKeyActive,
		},
	}
	mappings := newExternalUserMappingStub()
	svc := &ExternalUserService{
		adminService:  admin,
		apiKeyService: apiKeys,
		mappingRepo:   mappings,
	}

	result, err := svc.Create(context.Background(), ExternalUserInput{
		ExternalUserID: " u-1 ",
		Username:       " 张三 ",
	})

	require.NoError(t, err)
	require.Equal(t, ExternalUserStatusCreated, result.Status)
	require.Equal(t, "u-1", result.ExternalUserID)
	require.NotNil(t, result.APIKey)
	require.Equal(t, int64(201), result.APIKey.ID)
	require.Equal(t, "sk-created", result.APIKey.Key)

	require.Len(t, admin.createInputs, 1)
	input := admin.createInputs[0]
	require.Equal(t, "张三", input.Username)
	require.NotNil(t, input.Balance)
	require.Equal(t, float64(externalUserDefaultBalance), *input.Balance)
	require.Equal(t, externalUserDefaultConcurrency, input.Concurrency)
	require.Equal(t, []int64{7}, input.AllowedGroups)
	require.Equal(t, generatedExternalEmail("u-1"), input.Email)

	require.Len(t, apiKeys.createCalls, 1)
	require.Equal(t, int64(101), apiKeys.createCalls[0].userID)
	require.Equal(t, "张三", apiKeys.createCalls[0].req.Name)
	require.NotNil(t, apiKeys.createCalls[0].req.GroupID)
	require.Equal(t, int64(7), *apiKeys.createCalls[0].req.GroupID)

	mapping, err := mappings.GetByExternalUserID(context.Background(), "u-1")
	require.NoError(t, err)
	require.Equal(t, int64(101), mapping.UserID)
	require.Equal(t, int64(201), mapping.APIKeyID)
	require.Equal(t, "张三", mapping.UsernameSnapshot)
}

func TestExternalUserService_Create_ExistingReturnsSkippedWithAPIKey(t *testing.T) {
	groupID := int64(7)
	admin := &externalUserAdminStub{
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
		ID:             1,
		ExternalUserID: "u-2",
		UserID:         101,
		APIKeyID:       201,
	}))
	svc := &ExternalUserService{
		adminService:  admin,
		apiKeyService: apiKeys,
		mappingRepo:   mappings,
	}

	result, err := svc.Create(context.Background(), ExternalUserInput{
		ExternalUserID: "u-2",
		Username:       "李四",
	})

	require.NoError(t, err)
	require.Equal(t, ExternalUserStatusSkipped, result.Status)
	require.Equal(t, int64(101), result.User.ID)
	require.Equal(t, int64(201), result.APIKey.ID)
	require.Equal(t, "sk-existing", result.APIKey.Key)
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
		getErr:  ErrAPIKeyNotFound,
		nextKey: &APIKey{ID: 202, Key: "sk-replacement", Name: "王五", Status: StatusAPIKeyActive},
	}
	mappings := newExternalUserMappingStub()
	require.NoError(t, mappings.Create(context.Background(), &ExternalUserMapping{
		ID:             1,
		ExternalUserID: "u-3",
		UserID:         101,
		APIKeyID:       201,
	}))
	svc := &ExternalUserService{
		adminService:  admin,
		apiKeyService: apiKeys,
		mappingRepo:   mappings,
	}

	result, err := svc.Create(context.Background(), ExternalUserInput{
		ExternalUserID: "u-3",
		Username:       "王五",
	})

	require.NoError(t, err)
	require.Equal(t, ExternalUserStatusSkipped, result.Status)
	require.Equal(t, int64(202), result.APIKey.ID)
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
		nextKey: &APIKey{ID: 202, Key: "sk-new", Name: "新增", Status: StatusAPIKeyActive},
	}
	mappings := newExternalUserMappingStub()
	require.NoError(t, mappings.Create(context.Background(), &ExternalUserMapping{
		ID:             1,
		ExternalUserID: "existing",
		UserID:         101,
		APIKeyID:       201,
	}))
	svc := &ExternalUserService{
		adminService:  admin,
		apiKeyService: apiKeys,
		mappingRepo:   mappings,
	}

	result, err := svc.Sync(context.Background(), ExternalUserSyncInput{
		BatchID: "batch-1",
		Users: []ExternalUserInput{
			{ExternalUserID: "existing", Username: "已有"},
			{ExternalUserID: "new", Username: "新增"},
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
}

func TestExternalUserService_Create_MappingConflictRequeryFailureReturnsConflict(t *testing.T) {
	admin := &externalUserAdminStub{
		groups:   []Group{{ID: 7, Status: StatusActive}},
		nextUser: &User{ID: 101, Email: "ext@example.local", Username: "赵六"},
	}
	apiKeys := &externalUserAPIKeyStub{
		nextKey: &APIKey{ID: 201, Key: "sk-created", Name: "赵六", Status: StatusAPIKeyActive},
	}
	mappings := newExternalUserMappingStub()
	mappings.createErr = ErrExternalUserMappingExists
	svc := &ExternalUserService{
		adminService:  admin,
		apiKeyService: apiKeys,
		mappingRepo:   mappings,
	}

	_, err := svc.Create(context.Background(), ExternalUserInput{
		ExternalUserID: "u-conflict",
		Username:       "赵六",
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
	getErr      error
	createErr   error
	createCalls []externalUserAPIKeyCreateCall
}

func (s *externalUserAPIKeyStub) Create(_ context.Context, userID int64, req CreateAPIKeyRequest) (*APIKey, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	s.createCalls = append(s.createCalls, externalUserAPIKeyCreateCall{userID: userID, req: req})
	if s.nextKey == nil {
		return nil, errors.New("next api key missing")
	}
	key := *s.nextKey
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
