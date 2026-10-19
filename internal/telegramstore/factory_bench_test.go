package telegramstore

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

type nopInvoker struct{}

func (nopInvoker) Invoke(context.Context, bin.Encoder, bin.Decoder) error { return nil }

// BenchmarkBotRateLimitThroughput measures the steady-state getFile request
// rate that the factory middleware allows when several bot accounts download
// concurrently with production rate-limit defaults. Telegram itself is replaced
// by a no-op invoker so the middleware chain is the only bottleneck.
//
//	go test ./internal/telegramstore -run '^$' -bench BotRateLimitThroughput -benchtime 200x
func BenchmarkBotRateLimitThroughput(b *testing.B) {
	for _, bots := range []int{1, 4, 8} {
		b.Run(fmt.Sprintf("bots=%d", bots), func(b *testing.B) {
			const burst = 10
			factory, err := NewFactory(FactoryConfig{
				AppID: 1, AppHash: "bench", RateLimit: true, RateInterval: 50 * time.Millisecond, RateBurst: burst,
			})
			if err != nil {
				b.Fatal(err)
			}
			ctx := context.Background()
			invoke := func(invoker tg.Invoker) {
				if err := invoker.Invoke(ctx, &tg.UploadGetFileRequest{}, &tg.UploadFile{}); err != nil {
					b.Error(err)
				}
			}
			invokers := make([]tg.Invoker, bots)
			for i := range invokers {
				invokers[i] = factory.wrapInvoker(int64(7_000_000_000+i), nopInvoker{})
				for range burst {
					invoke(invokers[i]) // Exclude the initial token burst from the steady-state rate.
				}
			}

			b.ResetTimer()
			var issued atomic.Int64
			var workers sync.WaitGroup
			for worker := range bots * defaultTelegramReadParallel {
				invoker := invokers[worker%bots]
				workers.Go(func() {
					for issued.Add(1) <= int64(b.N) {
						invoke(invoker)
					}
				})
			}
			workers.Wait()
			b.StopTimer()

			seconds := b.Elapsed().Seconds()
			b.ReportMetric(float64(b.N)/seconds, "req/s")
			b.ReportMetric(float64(b.N)*telegramReadChunk/(1<<20)/seconds, "MiB/s")
		})
	}
}
