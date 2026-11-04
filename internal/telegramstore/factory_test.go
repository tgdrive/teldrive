package telegramstore

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/clock"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

func TestFactoryNTPIsOptInAndShared(t *testing.T) {
	t.Parallel()
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			calls := 0
			corrected := &fixedTelegramClock{Clock: clock.System, now: time.Now().Add(time.Hour)}
			factory, err := newFactory(FactoryConfig{
				AppID: 1, AppHash: "test", NTP: enabled, NTPServer: "time.example.org",
			}, func(hosts ...string) (clock.Clock, error) {
				calls++
				if len(hosts) != 1 || hosts[0] != "time.example.org" {
					t.Fatalf("NTP hosts = %v", hosts)
				}
				return corrected, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if enabled {
				if factory.clock != corrected || !factory.clock.Now().Equal(corrected.now) {
					t.Fatal("factory did not retain the NTP-corrected clock")
				}
			} else if factory.clock != clock.System {
				t.Fatal("disabled NTP must use the system clock")
			}
			for range 2 {
				if _, err := factory.New(new(session.StorageMemory)); err != nil {
					t.Fatal(err)
				}
				if _, err := factory.NewForAccount(new(session.StorageMemory), 42, nil); err != nil {
					t.Fatal(err)
				}
			}
			wantCalls := 0
			if enabled {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("NTP queries = %d, want %d", calls, wantCalls)
			}
		})
	}
}

type fixedTelegramClock struct {
	clock.Clock
	now time.Time
}

func (c *fixedTelegramClock) Now() time.Time { return c.now }

func TestFactoryNTPDefaultServerAndFailure(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("NTP unreachable")
	factory, err := newFactory(FactoryConfig{AppID: 1, AppHash: "test", NTP: true}, func(hosts ...string) (clock.Clock, error) {
		if len(hosts) != 1 || hosts[0] != "pool.ntp.org" {
			t.Fatalf("default NTP hosts = %v", hosts)
		}
		return nil, wantErr
	})
	if factory != nil || !errors.Is(err, wantErr) {
		t.Fatalf("factory = %v, error = %v; want NTP failure", factory, err)
	}
}

func TestFactoryRateLimitsEachTelegramAccountSeparately(t *testing.T) {
	t.Parallel()
	factory, err := NewFactory(FactoryConfig{
		AppID: 1, AppHash: "test", RateLimit: true, RateInterval: time.Hour, RateBurst: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Each call wraps a new invoker, like a new request-scoped client or pool.
	invoke := func(accountID int64) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		return factory.wrapInvoker(accountID, nopInvoker{}).Invoke(ctx, &tg.UploadGetFileRequest{}, &tg.UploadFile{})
	}

	if err := invoke(101); err != nil {
		t.Fatalf("first request for bot 101 = %v", err)
	}
	if err := invoke(101); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second client for bot 101 = %v, want shared budget exhausted", err)
	}
	if err := invoke(102); err != nil {
		t.Fatalf("bot 102 throttled by bot 101: %v", err)
	}
	for range 2 {
		if err := invoke(0); err != nil {
			t.Fatalf("unbound client shared a budget: %v", err)
		}
	}
}

type recordingDCPoolClient struct {
	home     telegram.CloseInvoker
	poolSize int64
	openDC   func(context.Context, int, int64) (telegram.CloseInvoker, error)
}

func (c *recordingDCPoolClient) Pool(size int64) (telegram.CloseInvoker, error) {
	c.poolSize = size
	return c.home, nil
}

func (c *recordingDCPoolClient) DC(ctx context.Context, dc int, size int64) (telegram.CloseInvoker, error) {
	return c.openDC(ctx, dc, size)
}

func TestFactoryPooledMigrationRetainsConnectionCountAndReusesDCs(t *testing.T) {
	for _, redirect := range []string{"FILE_MIGRATE", "STATS_MIGRATE"} {
		t.Run(redirect, func(t *testing.T) {
			factory, err := NewFactory(FactoryConfig{AppID: 1, AppHash: "test"})
			if err != nil {
				t.Fatal(err)
			}
			ctx := WithClientID(context.Background(), 42)
			homeCalls := 0
			home := &testDCPool{InvokeFunc: func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
				homeCalls++
				dc := input.(*tg.UploadGetFileRequest).Location.(*tg.InputDocumentFileLocation).ID
				if dc != 1 {
					return tgerr.New(303, fmt.Sprintf("%s_%d", redirect, dc))
				}
				output.(*tg.UploadFileBox).File = &tg.UploadFile{Bytes: []byte{1}}
				return nil
			}}
			targets := make(map[int]*testDCPool)
			client := &recordingDCPoolClient{home: home, openDC: func(gotCtx context.Context, dc int, size int64) (telegram.CloseInvoker, error) {
				if gotCtx != ctx || size != 8 {
					t.Errorf("DC %d opened with wrong context or connection count %d", dc, size)
				}
				if targets[dc] != nil {
					t.Errorf("DC %d pool opened more than once", dc)
				}
				pool := &testDCPool{InvokeFunc: func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
					if id := input.(*tg.UploadGetFileRequest).Location.(*tg.InputDocumentFileLocation).ID; id != int64(dc) {
						t.Errorf("request for file DC %d sent to DC %d", id, dc)
					}
					output.(*tg.UploadFileBox).File = &tg.UploadFile{Bytes: []byte{byte(dc)}}
					return nil
				}}
				targets[dc] = pool
				return pool, nil
			}}
			api, closePool, err := factory.pooledAPI(client, 42, 8)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := closePool(); err != nil {
					t.Error(err)
				}
			})
			for _, dc := range []int64{4, 1, 2, 4, 1, 2} {
				result, err := api.UploadGetFile(ctx, &tg.UploadGetFileRequest{Location: &tg.InputDocumentFileLocation{ID: dc}})
				if err != nil {
					t.Fatal(err)
				}
				if got := result.(*tg.UploadFile).Bytes; len(got) != 1 || got[0] != byte(dc) {
					t.Fatalf("incorrect file bytes from DC %d", dc)
				}
			}
			if client.poolSize != 8 || homeCalls != 6 || len(targets) != 2 {
				t.Fatalf("home pool size=%d home calls=%d target pools=%d", client.poolSize, homeCalls, len(targets))
			}
			if err := closePool(); err != nil {
				t.Fatal(err)
			}
			if home.closes.Load() != 1 || targets[2].closes.Load() != 1 || targets[4].closes.Load() != 1 {
				t.Fatal("home and target pools must all close exactly once")
			}
		})
	}
}
