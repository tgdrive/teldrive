package api

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"testing"
)

func TestIsClientDisconnect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "canceled context", err: context.Canceled, want: true},
		{name: "connection reset", err: syscall.ECONNRESET, want: true},
		{name: "broken pipe", err: syscall.EPIPE, want: true},
		{name: "wrapped connection reset", err: fmt.Errorf("write tcp: %w", syscall.ECONNRESET), want: true},
		{name: "stream failure", err: errors.New("upstream read failed"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isClientDisconnect(tt.err); got != tt.want {
				t.Fatalf("isClientDisconnect(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
