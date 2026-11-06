package repository

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAnalyticsIdentitiesOnlyBindUniqueNamesAndStableIDs(t *testing.T) {
	db, mock := newSQLMock(t)
	mock.ExpectQuery(`SELECT u.id`).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "person", "basis"}).
		AddRow(1, " 张\u200b三 ", "", "").AddRow(2, "李四", "", "").AddRow(3, "赵五", "stable", "manual").
		AddRow(4, "王六", "", "").AddRow(5, "王六", "", "").AddRow(6, "稳定冲突", "same", "external_id").AddRow(7, "另一账号", "same", "external_id"))
	dir := service.AnalyticsDirectory{People: []service.AnalyticsPerson{
		{ID: "p1", Name: "张三"}, {ID: "p2", Name: "李四"}, {ID: "p3", Name: "李四"}, {ID: "stable", Name: "已改姓名"},
		{ID: "p4", Name: "王六"}, {ID: "same", Name: "稳定冲突"},
	}}
	people, err := analyticsIdentities(context.Background(), db, dir)
	require.NoError(t, err)
	require.Equal(t, "p1", people[0].PersonID)
	require.Empty(t, people[1].PersonID)
	require.Equal(t, "stable", people[2].PersonID)
	require.Empty(t, people[3].PersonID)
	require.Empty(t, people[4].PersonID)
	require.Equal(t, "conflict", people[5].Basis)
	require.Empty(t, people[5].PersonID)
	require.Empty(t, people[6].PersonID)
	require.NoError(t, mock.ExpectationsWereMet())
}
