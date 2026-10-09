package bots

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestServiceValidationHelpers(t *testing.T) {
	t.Parallel()
	if _, err := NewService(nil, nil, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("NewService() error = %v", err)
	}
	s := &Service{}
	if _, err := s.Create(context.Background(), 0, "token"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create(user=0) error = %v", err)
	}
	if _, err := s.Create(context.Background(), 1, " "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create(empty) error = %v", err)
	}
	if _, err := s.List(context.Background(), ListInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("List() error = %v", err)
	}
	if err := s.Delete(context.Background(), 0, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Delete() error = %v", err)
	}
	if nonEmpty(" ") != nil || nonEmpty(" bot ") == nil {
		t.Fatal("nonEmpty validation failed")
	}
	_ = uuid.Nil
}
