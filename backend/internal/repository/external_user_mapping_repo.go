package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/externalusermapping"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type externalUserMappingRepository struct {
	client *dbent.Client
}

func NewExternalUserMappingRepository(client *dbent.Client) service.ExternalUserMappingRepository {
	return &externalUserMappingRepository{client: client}
}

func (r *externalUserMappingRepository) GetByExternalUserID(ctx context.Context, externalUserID string) (*service.ExternalUserMapping, error) {
	m, err := clientFromContext(ctx, r.client).ExternalUserMapping.Query().
		Where(
			externalusermapping.ExternalUserIDEQ(externalUserID),
			externalusermapping.DeletedAtIsNil(),
		).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrExternalUserMappingNotFound, nil)
	}
	return externalUserMappingEntityToService(m), nil
}

func (r *externalUserMappingRepository) ListActive(ctx context.Context) ([]service.ExternalUserMapping, error) {
	items, err := clientFromContext(ctx, r.client).ExternalUserMapping.Query().
		Where(externalusermapping.DeletedAtIsNil()).
		Order(dbent.Asc(externalusermapping.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]service.ExternalUserMapping, 0, len(items))
	for _, item := range items {
		if mapping := externalUserMappingEntityToService(item); mapping != nil {
			out = append(out, *mapping)
		}
	}
	return out, nil
}

func (r *externalUserMappingRepository) Create(ctx context.Context, mapping *service.ExternalUserMapping) error {
	if mapping == nil {
		return nil
	}
	m, err := clientFromContext(ctx, r.client).ExternalUserMapping.Create().
		SetExternalUserID(mapping.ExternalUserID).
		SetExternalOrganizationID(mapping.ExternalOrganizationID).
		SetUserID(mapping.UserID).
		SetAPIKeyID(mapping.APIKeyID).
		SetUsernameSnapshot(mapping.UsernameSnapshot).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrExternalUserMappingExists)
	}
	applyExternalUserMappingEntity(mapping, m)
	return nil
}

func (r *externalUserMappingRepository) UpdateAPIKeyID(ctx context.Context, id int64, apiKeyID int64) error {
	updated, err := clientFromContext(ctx, r.client).ExternalUserMapping.Update().
		Where(
			externalusermapping.IDEQ(id),
			externalusermapping.DeletedAtIsNil(),
		).
		SetAPIKeyID(apiKeyID).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return err
	}
	if updated == 0 {
		return service.ErrExternalUserMappingNotFound
	}
	return nil
}

func (r *externalUserMappingRepository) SoftDeleteByExternalUserID(ctx context.Context, externalUserID string) error {
	updated, err := clientFromContext(ctx, r.client).ExternalUserMapping.Update().
		Where(
			externalusermapping.ExternalUserIDEQ(externalUserID),
			externalusermapping.DeletedAtIsNil(),
		).
		SetDeletedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return err
	}
	if updated == 0 {
		return service.ErrExternalUserMappingNotFound
	}
	return nil
}

func externalUserMappingEntityToService(m *dbent.ExternalUserMapping) *service.ExternalUserMapping {
	if m == nil {
		return nil
	}
	return &service.ExternalUserMapping{
		ID:                     m.ID,
		ExternalUserID:         m.ExternalUserID,
		ExternalOrganizationID: m.ExternalOrganizationID,
		UserID:                 m.UserID,
		APIKeyID:               m.APIKeyID,
		UsernameSnapshot:       m.UsernameSnapshot,
		CreatedAt:              m.CreatedAt,
		UpdatedAt:              m.UpdatedAt,
		DeletedAt:              m.DeletedAt,
	}
}

func applyExternalUserMappingEntity(out *service.ExternalUserMapping, m *dbent.ExternalUserMapping) {
	if out == nil || m == nil {
		return
	}
	out.ID = m.ID
	out.ExternalUserID = m.ExternalUserID
	out.ExternalOrganizationID = m.ExternalOrganizationID
	out.UserID = m.UserID
	out.APIKeyID = m.APIKeyID
	out.UsernameSnapshot = m.UsernameSnapshot
	out.CreatedAt = m.CreatedAt
	out.UpdatedAt = m.UpdatedAt
	out.DeletedAt = m.DeletedAt
}
