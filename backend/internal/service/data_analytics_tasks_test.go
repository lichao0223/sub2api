package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type analyticsTaskMemoryRepo struct {
	mu      sync.Mutex
	control AnalyticsTaskControl
	workers map[string]AnalyticsTaskWorker
	readErr error
	saveErr error
}

func newAnalyticsTaskMemoryRepo() *analyticsTaskMemoryRepo {
	return &analyticsTaskMemoryRepo{control: AnalyticsTaskControl{Version: 1}, workers: map[string]AnalyticsTaskWorker{}}
}

func (r *analyticsTaskMemoryRepo) GetAnalyticsTaskControl(context.Context) (AnalyticsTaskControl, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.control, r.readErr
}

func (r *analyticsTaskMemoryRepo) UpdateAnalyticsTaskControl(_ context.Context, enabled bool, version, actorID int64) (AnalyticsTaskControl, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.saveErr != nil {
		return AnalyticsTaskControl{}, r.saveErr
	}
	if version != r.control.Version {
		return AnalyticsTaskControl{}, ErrAnalyticsControlConflict
	}
	r.control.Enabled, r.control.Version, r.control.UpdatedBy = enabled, version+1, actorID
	r.control.UpdatedAt = time.Now().UTC()
	return r.control, nil
}

func (r *analyticsTaskMemoryRepo) RecordAnalyticsTaskWorker(_ context.Context, worker AnalyticsTaskWorker) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workers[worker.InstanceID] = worker
	return nil
}

func (r *analyticsTaskMemoryRepo) ListAnalyticsTaskWorkers(context.Context) ([]AnalyticsTaskWorker, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]AnalyticsTaskWorker, 0, len(r.workers))
	for _, worker := range r.workers {
		out = append(out, worker)
	}
	return out, nil
}

func (*analyticsTaskMemoryRepo) GetAnalyticsProgress(context.Context) (*AnalyticsProgress, error) {
	return &AnalyticsProgress{}, nil
}

func TestAnalyticsDayProgressIsVisibleDuringWorkAndClearedOnCancellation(t *testing.T) {
	repo := newAnalyticsTaskMemoryRepo()
	repo.control.Enabled = true
	s := NewAnalyticsTaskService(repo, "progress")
	err := s.Run(context.Background(), func(ctx context.Context) error {
		require.NoError(t, ReportAnalyticsDay(ctx, "2026-11-01"))
		first, err := s.Status(ctx)
		require.NoError(t, err)
		require.NoError(t, ReportAnalyticsDay(ctx, "2026-11-01"))
		status, err := s.Status(ctx)
		require.NoError(t, err)
		require.Equal(t, first.AnalyticsStartedAt, status.AnalyticsStartedAt)
		require.Equal(t, "running", status.State)
		require.Equal(t, "2026-11-01", status.AnalyticsDay)
		require.NotNil(t, status.AnalyticsStartedAt)
		require.NotNil(t, status.LastSeenAt)
		_, err = s.SetEnabled(context.Background(), false, 1, 7)
		require.NoError(t, err)
		return ctx.Err()
	})
	require.ErrorIs(t, err, context.Canceled)
	status, err := s.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, "paused", status.State)
	require.Empty(t, status.AnalyticsDay)
	require.Nil(t, status.AnalyticsStartedAt)
}

func TestAnalyticsEnabledWithoutWorkerHeartbeatIsUnknown(t *testing.T) {
	repo := newAnalyticsTaskMemoryRepo()
	repo.control.Enabled = true
	s := NewAnalyticsTaskService(repo, "no-worker")
	status, err := s.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, "unknown", status.State)
}

func TestAnalyticsTasksRestartIgnoresStaleRunningWorker(t *testing.T) {
	ctx := context.Background()
	repo := newAnalyticsTaskMemoryRepo()
	repo.control.Enabled = true
	old := time.Now().Add(-time.Minute)
	repo.workers["old"] = AnalyticsTaskWorker{InstanceID: "old", Running: 1, ObservedVersion: 1, LastSeenAt: old,
		AnalyticsDay: "2026-11-08", AnalyticsStartedAt: &old}
	s := NewAnalyticsTaskService(repo, "restarted")
	require.NoError(t, s.Refresh(ctx))
	idle, err := s.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, "idle", idle.State)
	require.Zero(t, idle.Running)
	require.Empty(t, idle.AnalyticsDay)
	require.NoError(t, s.Run(ctx, func(ctx context.Context) error {
		require.NoError(t, ReportAnalyticsDay(ctx, "2026-11-09"))
		status, err := s.Status(ctx)
		require.NoError(t, err)
		require.Equal(t, "running", status.State)
		require.Equal(t, 1, status.Running)
		require.Equal(t, "2026-11-09", status.AnalyticsDay)
		return nil
	}))
}

