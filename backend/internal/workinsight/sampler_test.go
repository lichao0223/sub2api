package workinsight

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestProcessCandidateReleasesCoverageWhenSelectedBodyHasNoUserPrompt(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	service := &Service{redis: client}
	cfg := storedConfig{Config: DefaultConfig()}
	cfg.Enabled, cfg.SampleRate = true, 0
	service.config.Store(&cfg)

	err := service.processCandidate(context.Background(), securityaudit.Request{
		RequestID: "assistant-only", UserID: 42, Protocol: "openai_chat",
		Body: []byte(`{"messages":[{"role":"assistant","content":"not user work"}]}`),
	})
	require.NoError(t, err)

	now := time.Now()
	date := now.In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02")
	retry, err := service.sample(context.Background(), cfg, 42, date, now, "next-request")
	require.NoError(t, err)
	require.True(t, retry.selected)
}

func TestPayloadSlotsAreBoundedAndReleased(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	service := &Service{redis: client}
	ctx := context.Background()

	reserved, err := service.reservePayloadSlot(ctx, 1, time.Hour, 1)
	require.NoError(t, err)
	require.True(t, reserved)
	reserved, err = service.reservePayloadSlot(ctx, 2, time.Hour, 1)
	require.NoError(t, err)
	require.False(t, reserved)
	service.releasePayloadSlots(ctx, []string{"1"})
	reserved, err = service.reservePayloadSlot(ctx, 2, time.Hour, 1)
	require.NoError(t, err)
	require.True(t, reserved)
}

func TestSamplingSessionCoverageReleaseLimitsAndStableRate(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	service := &Service{redis: client}
	cfg := storedConfig{Config: DefaultConfig()}
	cfg.SampleRate = 0
	now := time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC)
	ctx := context.Background()

	first, err := service.sample(ctx, cfg, 1, "2026-08-11", now, "request-1")
	require.NoError(t, err)
	require.True(t, first.selected)
	require.Equal(t, "session_coverage", first.reason)
	require.NoError(t, service.finishSample(ctx, 1, "2026-08-11", first, false))

	retry, err := service.sample(ctx, cfg, 1, "2026-08-11", now.Add(time.Minute), "request-2")
	require.NoError(t, err)
	require.True(t, retry.selected, "failed coverage delivery must release the session claim")
	require.Equal(t, first.sessionID, retry.sessionID)
	require.NoError(t, service.finishSample(ctx, 1, "2026-08-11", retry, true))

	miss, err := service.sample(ctx, cfg, 1, "2026-08-11", now.Add(2*time.Minute), "request-3")
	require.NoError(t, err)
	require.False(t, miss.selected)
	require.Equal(t, "rate_miss", miss.reason)

	boundary, err := service.sample(ctx, cfg, 2, "2026-08-11", now, "boundary-1")
	require.NoError(t, err)
	require.NoError(t, service.finishSample(ctx, 2, "2026-08-11", boundary, true))
	atFive, err := service.sample(ctx, cfg, 2, "2026-08-11", now.Add(5*time.Minute), "boundary-2")
	require.NoError(t, err)
	require.False(t, atFive.selected)
	require.Equal(t, boundary.sessionID, atFive.sessionID, "exactly five minutes remains in the same session")
	afterFive, err := service.sample(ctx, cfg, 2, "2026-08-11", now.Add(10*time.Minute+time.Millisecond), "boundary-3")
	require.NoError(t, err)
	require.True(t, afterFive.selected)
	require.NotEqual(t, boundary.sessionID, afterFive.sessionID)

	limited := cfg
	limited.UserDailyLimit = 1
	limited.GlobalDailyLimit = 10
	one, err := service.sample(ctx, limited, 3, "2026-08-11", now, "limit-1")
	require.NoError(t, err)
	require.NoError(t, service.finishSample(ctx, 3, "2026-08-11", one, true))
	two, err := service.sample(ctx, limited, 3, "2026-08-11", now.Add(6*time.Minute), "limit-2")
	require.NoError(t, err)
	require.False(t, two.selected)
	require.Equal(t, "user_limit", two.reason)

	globalLimited := cfg
	globalLimited.UserDailyLimit, globalLimited.GlobalDailyLimit = 10, 1
	globalOne, err := service.sample(ctx, globalLimited, 10, "2026-08-12", now, "global-1")
	require.NoError(t, err)
	require.NoError(t, service.finishSample(ctx, 10, "2026-08-12", globalOne, true))
	globalTwo, err := service.sample(ctx, globalLimited, 11, "2026-08-12", now, "global-2")
	require.NoError(t, err)
	require.False(t, globalTwo.selected)
	require.Equal(t, "global_limit", globalTwo.reason)

	stable := cfg
	stable.SampleRate = 50
	covered, err := service.sample(ctx, stable, 4, "2026-08-11", now, "coverage")
	require.NoError(t, err)
	require.NoError(t, service.finishSample(ctx, 4, "2026-08-11", covered, true))
	a, err := service.sample(ctx, stable, 4, "2026-08-11", now.Add(time.Minute), "same-request")
	require.NoError(t, err)
	if a.selected {
		require.NoError(t, service.finishSample(ctx, 4, "2026-08-11", a, false))
	}
	b, err := service.sample(ctx, stable, 4, "2026-08-11", now.Add(2*time.Minute), "same-request")
	require.NoError(t, err)
	require.Equal(t, a.selected, b.selected, "probability sampling must be stable for the same request fingerprint")
}
