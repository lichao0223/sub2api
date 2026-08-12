package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type batchUpdateAPIKeyRepoStub struct {
	APIKeyRepository
	groupID int64
	ids     []int64
	all     bool
	fields  APIKeyBatchUpdateFields
}

func (s *batchUpdateAPIKeyRepoStub) BatchUpdateByGroup(_ context.Context, groupID int64, ids []int64, all bool, fields APIKeyBatchUpdateFields, _ func() (string, error)) (int, []string, error) {
	s.groupID, s.ids, s.all, s.fields = groupID, ids, all, fields
	return 2, nil, nil
}

func TestAdminBatchUpdateMovesWholeGroup(t *testing.T) {
	repo := &batchUpdateAPIKeyRepoStub{}
	svc := &APIKeyService{apiKeyRepo: repo}
	targetGroupID := int64(0)
	affected, err := svc.AdminBatchUpdate(context.Background(), AdminBatchUpdateAPIKeysRequest{
		GroupID: 7,
		All:     true,
		Fields: APIKeyBatchUpdateFields{
			GroupID:               &targetGroupID,
			RecreateInSourceGroup: true,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 2, affected)
	require.True(t, repo.all)
	require.Equal(t, int64(0), *repo.fields.GroupID)
	require.True(t, repo.fields.RecreateInSourceGroup)
}

func TestAdminBatchUpdateScopesAndDeduplicatesIDs(t *testing.T) {
	repo := &batchUpdateAPIKeyRepoStub{}
	svc := &APIKeyService{apiKeyRepo: repo}
	status := "inactive"
	affected, err := svc.AdminBatchUpdate(context.Background(), AdminBatchUpdateAPIKeysRequest{
		GroupID: 7,
		IDs:     []int64{3, 3, 4},
		Fields:  APIKeyBatchUpdateFields{Status: &status},
	})
	require.NoError(t, err)
	require.Equal(t, 2, affected)
	require.Equal(t, int64(7), repo.groupID)
	require.Equal(t, []int64{3, 4}, repo.ids)
	require.False(t, repo.all)
}
