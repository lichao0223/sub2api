//go:build integration

package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/externalusermapping"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestExternalUserMappingRepository_CreateGetUpdateAndSoftDelete(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := NewExternalUserMappingRepository(client)

	user := createExternalMappingTestUser(t, ctx, client, uniqueTestValue(t, "ext-map-user")+"@example.com")
	key := createExternalMappingTestAPIKey(t, ctx, client, user.ID, uniqueTestValue(t, "sk-ext-map"))

	mapping := &service.ExternalUserMapping{
		ExternalUserID:         uniqueTestValue(t, "external-user"),
		ExternalOrganizationID: uniqueTestValue(t, "external-org"),
		UserID:                 user.ID,
		APIKeyID:               key.ID,
		UsernameSnapshot:       "张三",
	}
	require.NoError(t, repo.Create(ctx, mapping))
	require.NotZero(t, mapping.ID)

	got, err := repo.GetByExternalUserID(ctx, mapping.ExternalUserID)
	require.NoError(t, err)
	require.Equal(t, user.ID, got.UserID)
	require.Equal(t, key.ID, got.APIKeyID)
	require.Equal(t, mapping.ExternalOrganizationID, got.ExternalOrganizationID)
	require.Equal(t, "张三", got.UsernameSnapshot)

	replacementKey := createExternalMappingTestAPIKey(t, ctx, client, user.ID, uniqueTestValue(t, "sk-ext-map-replacement"))
	require.NoError(t, repo.UpdateAPIKeyID(ctx, mapping.ID, replacementKey.ID))
	got, err = repo.GetByExternalUserID(ctx, mapping.ExternalUserID)
	require.NoError(t, err)
	require.Equal(t, replacementKey.ID, got.APIKeyID)

	require.NoError(t, repo.SoftDeleteByExternalUserID(ctx, mapping.ExternalUserID))
	_, err = repo.GetByExternalUserID(ctx, mapping.ExternalUserID)
	require.ErrorIs(t, err, service.ErrExternalUserMappingNotFound)

	deleted, err := client.ExternalUserMapping.Query().
		Where(externalusermapping.IDEQ(mapping.ID)).
		Only(mixins.SkipSoftDelete(ctx))
	require.NoError(t, err)
	require.NotNil(t, deleted.DeletedAt)
}

func TestExternalUserMappingRepository_DuplicateActiveExternalUserIDReturnsConflict(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewExternalUserMappingRepository(client)

	user := createExternalMappingTestUser(t, ctx, client, uniqueTestValue(t, "ext-map-conflict-user")+"@example.com")
	key1 := createExternalMappingTestAPIKey(t, ctx, client, user.ID, uniqueTestValue(t, "sk-ext-map-conflict-1"))
	key2 := createExternalMappingTestAPIKey(t, ctx, client, user.ID, uniqueTestValue(t, "sk-ext-map-conflict-2"))
	externalUserID := uniqueTestValue(t, "external-user-conflict")

	require.NoError(t, repo.Create(ctx, &service.ExternalUserMapping{
		ExternalUserID:         externalUserID,
		ExternalOrganizationID: "org-1",
		UserID:                 user.ID,
		APIKeyID:               key1.ID,
	}))
	err := repo.Create(ctx, &service.ExternalUserMapping{
		ExternalUserID:         externalUserID,
		ExternalOrganizationID: "org-1",
		UserID:                 user.ID,
		APIKeyID:               key2.ID,
	})
	require.ErrorIs(t, err, service.ErrExternalUserMappingExists)
}

func TestExternalUserMappingRepository_AllowsReuseAfterSoftDelete(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := NewExternalUserMappingRepository(client)

	user := createExternalMappingTestUser(t, ctx, client, uniqueTestValue(t, "ext-map-reuse-user")+"@example.com")
	key1 := createExternalMappingTestAPIKey(t, ctx, client, user.ID, uniqueTestValue(t, "sk-ext-map-reuse-1"))
	key2 := createExternalMappingTestAPIKey(t, ctx, client, user.ID, uniqueTestValue(t, "sk-ext-map-reuse-2"))
	externalUserID := uniqueTestValue(t, "external-user-reuse")

	require.NoError(t, repo.Create(ctx, &service.ExternalUserMapping{
		ExternalUserID:         externalUserID,
		ExternalOrganizationID: "org-1",
		UserID:                 user.ID,
		APIKeyID:               key1.ID,
	}))
	require.NoError(t, repo.SoftDeleteByExternalUserID(ctx, externalUserID))

	recreated := &service.ExternalUserMapping{
		ExternalUserID:         externalUserID,
		ExternalOrganizationID: "org-1",
		UserID:                 user.ID,
		APIKeyID:               key2.ID,
	}
	require.NoError(t, repo.Create(ctx, recreated))
	require.NotZero(t, recreated.ID)
	require.NotEqual(t, key1.ID, recreated.APIKeyID)
}

func createExternalMappingTestUser(t *testing.T, ctx context.Context, client *dbent.Client, email string) *dbent.User {
	t.Helper()
	user, err := client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-password-hash").
		SetUsername("test-user").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		SetBalance(0).
		SetConcurrency(1).
		Save(ctx)
	require.NoError(t, err)
	return user
}

func createExternalMappingTestAPIKey(t *testing.T, ctx context.Context, client *dbent.Client, userID int64, key string) *dbent.APIKey {
	t.Helper()
	apiKey, err := client.APIKey.Create().
		SetUserID(userID).
		SetKey(key).
		SetName("test-key").
		SetStatus(service.StatusAPIKeyActive).
		Save(ctx)
	require.NoError(t, err)
	return apiKey
}
