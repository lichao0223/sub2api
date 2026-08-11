package workinsight

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/stretchr/testify/require"
)

func TestTrySubmitIsBoundedAndHonorsDisabledAndExcludedUsers(t *testing.T) {
	service := &Service{queue: make(chan queuedRequest, 1)}
	cfg := storedConfig{Config: DefaultConfig()}
	service.config.Store(&cfg)
	request := securityaudit.Request{UserID: 7, UserEmail: "canary@example.test", Body: []byte(`{"messages":[]}`)}

	service.TrySubmit(request)
	require.Empty(t, service.queue)

	cfg.Enabled, cfg.ExcludedUserIDs = true, []int64{7}
	service.config.Store(&cfg)
	service.TrySubmit(request)
	require.Empty(t, service.queue)

	cfg.ExcludedUserIDs = nil
	cfg.SampleRate = 100
	service.config.Store(&cfg)
	service.TrySubmit(request)
	require.Len(t, service.queue, 1)
	require.Equal(t, int64(1), service.queuedCount.Load())
	require.Equal(t, int64(len(request.Body)), service.queuedBytes.Load())
	service.TrySubmit(request)
	require.Equal(t, int64(1), service.dropped.Load())
	require.Equal(t, int64(len(request.Body)), service.queuedBytes.Load())
}

func TestTrySubmitEnforcesQueueByteBudgetBeforeCopy(t *testing.T) {
	service := &Service{queue: make(chan queuedRequest, DefaultIngressLimit)}
	cfg := storedConfig{Config: DefaultConfig()}
	cfg.Enabled = true
	cfg.SampleRate = 100
	service.config.Store(&cfg)
	service.queuedBytes.Store(maxIngressQueueBytes - 1)

	service.TrySubmit(securityaudit.Request{Body: []byte(`{}`)})

	require.Empty(t, service.queue)
	require.Zero(t, service.queuedCount.Load())
	require.Equal(t, int64(maxIngressQueueBytes-1), service.queuedBytes.Load())
	require.Equal(t, int64(1), service.dropped.Load())
}

func TestTrySubmitDropsRateMissBeforeBodyCopy(t *testing.T) {
	service := &Service{queue: make(chan queuedRequest, 1)}
	cfg := storedConfig{Config: DefaultConfig()}
	cfg.Enabled, cfg.SampleRate = true, 0
	service.config.Store(&cfg)

	service.TrySubmit(securityaudit.Request{RequestID: "rate-miss", Body: make([]byte, 1<<20)})

	require.Empty(t, service.queue)
	require.Zero(t, service.queuedBytes.Load())
}
