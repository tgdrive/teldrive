package telegramstore

import (
	"context"
	"errors"
	"testing"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

func TestClientRunnerValidation(t *testing.T) {
	t.Parallel()
	callback := func(context.Context, *tg.Client) error { return nil }
	if err := (ClientRunner{}).Run(context.Background(), 1, OperationUpload, callback); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("missing provider error = %v", err)
	}
	runner := ClientRunner{Provider: ClientProviderFunc(func(context.Context, int64, Operation) (*telegram.Client, error) {
		return nil, errors.New("session unavailable")
	})}
	if err := runner.Run(context.Background(), 1, OperationUpload, callback); err == nil {
		t.Fatal("expected provider error")
	}
	runner.Provider = ClientProviderFunc(func(context.Context, int64, Operation) (*telegram.Client, error) {
		return nil, nil
	})
	if err := runner.Run(context.Background(), 1, OperationDownload, callback); !errors.Is(err, ErrClientUnavailable) {
		t.Fatalf("nil client error = %v", err)
	}
	if err := runner.Run(context.Background(), 0, OperationUpload, callback); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid user error = %v", err)
	}
	if err := runner.Run(context.Background(), 1, OperationUpload, nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("nil callback error = %v", err)
	}
}

func TestRunClientOperation(t *testing.T) {
	callbackFailure := errors.New("callback failed")
	transportFailure := errors.New("transport failed")
	customCause := errors.New("part lease lost")
	for _, tc := range []struct {
		name        string
		cancelCause error
		callbackErr error
		runErr      error
		runCallback bool
		cancelChild bool
		want        error
	}{
		{name: "normal shutdown", runCallback: true},
		{name: "cancelled before ready", cancelCause: context.Canceled, want: context.Canceled},
		{name: "deadline before ready", cancelCause: context.DeadlineExceeded, want: context.DeadlineExceeded},
		{name: "custom cancellation cause", cancelCause: customCause, want: customCause},
		{name: "callback cancellation swallowed", runCallback: true, cancelChild: true, want: context.Canceled},
		{name: "callback error swallowed", runCallback: true, callbackErr: callbackFailure, want: callbackFailure},
		{name: "transport error", runErr: transportFailure, want: transportFailure},
		{name: "no callback and no error", want: ErrClientUnavailable},
		{name: "parent cancelled after success", runCallback: true, cancelCause: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			calls := 0
			run := func(ctx context.Context, fn func(context.Context) error) error {
				if tc.runCallback {
					child, stop := context.WithCancel(ctx)
					if tc.cancelChild {
						stop()
					}
					// Model gotd swallowing a callback cancellation and cancelling
					// its child context on successful shutdown as well.
					_ = fn(child)
					stop()
				}
				if tc.cancelCause != nil {
					cancel(tc.cancelCause)
				}
				return tc.runErr
			}
			err := RunClientOperation(ctx, run, func(ctx context.Context) error {
				calls++
				if err := ctx.Err(); err != nil {
					return err
				}
				return tc.callbackErr
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("RunClientOperation() = %v, want %v", err, tc.want)
			}
			wantCalls := 0
			if tc.runCallback {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("callback calls = %d, want %d", calls, wantCalls)
			}
		})
	}
}

func TestRunClientOperationAlreadyCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	run := func(context.Context, func(context.Context) error) error {
		t.Fatal("started client with cancelled context")
		return nil
	}
	if err := RunClientOperation(ctx, run, func(context.Context) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunClientOperation() = %v, want context.Canceled", err)
	}
}
