//go:build integration

package botgateway

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"golang.org/x/time/rate"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

type benchJob struct {
	requests    int
	connections int
}

// BenchmarkDownloadBotSelection replays a fixed arrival order of download jobs
// through UploadAwareRunner. Each simulated bot serves one request per
// millisecond, so jobs stacked on the same bot share its budget. Elapsed time is
// therefore set by the most loaded bot.
//
//	scripts/test-postgres.sh go test -tags=integration ./internal/botgateway -run '^$' -bench DownloadBotSelection -benchtime 20x
func BenchmarkDownloadBotSelection(b *testing.B) {
	const bots = 4
	stream := benchJob{requests: 64, connections: 4}
	lookup := benchJob{requests: 1, connections: 1}
	workloads := map[string][]benchJob{
		"uniform": repeatJobs(16, stream),
		"mixed":   repeatJobs(16, lookup, stream),
	}
	db := testpostgres.New(b)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		b.Fatal(err)
	}
	for botID := range int64(bots) {
		if _, err := db.Pool.Exec(ctx, `
INSERT INTO bots (bot_id, user_id, token_ciphertext, enabled) VALUES ($1, 1001, $2, true)`, 101+botID, []byte("ciphertext")); err != nil {
			b.Fatal(err)
		}
	}
	cipher, err := secureblob.NewWithKey(bytes.Repeat([]byte{3}, 32), bytes.NewReader(bytes.Repeat([]byte{4}, 24)))
	if err != nil {
		b.Fatal(err)
	}
	factory, err := telegramstore.NewFactory(telegramstore.FactoryConfig{AppID: 12345, AppHash: "test-hash"})
	if err != nil {
		b.Fatal(err)
	}

	for _, name := range []string{"uniform", "mixed"} {
		jobs := workloads[name]
		b.Run(fmt.Sprintf("bots=%d/%s", bots, name), func(b *testing.B) {
			runner, err := NewUploadAwareRunner(db.Pool, cipher, factory, &recordingRunner{}, bots)
			if err != nil {
				b.Fatal(err)
			}
			budgets := make(map[int64]*rate.Limiter, bots)
			for botID := range int64(bots) {
				budgets[101+botID] = rate.NewLimiter(rate.Every(time.Millisecond), 1)
			}
			started := make(chan struct{})
			runner.runBotFunc = func(ctx context.Context, _ int64, bot *sqlcgen.Bot, _ int, fn func(context.Context, *tg.Client) error) error {
				started <- struct{}{}
				return fn(telegramstore.WithClientID(ctx, bot.BotID), nil)
			}
			totalRequests := 0
			for _, job := range jobs {
				totalRequests += job.requests
			}

			b.ResetTimer()
			for range b.N {
				var running sync.WaitGroup
				for _, job := range jobs {
					running.Go(func() {
						err := runner.RunPooled(ctx, 1001, telegramstore.OperationDownload, job.connections, func(ctx context.Context, _ *tg.Client) error {
							botID, _ := telegramstore.ClientID(ctx)
							for range job.requests {
								if err := budgets[botID].Wait(ctx); err != nil {
									return err
								}
							}
							return nil
						})
						if err != nil {
							b.Error(err)
						}
					})
					<-started // Fix the arrival order: the next job is submitted after this one is placed.
				}
				running.Wait()
			}
			b.StopTimer()
			b.ReportMetric(float64(totalRequests*b.N)/b.Elapsed().Seconds(), "req/s")
		})
	}
}

func repeatJobs(count int, pattern ...benchJob) []benchJob {
	jobs := make([]benchJob, 0, count*len(pattern))
	for range count {
		jobs = append(jobs, pattern...)
	}
	return jobs
}
