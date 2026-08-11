package workinsight

import (
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
)

func BenchmarkTrySubmit(b *testing.B) {
	for _, size := range []int{4 << 10, 64 << 10, 1 << 20} {
		b.Run(fmt.Sprintf("enabled/%dKiB", size>>10), func(b *testing.B) {
			benchmarkTrySubmit(b, true, size, 100)
		})
	}
	b.Run("rate_miss/1024KiB", func(b *testing.B) { benchmarkTrySubmit(b, true, 1<<20, 0) })
	b.Run("disabled", func(b *testing.B) { benchmarkTrySubmit(b, false, 64<<10, 100) })
}

func benchmarkTrySubmit(b *testing.B, enabled bool, size, rate int) {
	service := &Service{queue: make(chan queuedRequest, 1)}
	cfg := storedConfig{Config: DefaultConfig()}
	cfg.Enabled = enabled
	cfg.SampleRate = rate
	service.config.Store(&cfg)
	request := securityaudit.Request{RequestID: "benchmark", Body: make([]byte, size)}
	if enabled && rate < 100 {
		service.ingressSelected(cfg, request, time.Now())
	}
	b.SetBytes(int64(size))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		service.TrySubmit(request)
		if enabled && cfg.SampleRate > 0 {
			queued := <-service.queue
			service.queuedCount.Add(-1)
			service.queuedBytes.Add(-queued.bytes)
		}
	}
}
