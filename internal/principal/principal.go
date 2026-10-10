package principal

import (
	"context"

	"github.com/google/uuid"
)

type Identity struct {
	UserID    int64
	SessionID uuid.UUID
	Roles     []string
	Source    string
}

type contextKey struct{}

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, contextKey{}, identity)
}

func FromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(contextKey{}).(Identity)
	return identity, ok && identity.UserID > 0
}
