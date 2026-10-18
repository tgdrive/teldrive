package api

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestShareDownloadCookie(t *testing.T) {
	t.Parallel()
	h := &Handler{}
	expiry := time.Now().UTC().Add(15 * time.Minute)
	cookie := h.shareDownloadCookie(WithCookieSecure(context.Background(), true), "tds_example", "opaque-secret", expiry)
	if cookie.Name != shareDownloadCookieName || cookie.Value != "opaque-secret" || cookie.Path != "/api/v1/public/shares/tds_example/" {
		t.Fatalf("cookie scope = %#v", cookie)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Domain != "" {
		t.Fatalf("cookie security = %#v", cookie)
	}
	if !cookie.Expires.Equal(expiry) || cookie.MaxAge < 899 || cookie.MaxAge > 900 {
		t.Fatalf("cookie expiry = %#v", cookie)
	}
	// Match the existing browser auth behavior for explicitly insecure local HTTP.
	if h.shareDownloadCookie(context.Background(), "tds_example", "secret", expiry).Secure {
		t.Fatal("local HTTP cookie should follow the configured transport security")
	}
}
