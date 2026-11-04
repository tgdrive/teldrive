package telegramstore

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

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
