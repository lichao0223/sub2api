package service

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsPeriodFollowsSelection(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	p, err := NewAnalyticsPeriod(2026, "year", nil, now)
	require.NoError(t, err)
	require.Len(t, p.Months, 11)
	require.Equal(t, "month", p.Grain)
	p, err = NewAnalyticsPeriod(2024, "month", []int{2}, now)
	require.NoError(t, err)
	require.Equal(t, "day", p.Grain)
	p, err = NewAnalyticsPeriod(2026, "months", []int{10, 1, 3, 3}, now)
	require.NoError(t, err)
	require.Equal(t, []int{1, 3, 10}, p.Months)
	_, err = NewAnalyticsPeriod(2026, "month", []int{12}, now)
	require.Error(t, err)
	_, err = NewAnalyticsPeriod(2026, "months", nil, now)
	require.Error(t, err)
}

func TestAnalyticsFixedAllocationBalancesExactly(t *testing.T) {
	for _, total := range []string{"100", "0.000000000001", "99999999.999999999999"} {
		split := AllocateAnalyticsCost(decimal.RequireFromString(total), []decimal.Decimal{decimal.NewFromInt(1), decimal.NewFromInt(1), decimal.NewFromInt(1)})
		sum := decimal.Zero
		for _, v := range split {
			sum = sum.Add(v)
		}
		require.True(t, sum.Equal(decimal.RequireFromString(total)))
		require.True(t, split[0].Sub(split[2]).LessThanOrEqual(decimal.New(1, -12)))
	}
}

func TestAnalyticsSQLRejectsExpressionsAndKeepsOnlyDirectoryFields(t *testing.T) {
	org := "INSERT INTO `devp_org` (`id`,`parent_id`,`org_name`) VALUES ('1','0','公司'),('2','1','研发'),('3','2','平台'),('4','3','四级');"
	person := "INSERT INTO devp_user (id,real_name,dev_org_id,password,phone) VALUES ('p','张三','4','secret','123');"
	d, _, err := ParseAnalyticsDirectory(org, person)
	require.NoError(t, err)
	require.Equal(t, []string{"1", "2", "3", "4"}, d.Orgs[3].Path)
	require.NotContains(t, AnalyticsJSON(d), "secret")
	require.NotContains(t, AnalyticsJSON(d), "phone")
	for _, invalid := range []string{
		"INSERT INTO devp_user(id,real_name,dev_org_id) VALUES('p',NOW(),'1');",
		"DELETE FROM devp_user;",
		"INSERT INTO devp_user(id,real_name,dev_org_id) VALUES('p','张三','1'),('p','李四','1');",
	} {
		_, _, err := ParseAnalyticsDirectory(org, invalid)
		require.Error(t, err)
	}
	_, _, err = ParseAnalyticsDirectory("INSERT INTO devp_org(id,parent_id,org_name) VALUES('1','2','a'),('2','1','b');", person)
	require.Error(t, err)
}

func TestAnalyticsSQLQuotedSemicolonsCommentsAndInlineSchema(t *testing.T) {
	org := "CREATE TABLE devp_org (`id` varchar(64), `parent_id` varchar(64), `org_name` varchar(64)); LOCK TABLES devp_org WRITE; INSERT INTO devp_org VALUES('1','0','公司;总部'),('2','1','研发/*保留*/'); UNLOCK TABLES;"
	person := "-- 导出\nINSERT INTO devp_user(id,real_name,dev_org_id) VALUES('p','张''三','2');"
	dir, _, err := ParseAnalyticsDirectory(org, person)
	require.NoError(t, err)
	require.Equal(t, "公司;总部", dir.Orgs[0].Name)
	require.Equal(t, "研发/*保留*/", dir.Orgs[1].Name)
	require.Equal(t, "张'三", dir.People[0].Name)
	require.Equal(t, "张三A", NormalizeAnalyticsName(" 张\u200b三Ａ "))
}

