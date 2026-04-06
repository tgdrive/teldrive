package services

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContextReader(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsContextCauseWhenCancelledBeforeRead", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(errors.New("cancelled"))

		reader := newContextReader(ctx, nilReader{})

		_, err := reader.Read(make([]byte, 1))
		require.EqualError(t, err, "cancelled")
	})
}

type nilReader struct{}

func (nilReader) Read([]byte) (int, error) { return 0, io.EOF }
