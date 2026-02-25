package integration_test

import (
	"context"
	"testing"
)

func TestEventsAndVersionRoutes_Basic(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	_, client, _ := loginWithClient(t, s, 7207, "user7207")

	if _, err := client.VersionVersion(ctx); err != nil {
		t.Fatalf("VersionVersion failed: %v", err)
	}
	if _, err := client.EventsGetEvents(ctx); err != nil {
		t.Fatalf("EventsGetEvents failed: %v", err)
	}
}
