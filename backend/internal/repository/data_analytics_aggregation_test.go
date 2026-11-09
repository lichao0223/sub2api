package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsBatchFailureRetriesTimeoutsSooner(t *testing.T) {
	for _, err := range []error{context.DeadlineExceeded, &pq.Error{Code: "57014", Message: "canceling statement due to user request"}} {
		message, retry := analyticsBatchFailure(err)
		require.Equal(t, 5*time.Minute, retry)
		require.Contains(t, message, "5分钟后自动重试")
		require.NotContains(t, message, "pq:")
	}
	message, retry := analyticsBatchFailure(errors.New("invalid price"))
	require.Equal(t, "invalid price", message)
	require.Equal(t, time.Hour, retry)
}
