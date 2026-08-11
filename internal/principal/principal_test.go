package principal

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestContextRoundTrip(t *testing.T) {
	t.Parallel()
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("unexpected identity in empty context")
	}
	identity := Identity{UserID: 1001, SessionID: uuid.New(), Roles: []string{"user"}, Source: "bearer"}
	got, ok := FromContext(WithIdentity(context.Background(), identity))
	if !ok || got.UserID != identity.UserID || got.SessionID != identity.SessionID || got.Source != identity.Source {
		t.Fatalf("identity = %#v, ok=%v", got, ok)
	}
}
