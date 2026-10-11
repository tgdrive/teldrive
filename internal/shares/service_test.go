package shares

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestServiceValidationAndTokenHash(t *testing.T) {
	t.Parallel()
	if _, err := NewService(nil, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("NewService(nil) error = %v", err)
	}
	s := &Service{now: time.Now}
	past := time.Now().Add(-time.Minute)
	zero := int64(0)
	for _, input := range []CreateInput{
		{},
		{OwnerID: 1, FileID: uuid.New(), ExpiresAt: &past},
		{OwnerID: 1, FileID: uuid.New(), MaxDownloads: &zero},
	} {
		if _, err := s.Create(context.Background(), input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("Create(%#v) error = %v", input, err)
		}
	}
	if _, err := s.List(context.Background(), ListInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("List() error = %v", err)
	}
	if err := s.Revoke(context.Background(), 0, uuid.Nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Revoke() error = %v", err)
	}
	if _, err := s.resolveRow(context.Background(), "", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolveRow(empty) error = %v", err)
	}
	if len(tokenHash("token")) != 32 {
		t.Fatal("tokenHash must be SHA-256")
	}
}
