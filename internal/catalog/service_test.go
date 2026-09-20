package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestServiceRejectsInvalidInputsBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()
	svc := NewService(nil, nil)
	ctx := context.Background()
	id := uuid.New()

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{name: "create owner", call: func() error { _, err := svc.CreateFolder(ctx, CreateFolderInput{Name: "x"}); return err }, want: ErrInvalidOwner},
		{name: "get owner", call: func() error { _, err := svc.Get(ctx, 0, id); return err }, want: ErrInvalidOwner},
		{name: "list owner", call: func() error { _, err := svc.List(ctx, ListInput{}); return err }, want: ErrInvalidOwner},
		{name: "rename owner", call: func() error { _, err := svc.Rename(ctx, 0, id, nil, "x"); return err }, want: ErrInvalidOwner},
		{name: "move owner", call: func() error { _, err := svc.Move(ctx, 0, id, nil, nil); return err }, want: ErrInvalidOwner},
		{name: "trash owner", call: func() error { _, err := svc.Trash(ctx, 0, id); return err }, want: ErrInvalidOwner},
		{name: "restore owner", call: func() error { _, err := svc.Restore(ctx, 0, id); return err }, want: ErrInvalidOwner},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}
