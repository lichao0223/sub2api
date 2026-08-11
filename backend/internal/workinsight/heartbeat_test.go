package workinsight

import (
	"context"
	"errors"
	"testing"
	"time"

	appservice "github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type heartbeatOps struct {
	appservice.OpsRepository
	inputs []*appservice.OpsUpsertJobHeartbeatInput
	err    error
}

func (f *heartbeatOps) UpsertJobHeartbeat(_ context.Context, input *appservice.OpsUpsertJobHeartbeatInput) error {
	f.inputs = append(f.inputs, input)
	return f.err
}

func TestWorkInsightJobsReuseOpsHeartbeatsAndIgnoreHeartbeatFailure(t *testing.T) {
	ops := &heartbeatOps{}
	service := &Service{ops: ops}
	for _, name := range []string{jobBatchScheduler, jobReconciliation, jobDailyFinalize, jobCleanup} {
		service.reportJob(name, time.Now().Add(-time.Millisecond), "canary", nil)
	}
	require.Len(t, ops.inputs, 4)
	for _, input := range ops.inputs {
		require.NotNil(t, input.LastSuccessAt)
		require.Equal(t, "canary", *input.LastResult)
		require.Nil(t, input.LastError)
	}

	ops.err = errors.New("heartbeat unavailable")
	service.reportJob(jobCleanup, time.Now(), "", errors.New("cleanup failed"))
	require.Equal(t, jobCleanup+"_failed", *ops.inputs[4].LastError)
}
