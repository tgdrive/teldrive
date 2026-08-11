package api

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBrowserCookieSecureFromContext(t *testing.T) {
	t.Parallel()
	handler := &Handler{}
	insecure := handler.browserCookie(context.Background(), "name", "value", time.Minute).String()
	if strings.Contains(insecure, "Secure") {
		t.Fatalf("insecure cookie = %q", insecure)
	}
	secure := handler.browserCookie(WithBrowserCookieSecure(context.Background(), true), "name", "value", time.Minute).String()
	if !strings.Contains(secure, "Secure") {
		t.Fatalf("secure cookie = %q", secure)
	}
}
