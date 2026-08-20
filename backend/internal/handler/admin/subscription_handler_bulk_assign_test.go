package admin

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type bulkAssignUserListerStub struct {
	pages   [][]service.User
	filters []service.UserListFilters
}

func (s *bulkAssignUserListerStub) ListUsers(_ context.Context, page, _ int, filters service.UserListFilters, _, _ string) ([]service.User, int64, error) {
	s.filters = append(s.filters, filters)
	if page > len(s.pages) {
		return nil, int64(len(s.pages)), nil
	}
	return s.pages[page-1], int64(len(s.pages)), nil
}

func TestListAllActiveUserIDsExcludesDisabledAndDeletedUsers(t *testing.T) {
	deletedAt := time.Now()
	lister := &bulkAssignUserListerStub{pages: [][]service.User{
		{{ID: 1, Status: service.StatusActive}},
		{{ID: 2, Status: service.StatusDisabled}, {ID: 3, Status: service.StatusActive, DeletedAt: &deletedAt}},
	}}

	ids, err := listAllActiveUserIDs(context.Background(), lister, 1)

	require.NoError(t, err)
	require.Equal(t, []int64{1}, ids)
	require.Len(t, lister.filters, 2)
	for _, filters := range lister.filters {
		require.Equal(t, service.StatusActive, filters.Status)
		require.NotNil(t, filters.IncludeSubscriptions)
		require.False(t, *filters.IncludeSubscriptions)
	}
}
