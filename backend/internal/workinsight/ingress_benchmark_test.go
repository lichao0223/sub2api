package workinsight

import (
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
)

func BenchmarkTrySubmit(b *testing.B) {
	for _, size := range []int{4 << 10, 64 << 10, 1 << 20} {
		b.Run(fmt.Sprintf("enabled/%dKiB", size>>10), func(b *testing.B) {
			benchmarkTrySubmit(b, true, size)
		})
	}
	b.Run("disabled", func(b *testing.B) { benchmarkTrySubmit(b, false, 64<<10) })
}

func benchmarkTrySubmit(b *testing.B, enabled bool, size int) {
	service := &Service{queue: make(chan queuedRequest, 1)}
	cfg := storedConfig{Config: DefaultConfig()}
	cfg.Enabled = enabled
	service.config.Store(&cfg)
	request := securityaudit.Request{RequestID: "benchmark", Body: make([]byte, size)}
	b.SetBytes(int64(size))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		service.TrySubmit(request)
		if enabled {
			queued := <-service.queue
			service.queuedCount.Add(-1)
			service.queuedBytes.Add(-queued.bytes)
		}
	}
}
