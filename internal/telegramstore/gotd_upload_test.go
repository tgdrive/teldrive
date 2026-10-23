package telegramstore

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gotd/td/tg"
)

type interruptedUploadRunner struct {
	cancel context.CancelCauseFunc
	cause  error
}

func (r interruptedUploadRunner) Run(context.Context, int64, Operation, func(context.Context, *tg.Client) error) error {
	if r.cause != nil {
		r.cancel(r.cause)
	}
	// Model a runner that returns nil without completing the upload callback.
	return nil
}

func TestGotdUploadWithoutCompletedCallback(t *testing.T) {
	for _, cause := range []error{nil, context.Canceled, context.DeadlineExceeded} {
		name := "no callback"
		if cause != nil {
			name = cause.Error()
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			storage := NewGotdStorage(interruptedUploadRunner{cancel: cancel, cause: cause}, nil)
			part, err := storage.Upload(ctx, UploadRequest{
				UserID: 1, ChannelID: 2, Name: "part", Size: 4, Reader: strings.NewReader("data"),
			})
			want := cause
			if want == nil {
				want = ErrClientUnavailable
			}
			if !errors.Is(err, want) || errors.Is(err, ErrMessageNotFound) {
				t.Fatalf("Upload() = %v, want %v, not ErrMessageNotFound", err, want)
			}
			if part != (StoredPart{}) {
				t.Fatalf("Upload() returned a stored part on failure: %+v", part)
			}
		})
	}
}