func TestAnalyticsTasksDisabledDoesNotExecute(t *testing.T) {
	repo := newAnalyticsTaskMemoryRepo()
	s := NewAnalyticsTaskService(repo, "a")
	called := false
	err := s.Run(context.Background(), func(context.Context) error { called = true; return nil })
	require.ErrorIs(t, err, ErrAnalyticsTasksPaused)
	require.False(t, called)
	status, err := s.Status(context.Background())
	require.NoError(t, err)
	require.False(t, status.Control.Enabled)
	require.Equal(t, "paused", status.State)
}

func TestAnalyticsTasksPauseCancelsRunningWorkAndPreservesLastSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	repo := newAnalyticsTaskMemoryRepo()
	s := NewAnalyticsTaskService(repo, "a")
	_, err := s.SetEnabled(ctx, true, 1, 7)
	require.NoError(t, err)
	require.NoError(t, s.Run(ctx, func(taskCtx context.Context) error {
		version, ok := AnalyticsTaskVersion(taskCtx)
		require.True(t, ok)
		require.EqualValues(t, 2, version)
		return nil
	}))
	before, err := s.Status(ctx)
	require.NoError(t, err)
	require.NotNil(t, before.LastSuccessAt)

	entered, stopped, release := make(chan struct{}), make(chan error, 1), make(chan struct{})
	go func() {
		stopped <- s.Run(ctx, func(taskCtx context.Context) error {
			close(entered)
			<-taskCtx.Done()
			<-release
			return nil // A worker returning nil after cancellation is not a success.
		})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("work did not start")
	}
	_, err = s.SetEnabled(ctx, false, 2, 7)
	require.NoError(t, err)
	pausing, err := s.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, "pausing", pausing.State)
	close(release)
	select {
	case err = <-stopped:
		require.ErrorIs(t, err, context.Canceled)
	case <-ctx.Done():
		t.Fatal("pause did not stop work")
	}
	after, err := s.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, "paused", after.State)
	require.Equal(t, before.LastSuccessAt, after.LastSuccessAt)
}

func TestAnalyticsTasksOtherInstanceRefreshCancelsAndRestartStaysPaused(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	repo := newAnalyticsTaskMemoryRepo()
	a, b := NewAnalyticsTaskService(repo, "a"), NewAnalyticsTaskService(repo, "b")
	_, err := a.SetEnabled(ctx, true, 1, 7)
	require.NoError(t, err)
	entered, stopped := make(chan struct{}), make(chan error, 1)
	go func() {
		stopped <- b.Run(ctx, func(taskCtx context.Context) error {
			close(entered)
			<-taskCtx.Done()
			return taskCtx.Err()
		})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("work did not start")
	}
	_, err = a.SetEnabled(ctx, false, 2, 7)
	require.NoError(t, err)
	require.NoError(t, b.Refresh(ctx))
	select {
	case err = <-stopped:
		require.ErrorIs(t, err, context.Canceled)
	case <-ctx.Done():
		t.Fatal("other instance did not stop")
	}
	restarted := NewAnalyticsTaskService(repo, "c")
	require.ErrorIs(t, restarted.Run(ctx, func(context.Context) error {
		t.Fatal("restarted process ran while paused")
		return nil
	}), ErrAnalyticsTasksPaused)
	_, err = restarted.SetEnabled(ctx, true, 3, 7)
	require.NoError(t, err)
	require.NoError(t, restarted.Run(ctx, func(context.Context) error { return nil }))
}

func TestAnalyticsTasksReadErrorFailsClosedAndCancelsWork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	repo := newAnalyticsTaskMemoryRepo()
	s := NewAnalyticsTaskService(repo, "a")
	_, err := s.SetEnabled(ctx, true, 1, 7)
	require.NoError(t, err)
	entered, stopped := make(chan struct{}), make(chan error, 1)
	go func() {
		stopped <- s.Run(ctx, func(taskCtx context.Context) error {
			close(entered)
			<-taskCtx.Done()
			return taskCtx.Err()
		})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("work did not start")
	}
	repo.mu.Lock()
	repo.readErr = errors.New("storage unavailable")
	repo.mu.Unlock()
	require.Error(t, s.Refresh(ctx))
	select {
	case err = <-stopped:
		require.ErrorIs(t, err, context.Canceled)
	case <-ctx.Done():
		t.Fatal("read error did not stop work")
	}
	require.ErrorContains(t, s.Run(ctx, func(context.Context) error {
		t.Fatal("new work ran after control read failure")
		return nil
	}), "storage unavailable")
}

