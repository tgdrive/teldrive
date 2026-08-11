package fileops

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestServiceValidationAndUUIDConversion(t *testing.T) {
	t.Parallel()
	if _, err := NewService(nil, nil, nil, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("NewService() error = %v", err)
	}
	s := &Service{}
	if _, err := s.Copy(context.Background(), CopyInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Copy() error = %v", err)
	}
	if err := s.Purge(context.Background(), 0, uuid.Nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Purge() error = %v", err)
	}
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	converted := pgUUIDs(ids)
	if len(converted) != len(ids) {
		t.Fatalf("pgUUIDs() length = %d", len(converted))
	}
	for index, id := range ids {
		if !converted[index].Valid || uuid.UUID(converted[index].Bytes) != id {
			t.Fatalf("pgUUIDs()[%d] = %#v", index, converted[index])
		}
	}
}
