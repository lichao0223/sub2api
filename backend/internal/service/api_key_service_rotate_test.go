package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type rotateAPIKeyRepoStub struct {
	APIKeyRepository
	oldKeys []string
	newKeys []string
}

func (s *rotateAPIKeyRepoStub) ListKeysByUserID(context.Context, int64) ([]string, error) {
	return append([]string(nil), s.oldKeys...), nil
}

func (s *rotateAPIKeyRepoStub) RotateByUserID(_ context.Context, userID int64, keys []string) ([]APIKey, error) {
	s.newKeys = append([]string(nil), keys...)
	out := make([]APIKey, len(keys))
	for i, key := range keys {
		out[i] = APIKey{ID: int64(i + 1), UserID: userID, Key: key}
	}
	return out, nil
}

type rotateAPIKeyUserRepoStub struct{ UserRepository }

func (rotateAPIKeyUserRepoStub) GetByID(context.Context, int64) (*User, error) {
	return &User{ID: 7}, nil
}

func TestAPIKeyService_RotateUserKeysReplacesEveryKey(t *testing.T) {
	repo := &rotateAPIKeyRepoStub{oldKeys: []string{"sk-old-1", "sk-old-2"}}
	svc := NewAPIKeyService(repo, rotateAPIKeyUserRepoStub{}, nil, nil, nil, nil, &config.Config{})

	keys, err := svc.RotateUserKeys(context.Background(), 7)

	require.NoError(t, err)
	require.Len(t, keys, 2)
	require.Len(t, repo.newKeys, 2)
	require.NotEqual(t, repo.newKeys[0], repo.newKeys[1])
	require.NotContains(t, repo.oldKeys, repo.newKeys[0])
}
