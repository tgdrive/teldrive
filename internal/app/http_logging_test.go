package app

import (
	"strings"
	"testing"
)

func TestHTTPQueriesDoNotLogPlaybackCredentials(t *testing.T) {
	query := redactedHTTPQuery("ticket=private-capability&start=10&token=private-token")
	if strings.Contains(query, "private-") || !strings.Contains(query, "start=10") {
		t.Fatal(query)
	}
}
