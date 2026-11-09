package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
)

const AnalyticsTaskSettingKey = "data_analytics.tasks_enabled"

var (
	ErrAnalyticsTasksPaused     = errors.New("data analytics tasks are paused")
	ErrAnalyticsControlConflict = errors.New("data analytics settings changed; refresh and try again")
)

type AnalyticsTaskControl struct {
	Enabled   bool      `json:"enabled"`
	Version   int64     `json:"version"`
	UpdatedBy int64     `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AnalyticsTaskWorker struct {
	InstanceID         string     `json:"instance_id"`
	Running            int        `json:"running"`
	ObservedVersion    int64      `json:"observed_version"`
	LastSeenAt         time.Time  `json:"last_seen_at"`
	LastSuccessAt      *time.Time `json:"last_success_at"`
	AnalyticsDay       string     `json:"analytics_day"`
	AnalyticsStartedAt *time.Time `json:"analytics_started_at"`
}

type AnalyticsProgress struct {
	DirectoryReady  bool       `json:"directory_ready"`
	Seeded          bool       `json:"seeded"`
	TotalDays       int64      `json:"total_days"`
	CompletedDays   int64      `json:"completed_days"`
	BacklogDays     int64      `json:"backlog_days"`
	FailedDays      int64      `json:"failed_days"`
	PendingRequests int64      `json:"pending_requests"`
	RetryAt         *time.Time `json:"retry_at"`
	LastSuccessAt   *time.Time `json:"last_success_at"`
	LastError       string     `json:"last_error"`
}

type AnalyticsTaskStatus struct {
	Control            AnalyticsTaskControl `json:"control"`
	State              string               `json:"state"`
	Running            int                  `json:"running"`
	LastSuccessAt      *time.Time           `json:"last_success_at"`
	LastSeenAt         *time.Time           `json:"last_seen_at"`
	AnalyticsDay       string               `json:"analytics_day"`
	AnalyticsStartedAt *time.Time           `json:"analytics_started_at"`
	Progress           *AnalyticsProgress   `json:"progress"`
}

type AnalyticsTaskRepository interface {
	GetAnalyticsTaskControl(context.Context) (AnalyticsTaskControl, error)
	UpdateAnalyticsTaskControl(context.Context, bool, int64, int64) (AnalyticsTaskControl, error)
	RecordAnalyticsTaskWorker(context.Context, AnalyticsTaskWorker) error
	ListAnalyticsTaskWorkers(context.Context) ([]AnalyticsTaskWorker, error)
	GetAnalyticsProgress(context.Context) (*AnalyticsProgress, error)
}

// mu serializes control reads, registration and completion, so an older read
// cannot start work after a local pause has been acknowledged.
type AnalyticsTaskService struct {
	repo            AnalyticsTaskRepository
	mu              sync.Mutex
	worker          AnalyticsTaskWorker
	tasks           map[uint64]context.CancelFunc
	nextID          uint64
	analyticsTaskID uint64
}

func NewAnalyticsTaskService(repo AnalyticsTaskRepository, instanceID string) *AnalyticsTaskService {
	return &AnalyticsTaskService{repo: repo, worker: AnalyticsTaskWorker{InstanceID: instanceID}, tasks: make(map[uint64]context.CancelFunc)}
}

func ProvideAnalyticsTaskService(repo AnalyticsTaskRepository, wheel *TimingWheelService) *AnalyticsTaskService {
	host, _ := os.Hostname()
	s := NewAnalyticsTaskService(repo, fmt.Sprintf("%s:%s", host, uuid.NewString()))
	refresh := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.Refresh(ctx); err != nil && !errors.Is(err, ErrAnalyticsTasksPaused) {
			logger.LegacyPrintf("service.data_analytics", "task control refresh failed: %v", err)
		}
	}
	refresh()
	wheel.ScheduleRecurring("analytics:task-control", 5*time.Second, refresh)
	return s
}

type analyticsTaskVersionKey struct{}
type analyticsDayReporterKey struct{}

// Report outside the publishing transaction so readers can see the day while it runs.
func ReportAnalyticsDay(ctx context.Context, day string) error {
	if report, ok := ctx.Value(analyticsDayReporterKey{}).(func(context.Context, string) error); ok {
		return report(ctx, day)
	}
	return nil
}

func AnalyticsTaskVersion(ctx context.Context) (int64, bool) {
	version, ok := ctx.Value(analyticsTaskVersionKey{}).(int64)
	return version, ok
}

func (s *AnalyticsTaskService) cancelLocked() {
	for _, cancel := range s.tasks {
		cancel()
	}
}

func (s *AnalyticsTaskService) refreshLocked(ctx context.Context) (AnalyticsTaskControl, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	control, err := s.repo.GetAnalyticsTaskControl(ctx)
	if err != nil {
		s.cancelLocked()
		return AnalyticsTaskControl{}, err
	}
	if !control.Enabled || (s.worker.ObservedVersion != 0 && control.Version != s.worker.ObservedVersion) {
		s.cancelLocked()
	}
	s.worker.ObservedVersion = control.Version
	return control, nil
}

func (s *AnalyticsTaskService) recordLocked(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	s.worker.Running = len(s.tasks)
	s.worker.LastSeenAt = time.Now().UTC()
	if err := s.repo.RecordAnalyticsTaskWorker(ctx, s.worker); err != nil {
		s.cancelLocked()
		return err
	}
	return nil
}

func (s *AnalyticsTaskService) Refresh(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.refreshLocked(ctx); err != nil {
		return err
	}
	return s.recordLocked(ctx)
}

func (s *AnalyticsTaskService) RequireEnabled(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	control, err := s.refreshLocked(ctx)
	if err != nil {
		return err
	}
	if !control.Enabled {
		return ErrAnalyticsTasksPaused
	}
	return nil
}

func (s *AnalyticsTaskService) SetEnabled(ctx context.Context, enabled bool, version, actorID int64) (AnalyticsTaskControl, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	control, err := s.repo.UpdateAnalyticsTaskControl(ctx, enabled, version, actorID)
	if err != nil {
		return AnalyticsTaskControl{}, err
	}
	s.cancelLocked()
	s.worker.ObservedVersion = control.Version
	// The setting is already durable. A failed heartbeat is reflected as an
	// unacknowledged instance in Status, not as a failed setting save.
	_ = s.recordLocked(ctx)
	return control, nil
}

func (s *AnalyticsTaskService) Run(ctx context.Context, work func(context.Context) error) error {
	s.mu.Lock()
	control, err := s.refreshLocked(ctx)
	if err == nil && !control.Enabled {
		err = ErrAnalyticsTasksPaused
	}
	if err != nil {
		s.mu.Unlock()
		return err
	}
	taskCtx, cancel := context.WithCancel(context.WithValue(ctx, analyticsTaskVersionKey{}, control.Version))
	s.nextID++
	id := s.nextID
	s.tasks[id] = cancel
	taskCtx = context.WithValue(taskCtx, analyticsDayReporterKey{}, func(reportCtx context.Context, day string) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := reportCtx.Err(); err != nil {
			return err
		}
		s.analyticsTaskID = id
		if s.worker.AnalyticsDay != day || s.worker.AnalyticsStartedAt == nil {
			now := time.Now().UTC()
			s.worker.AnalyticsDay, s.worker.AnalyticsStartedAt = day, &now
		}
		return s.recordLocked(reportCtx)
	})
	err = s.recordLocked(ctx)
	if err != nil {
		delete(s.tasks, id)
		cancel()
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	// Always release registration, including when a caller panics.
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.tasks, id)
		if s.analyticsTaskID == id {
			s.worker.AnalyticsDay, s.worker.AnalyticsStartedAt = "", nil
			s.analyticsTaskID = 0
		}
		cancel()
		recordCtx, recordCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer recordCancel()
		_ = s.recordLocked(recordCtx)
	}()
	if err := taskCtx.Err(); err != nil {
		return err
	}
	err = work(taskCtx)
	if err == nil {
		err = taskCtx.Err()
	}
	if err == nil {
		s.mu.Lock()
		if taskCtx.Err() == nil {
			now := time.Now().UTC()
			s.worker.LastSuccessAt = &now
		} else {
			err = taskCtx.Err()
		}
		s.mu.Unlock()
	}
	return err
}

func (s *AnalyticsTaskService) Status(ctx context.Context) (*AnalyticsTaskStatus, error) {
	control, err := s.repo.GetAnalyticsTaskControl(ctx)
	if err != nil {
		return nil, err
	}
	workers, err := s.repo.ListAnalyticsTaskWorkers(ctx)
	if err != nil {
		return nil, err
	}
	status := &AnalyticsTaskStatus{Control: control, State: "paused"}
	status.Progress, err = s.repo.GetAnalyticsProgress(ctx)
	if err != nil {
		return nil, err
	}
	if control.Enabled {
		status.State = "idle"
	}
	heartbeatCutoff := time.Now().Add(-15 * time.Second)
	staleRunning, unacknowledged := false, false
	for _, worker := range workers {
		if status.LastSeenAt == nil || worker.LastSeenAt.After(*status.LastSeenAt) {
			seen := worker.LastSeenAt
			status.LastSeenAt = &seen
		}
		if worker.LastSuccessAt != nil && (status.LastSuccessAt == nil || worker.LastSuccessAt.After(*status.LastSuccessAt)) {
			status.LastSuccessAt = worker.LastSuccessAt
		}
		if worker.LastSeenAt.Before(heartbeatCutoff) {
			staleRunning = staleRunning || worker.Running > 0
			continue
		}
		if worker.AnalyticsDay != "" && worker.AnalyticsStartedAt != nil &&
			(status.AnalyticsStartedAt == nil || worker.AnalyticsStartedAt.After(*status.AnalyticsStartedAt)) {
			status.AnalyticsDay, status.AnalyticsStartedAt = worker.AnalyticsDay, worker.AnalyticsStartedAt
		}
		status.Running += worker.Running
		if !control.Enabled && worker.ObservedVersion != control.Version {
			unacknowledged = true
		}
	}
	if status.Running > 0 || unacknowledged {
		if control.Enabled {
			status.State = "running"
		} else {
			status.State = "pausing"
		}
	}
	// An old instance must not hide live work after a restart, but cannot acknowledge a pause.
	if (!control.Enabled && staleRunning) || (control.Enabled && (status.LastSeenAt == nil || status.LastSeenAt.Before(heartbeatCutoff))) {
		status.State = "unknown"
	}
	return status, nil
}
