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