func TestAnalyticsTasksStaleSaveAndSaveFailureDoNotChangeControl(t *testing.T) {
	repo := newAnalyticsTaskMemoryRepo()
	s := NewAnalyticsTaskService(repo, "a")
	_, err := s.SetEnabled(context.Background(), true, 1, 7)
	require.NoError(t, err)
	_, err = s.SetEnabled(context.Background(), false, 1, 8)
	require.ErrorIs(t, err, ErrAnalyticsControlConflict)
	repo.saveErr = errors.New("save failed")
	_, err = s.SetEnabled(context.Background(), false, 2, 8)
	require.EqualError(t, err, "save failed")
	require.True(t, repo.control.Enabled)
	require.EqualValues(t, 2, repo.control.Version)
}

func TestAnalyticsTasksStaleRunningHeartbeatIsUnknown(t *testing.T) {
	repo := newAnalyticsTaskMemoryRepo()
	repo.workers["lost"] = AnalyticsTaskWorker{InstanceID: "lost", Running: 1, ObservedVersion: 1, LastSeenAt: time.Now().Add(-time.Minute)}
	s := NewAnalyticsTaskService(repo, "a")
	status, err := s.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, "unknown", status.State)
}

func TestAnalyticsTasksCanceledCallerDoesNotStartWork(t *testing.T) {
	repo := newAnalyticsTaskMemoryRepo()
	repo.control.Enabled = true
	s := NewAnalyticsTaskService(repo, "a")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.Run(ctx, func(context.Context) error {
		t.Fatal("canceled caller started work")
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	status, err := s.Status(context.Background())
	require.NoError(t, err)
	require.Nil(t, status.LastSuccessAt)
	require.Zero(t, status.Running)
}

type controlledCostRepo struct {
	CostManagementRepository
	incremental func(context.Context, int) (bool, error)
	recalculate func(context.Context) (bool, error)
}

func (r *controlledCostRepo) RunCostIncremental(ctx context.Context, limit int) (bool, error) {
	return r.incremental(ctx, limit)
}
func (r *controlledCostRepo) RunNextCostRecalculation(ctx context.Context) (bool, error) {
	return r.recalculate(ctx)
}

func TestAnalyticsTasksGuardSharedCostAndManualRecalculation(t *testing.T) {
	controller := NewAnalyticsTaskService(newAnalyticsTaskMemoryRepo(), "a")
	repo := &controlledCostRepo{
		incremental: func(context.Context, int) (bool, error) { t.Fatal("paused incremental ran"); return false, nil },
		recalculate: func(context.Context) (bool, error) { t.Fatal("paused recalculation ran"); return false, nil },
	}
	s := NewCostManagementService(repo, nil)
	s.tasks = controller
	s.runAggregation()
	day := time.Now().AddDate(0, 0, -1)
	_, err := s.CreateRecalculation(context.Background(), day, day, 7)
	require.ErrorIs(t, err, ErrAnalyticsTasksPaused)
}

func TestAnalyticsTasksSharedCostPauseDoesNotStartRecalculation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	controller := NewAnalyticsTaskService(newAnalyticsTaskMemoryRepo(), "a")
	_, err := controller.SetEnabled(ctx, true, 1, 7)
	require.NoError(t, err)
	entered, done := make(chan struct{}), make(chan struct{})
	repo := &controlledCostRepo{
		incremental: func(ctx context.Context, _ int) (bool, error) {
			close(entered)
			<-ctx.Done()
			return false, ctx.Err()
		},
		recalculate: func(context.Context) (bool, error) { t.Error("recalculation ran after pause"); return false, nil },
	}
	s := NewCostManagementService(repo, nil)
	s.tasks = controller
	go func() { defer close(done); s.runAggregation() }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("incremental did not start")
	}
	_, err = controller.SetEnabled(ctx, false, 2, 7)
	require.NoError(t, err)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("cost aggregation did not stop")
	}
}
