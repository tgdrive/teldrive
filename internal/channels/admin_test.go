package channels

import (
	"context"
	"errors"
	"testing"
)

func TestAdminValidation(t *testing.T) {
	t.Parallel()
	s := &Service{}
	if _, err := s.List(context.Background(), ListInput{}); !errors.Is(err, ErrInvalidOwner) {
		t.Fatalf("List() error = %v", err)
	}
	if _, err := s.Create(context.Background(), 0, "name", false); !errors.Is(err, ErrInvalidOwner) {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := s.Select(context.Background(), 0, 0); !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("Select() error = %v", err)
	}
	if err := s.Delete(context.Background(), 0, 0); !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("Delete() error = %v", err)
	}
}
