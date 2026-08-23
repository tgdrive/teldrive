package app

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tgdrive/teldrive/v2/internal/authn"
)

func TestRequestSecurity(t *testing.T) {
	t.Parallel()
	security, err := newRequestSecurity([]string{"10.0.0.0/8", "192.0.2.10"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		tls        bool
		wantSecure bool
	}{
		{name: "direct TLS", remoteAddr: "203.0.113.1:1234", tls: true, wantSecure: true},
		{name: "trusted proxy HTTPS", remoteAddr: "10.1.2.3:1234", forwarded: "https", wantSecure: true},
		{name: "trusted address HTTPS", remoteAddr: "192.0.2.10:1234", forwarded: "https", wantSecure: true},
		{name: "untrusted forwarded header", remoteAddr: "203.0.113.1:1234", forwarded: "https"},
		{name: "trusted proxy HTTP", remoteAddr: "10.1.2.3:1234", forwarded: "http"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got bool
			handler := security.middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = requestIsSecure(r)
			}))
			request := httptest.NewRequest(http.MethodGet, "http://drive.example.test", nil)
			request.RemoteAddr = test.remoteAddr
			request.Header.Set("X-Forwarded-Proto", test.forwarded)
			if test.tls {
				request.TLS = &tls.ConnectionState{}
			}
			handler.ServeHTTP(httptest.NewRecorder(), request)
			if got != test.wantSecure {
				t.Fatalf("secure = %t, want %t", got, test.wantSecure)
			}
		})
	}
	if _, err := newRequestSecurity([]string{"not-an-address"}); err == nil {
		t.Fatal("invalid trusted proxy was accepted")
	}
	if _, err := newRequestSecurity(nil); err != nil {
		t.Fatalf("empty trusted proxies: %v", err)
	}
	for _, value := range []string{"https", "HTTPS, http"} {
		request := httptest.NewRequest(http.MethodGet, "http://drive.example.test", nil)
		request.Header.Set("X-Forwarded-Proto", value)
		if forwardedProto(request) != "https" {
			t.Fatalf("forwardedProto(%q) = %q", value, forwardedProto(request))
		}
	}
}

func TestBrowserCSRFMiddleware(t *testing.T) {
	t.Parallel()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := browserCSRFMiddleware(next)

	tests := []struct {
		name       string
		method     string
		cookie     bool
		headers    map[string]string
		wantStatus int
	}{
		{name: "safe method with cookie", method: http.MethodGet, cookie: true, wantStatus: http.StatusNoContent},
		{name: "API key client bypass", method: http.MethodPost, cookie: true, headers: map[string]string{"X-API-Key": "tdk_test", "Sec-Fetch-Site": "cross-site"}, wantStatus: http.StatusNoContent},
		{name: "bearer client bypass", method: http.MethodDelete, cookie: true, headers: map[string]string{"Authorization": "Bearer token", "Sec-Fetch-Site": "cross-site"}, wantStatus: http.StatusNoContent},
		{name: "no browser cookie", method: http.MethodPost, headers: map[string]string{"Sec-Fetch-Site": "cross-site"}, wantStatus: http.StatusNoContent},
		{name: "same origin fetch metadata", method: http.MethodPost, cookie: true, headers: map[string]string{"Sec-Fetch-Site": "same-origin"}, wantStatus: http.StatusNoContent},
		{name: "same site fetch metadata", method: http.MethodPost, cookie: true, headers: map[string]string{"Sec-Fetch-Site": "same-site"}, wantStatus: http.StatusNoContent},
		{name: "matching origin fallback", method: http.MethodPost, cookie: true, headers: map[string]string{"Origin": "http://drive.example.test"}, wantStatus: http.StatusNoContent},
		{name: "cross site rejected", method: http.MethodPost, cookie: true, headers: map[string]string{"Sec-Fetch-Site": "cross-site"}, wantStatus: http.StatusForbidden},
		{name: "missing origin rejected", method: http.MethodPost, cookie: true, wantStatus: http.StatusForbidden},
		{name: "mismatched origin rejected", method: http.MethodPatch, cookie: true, headers: map[string]string{"Origin": "https://evil.example"}, wantStatus: http.StatusForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "http://drive.example.test/v1/files", nil)
			if test.cookie {
				request.AddCookie(&http.Cookie{Name: accessCookieName, Value: "access"})
			}
			for name, value := range test.headers {
				request.Header.Set(name, value)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.wantStatus, response.Body.String())
			}
			if test.wantStatus == http.StatusForbidden {
				if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
					t.Fatalf("content type = %q", contentType)
				}
				if !strings.Contains(response.Body.String(), "csrf_rejected") {
					t.Fatalf("body = %s", response.Body.String())
				}
			}
		})
	}
}

type renewalStub struct {
	calls   atomic.Int32
	renewal authn.AccessRenewal
	err     error
}

func (s *renewalStub) RenewAccess(_ context.Context, _ string) (*authn.AccessRenewal, error) {
	s.calls.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	renewal := s.renewal
	return &renewal, nil
}

func TestSessionRenewalMiddleware(t *testing.T) {
	now := time.Now().UTC()
	if accessCookieNeedsRefresh(testAccessJWT(now.Add(time.Hour)), now) {
		t.Fatal("fresh access cookie requested renewal")
	}
	if !accessCookieNeedsRefresh(testAccessJWT(now.Add(-time.Minute)), now) {
		t.Fatal("expired access cookie did not request renewal")
	}
	for _, path := range []string{"/health/live", "/v1/auth/refresh", "/v1/auth/cookie/refresh", "/v1/auth/telegram/start"} {
		if !sessionRenewalSkipped(httptest.NewRequest(http.MethodGet, "http://drive.example.test"+path, nil)) {
			t.Fatalf("renewal should skip %s", path)
		}
	}
	for _, path := range []string{"/v1/me", "/v1/files", "/v1/auth/logout", "/v1/auth/cookie/logout"} {
		if sessionRenewalSkipped(httptest.NewRequest(http.MethodGet, "http://drive.example.test"+path, nil)) {
			t.Fatalf("renewal should allow %s", path)
		}
	}
	stub := &renewalStub{renewal: authn.AccessRenewal{AccessToken: "a2", ExpiresIn: 900}}
	handler := sessionRenewalMiddleware(stub, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		access, err := r.Cookie(accessCookieName)
		if err != nil || access.Value != "a2" {
			t.Fatalf("renewed access cookie = %#v, %v", access, err)
		}
		refresh, err := r.Cookie(refreshCookieName)
		if err != nil || refresh.Value != "r1" {
			t.Fatalf("refresh cookie changed = %#v, %v", refresh, err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	for range 2 {
		request := httptest.NewRequest(http.MethodGet, "http://drive.example.test/v1/files", nil)
		request.AddCookie(&http.Cookie{Name: accessCookieName, Value: testAccessJWT(now.Add(-time.Minute))})
		request.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "r1"})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d", response.Code)
		}
		cookies := response.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Name != accessCookieName || cookies[0].Value != "a2" {
			t.Fatalf("renewal cookies = %#v", cookies)
		}
	}
	if stub.calls.Load() != 2 {
		t.Fatalf("renewal calls = %d, want 2", stub.calls.Load())
	}
}

func testAccessJWT(expiresAt time.Time) string {
	payload, _ := json.Marshal(map[string]int64{"exp": expiresAt.Unix()})
	return "h." + base64.RawURLEncoding.EncodeToString(payload) + ".s"
}
