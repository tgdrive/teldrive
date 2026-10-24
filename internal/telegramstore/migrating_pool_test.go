package telegramstore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

type testDCPool struct {
	telegram.InvokeFunc
	closes atomic.Int32
}

func (p *testDCPool) Close() error { p.closes.Add(1); return nil }

func unwrappedPool(api tg.Invoker) tg.Invoker { return api }

func fileRedirectPool(dc int) *testDCPool {
	return &testDCPool{InvokeFunc: func(context.Context, bin.Encoder, bin.Decoder) error {
		return fmt.Errorf("invoke pool: %w", tgerr.New(303, fmt.Sprintf("FILE_MIGRATE_%d", dc)))
	}}
}

func TestMigratingPoolConcurrentReadsReuseTarget(t *testing.T) {
	home := fileRedirectPool(4)
	var opens, reads atomic.Int32
	target := &testDCPool{InvokeFunc: func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		request := input.(*tg.UploadGetFileRequest)
		output.(*tg.UploadFileBox).File = &tg.UploadFile{Bytes: []byte{byte(request.Offset)}}
		reads.Add(1)
		return nil
	}}
	router := newMigratingPool(home, func(_ context.Context, dc int) (telegram.CloseInvoker, error) {
		opens.Add(1)
		if dc != 4 {
			return nil, fmt.Errorf("unexpected DC %d", dc)
		}
		return target, nil
	}, unwrappedPool)
	var wg sync.WaitGroup
	for n := range 32 {
		wg.Go(func() {
			var output tg.UploadFileBox
			err := router.Invoke(context.Background(), &tg.UploadGetFileRequest{Offset: int64(n)}, &output)
			if err != nil {
				t.Errorf("read: %v", err)
				return
			}
			if got := output.File.(*tg.UploadFile).Bytes; len(got) != 1 || got[0] != byte(n) {
				t.Errorf("wrong chunk: %v", got)
			}
		})
	}
	wg.Wait()
	if opens.Load() != 1 || reads.Load() != 32 {
		t.Fatalf("opens=%d reads=%d", opens.Load(), reads.Load())
	}
	if err := router.Close(); err != nil {
		t.Fatal(err)
	}
	if err := router.Close(); err != nil {
		t.Fatal(err)
	}
	if home.closes.Load() != 1 || target.closes.Load() != 1 {
		t.Fatal("pools not closed exactly once")
	}
	if err := router.Invoke(context.Background(), nil, nil); !errors.Is(err, errMigratingPoolClosed) {
		t.Fatalf("after close: %v", err)
	}
}

func TestMigratingPoolDoesNotRerouteOtherErrors(t *testing.T) {
	for _, rpcErr := range []error{nil, tgerr.New(400, "FILE_REFERENCE_EXPIRED"), tgerr.New(303, "PHONE_MIGRATE_4"), tgerr.New(303, "FILE_MIGRATE_0")} {
		t.Run(fmt.Sprint(rpcErr), func(t *testing.T) {
			home := &testDCPool{InvokeFunc: func(context.Context, bin.Encoder, bin.Decoder) error { return rpcErr }}
			router := newMigratingPool(home, func(context.Context, int) (telegram.CloseInvoker, error) {
				t.Fatal("unexpected DC pool")
				return nil, nil
			}, unwrappedPool)
			defer router.Close()
			if err := router.Invoke(context.Background(), nil, nil); !errors.Is(err, rpcErr) {
				t.Fatalf("got %v want %v", err, rpcErr)
			}
		})
	}
}

func TestMigratingPoolFailedAuthorizationCanRetry(t *testing.T) {
	authErr := errors.New("export authorization failed")
	var opens int
	target := &testDCPool{InvokeFunc: func(context.Context, bin.Encoder, bin.Decoder) error { return nil }}
	router := newMigratingPool(fileRedirectPool(2), func(context.Context, int) (telegram.CloseInvoker, error) {
		opens++
		if opens == 1 {
			return nil, authErr
		}
		return target, nil
	}, unwrappedPool)
	defer router.Close()
	if err := router.Invoke(context.Background(), nil, nil); !errors.Is(err, authErr) {
		t.Fatalf("authorization error: %v", err)
	}
	if err := router.Invoke(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}
	if opens != 2 {
		t.Fatalf("opens=%d", opens)
	}
}

func TestMigratingPoolBoundsRedirects(t *testing.T) {
	target := fileRedirectPool(1)
	router := newMigratingPool(fileRedirectPool(4), func(context.Context, int) (telegram.CloseInvoker, error) { return target, nil }, unwrappedPool)
	defer router.Close()
	err := router.Invoke(context.Background(), nil, nil)
	if rpcErr, ok := tgerr.As(err); !ok || rpcErr.Argument != 1 {
		t.Fatalf("target redirect: %v", err)
	}
}

func TestMigratingPoolCancelledWaiterAndCloseDuringOpen(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	target := &testDCPool{InvokeFunc: func(context.Context, bin.Encoder, bin.Decoder) error { t.Error("invoked closed target"); return nil }}
	router := newMigratingPool(fileRedirectPool(4), func(context.Context, int) (telegram.CloseInvoker, error) {
		close(started)
		<-release
		return target, nil
	}, unwrappedPool)
	done := make(chan error, 1)
	go func() { done <- router.Invoke(context.Background(), nil, nil) }()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := router.Invoke(ctx, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter: %v", err)
	}
	if err := router.Close(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, errMigratingPoolClosed) {
		t.Fatalf("opener: %v", err)
	}
	if target.closes.Load() != 1 {
		t.Fatal("late pool leaked")
	}
}