func TestAnalyticsSQLCreateColumnsIgnoreIndexesAndQuotedMetadata(t *testing.T) {
	for _, schema := range []string{
		"`id` varchar(64), `parent_id` varchar(64), `org_name` varchar(64), KEY `idx_parent` (`parent_id` ASC, `id` ASC)",
		"`id` varchar(64),\n`parent_id` varchar(64),\n`org_name` varchar(64),\nUNIQUE KEY `idx_org` (`parent_id` ASC,\n`org_name` DESC),\nPRIMARY KEY (`id`)",
		"`id` varchar(64), `parent_id` varchar(64), `org_name` varchar(64) COMMENT '名称, `id` 不是真实字段'",
		"`id` varchar(64), `parent_id` varchar(64), `org_name` enum('公司', '研发, `id` 引用')",
		"`id` varchar(64), `parent_id` varchar(64), `org_name` varchar(64) COMMENT '名称\\'含逗号, `id` 引用'",
		"`id` varchar(64), `parent_id` varchar(64), `org_name` varchar(64) COMMENT '名称''含逗号, `id` 引用'",
		"`id` varchar(64), `parent_id` varchar(64), `org_name` varchar(64) COMMENT \"名称, `id` 引用\"",
	} {
		t.Run(schema, func(t *testing.T) {
			org := "CREATE TABLE devp_org (" + schema + ") COMMENT='目录, `id` 仅作说明'; INSERT INTO devp_org VALUES('1','0','公司');"
			people := "CREATE TABLE devp_user (`id` varchar(64), `real_name` varchar(64), `dev_org_id` varchar(64), KEY `idx_org` (`dev_org_id` ASC, `id` ASC)); INSERT INTO devp_user VALUES('p','张三','1');"
			directory, _, err := ParseAnalyticsDirectory(org, people)
			require.NoError(t, err)
			require.Equal(t, []AnalyticsOrg{{ID: "1", Parent: "0", Name: "公司", Path: []string{"1"}, Names: []string{"公司"}, Level: 1}}, directory.Orgs)
			require.Equal(t, []AnalyticsPerson{{ID: "p", Name: "张三", Org: "1", Active: true}}, directory.People)
		})
	}
}

func TestAnalyticsSQLDuplicateColumnErrorsNameFileAndColumn(t *testing.T) {
	org := "INSERT INTO devp_org(id,parent_id,org_name) VALUES('1','0','公司');"
	people := "INSERT INTO devp_user(id,real_name,dev_org_id) VALUES('p','张三','1');"
	for _, tc := range []struct{ name, org, people, message string }{
		{"organization", "INSERT INTO devp_org(id,parent_id,org_name,id) VALUES('1','0','公司','1');", people, "devp_org.sql：SQL列名重复：id"},
		{"person", org, "INSERT INTO devp_user(id,real_name,dev_org_id,id) VALUES('p','张三','1','p');", "devp_user.sql：SQL列名重复：id"},
		{"schema", "CREATE TABLE devp_org (`id` varchar(64), `parent_id` varchar(64), `org_name` varchar(64), `id` varchar(64)); INSERT INTO devp_org VALUES('1','0','公司','1');", people, "devp_org.sql：SQL列名重复：id"},
		{"unclosed schema", "CREATE TABLE devp_org (`id` varchar(64); INSERT INTO devp_org VALUES('1','0','公司');", people, "devp_org.sql：CREATE TABLE括号未闭合"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ParseAnalyticsDirectory(tc.org, tc.people)
			require.EqualError(t, err, tc.message)
		})
	}
}

func TestAnalyticsAllocationSkipsZeroWeightAndUsesRequestsFallback(t *testing.T) {
	result := AllocateAnalyticsCost(decimal.RequireFromString("0.000000000001"), []decimal.Decimal{decimal.Zero, decimal.NewFromInt(1), decimal.NewFromInt(1)})
	require.True(t, result[0].IsZero())
	require.Equal(t, "0.000000000001", result[1].String())
	require.True(t, result[2].IsZero())
}

type analyticsFeatureRepo struct {
	DataAnalyticsRepository
	called     bool
	controlled bool
}

func (r *analyticsFeatureRepo) ApplyAnalyticsDirectory(ctx context.Context, _ string, _ time.Time, _, _ int64) error {
	r.called = true
	_, r.controlled = AnalyticsTaskVersion(ctx)
	return nil
}
func (r *analyticsFeatureRepo) AnalyticsReport(ctx context.Context, _ AnalyticsQuery) (*AnalyticsReport, error) {
	r.called = true
	_, r.controlled = AnalyticsTaskVersion(ctx)
	return &AnalyticsReport{}, nil
}
func TestAnalyticsFeatureWritesAndExportsUseTaskControl(t *testing.T) {
	control := newAnalyticsTaskMemoryRepo()
	tasks := NewAnalyticsTaskService(control, "feature-test")
	repo := &analyticsFeatureRepo{}
	s := NewDataAnalyticsService(repo, tasks)
	require.ErrorIs(t, s.Apply(context.Background(), "preview", time.Now(), 0, 1), ErrAnalyticsTasksPaused)
	_, err := s.Report(context.Background(), AnalyticsQuery{Export: true})
	require.ErrorIs(t, err, ErrAnalyticsTasksPaused)
	require.False(t, repo.called)
	_, err = s.Report(context.Background(), AnalyticsQuery{})
	require.NoError(t, err)
	require.False(t, repo.controlled)
	_, err = tasks.SetEnabled(context.Background(), true, 1, 1)
	require.NoError(t, err)
	require.NoError(t, s.Apply(context.Background(), "preview", time.Now(), 0, 1))
	require.True(t, repo.controlled)
}
