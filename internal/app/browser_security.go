package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/tgdrive/teldrive/v2/internal/api"
	"github.com/tgdrive/teldrive/v2/internal/authn"
)

const (
	browserAccessCookieName  = "teldrive_access"
	browserRefreshCookieName = "teldrive_refresh"
)

const (
	browserAccessRefreshSkew = 10 * time.Second
	browserRefreshTimeout    = 5 * time.Second
)

type browserSessionRefresher interface {
	RenewAccess(context.Context, string) (*authn.AccessRenewal, error)
}

type browserSessionRenewer struct {
	auth browserSessionRefresher
	now  func() time.Time
}

func browserSessionRenewalMiddleware(auth browserSessionRefresher, next http.Handler) http.Handler {
	if auth == nil {
		return next
	}
	return (&browserSessionRenewer{auth: auth, now: time.Now}).middleware(next)
}

func (m *browserSessionRenewer) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if browserSessionRenewalSkipped(r) {
			next.ServeHTTP(w, r)
			return
		}
		refreshCookie, err := r.Cookie(browserRefreshCookieName)
		if err != nil || strings.TrimSpace(refreshCookie.Value) == "" {
			next.ServeHTTP(w, r)
			return
		}
		accessCookie, accessErr := r.Cookie(browserAccessCookieName)
		if accessErr == nil && !accessCookieNeedsRefresh(accessCookie.Value, m.now().UTC()) {
			next.ServeHTTP(w, r)
			return
		}

		renewal, err := m.renew(r.Context(), refreshCookie.Value)
		if err != nil {
			if errors.Is(err, authn.ErrInvalidCredential) || errors.Is(err, authn.ErrSessionNotFound) {
				clearBrowserSessionCookies(w, r)
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    "session_refresh_unavailable",
					"message": "The browser session could not be renewed.",
				},
			})
			return
		}

		accessTTL := time.Duration(renewal.ExpiresIn) * time.Second
		access := browserSessionCookie(r, browserAccessCookieName, renewal.AccessToken, accessTTL)
		http.SetCookie(w, access)
		replaceRequestCookie(r, access)
		next.ServeHTTP(w, r)
	})
}

func browserSessionRenewalSkipped(r *http.Request) bool {
	if r == nil || hasExplicitAPICredential(r) || !strings.HasPrefix(r.URL.Path, "/v1/") {
		return true
	}
	if !strings.HasPrefix(r.URL.Path, "/v1/auth/") {
		return false
	}
	return r.URL.Path != "/v1/auth/logout" && r.URL.Path != "/v1/auth/browser/logout"
}

func (m *browserSessionRenewer) renew(ctx context.Context, refreshToken string) (*authn.AccessRenewal, error) {
	refreshCtx, cancel := context.WithTimeout(ctx, browserRefreshTimeout)
	defer cancel()
	return m.auth.RenewAccess(refreshCtx, refreshToken)
}

func accessCookieNeedsRefresh(raw string, now time.Time) bool {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 3 {
		return true
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return true
	}
	var claims struct {
		ExpiresAt int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.ExpiresAt <= 0 {
		return true
	}
	return !time.Unix(claims.ExpiresAt, 0).After(now.Add(browserAccessRefreshSkew))
}

func browserSessionCookie(r *http.Request, name, value string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(ttl / time.Second),
		Expires:  time.Now().UTC().Add(ttl),
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteLaxMode,
	}
}

func clearBrowserSessionCookies(w http.ResponseWriter, r *http.Request) {
	for _, name := range []string{browserAccessCookieName, browserRefreshCookieName} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Path:     "/",
			MaxAge:   -1,
			Expires:  time.Unix(1, 0).UTC(),
			HttpOnly: true,
			Secure:   requestIsSecure(r),
			SameSite: http.SameSiteLaxMode,
		})
	}
}

func replaceRequestCookie(r *http.Request, replacement *http.Cookie) {
	cookies := r.Cookies()
	r.Header.Del("Cookie")
	replaced := false
	for _, cookie := range cookies {
		if cookie.Name == replacement.Name {
			if replaced {
				continue
			}
			cookie.Value = replacement.Value
			replaced = true
		}
		r.AddCookie(cookie)
	}
	if !replaced {
		r.AddCookie(&http.Cookie{Name: replacement.Name, Value: replacement.Value})
	}
}

type requestSecurity struct {
	trustedProxies []netip.Prefix
}

func newRequestSecurity(values []string) (*requestSecurity, error) {
	security := &requestSecurity{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			address, addressErr := netip.ParseAddr(value)
			if addressErr != nil {
				return nil, err
			}
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		security.trustedProxies = append(security.trustedProxies, prefix)
	}
	return security, nil
}

func (s *requestSecurity) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secure := r.TLS != nil || (s.isTrustedProxy(r) && forwardedProto(r) == "https")
		ctx := context.WithValue(r.Context(), requestSecureContextKey{}, secure)
		ctx = api.WithBrowserCookieSecure(ctx, secure)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type requestSecureContextKey struct{}

func requestIsSecure(r *http.Request) bool {
	secure, _ := r.Context().Value(requestSecureContextKey{}).(bool)
	return secure
}

func (s *requestSecurity) isTrustedProxy(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	address, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return false
	}
	for _, prefix := range s.trustedProxies {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func forwardedProto(r *http.Request) string {
	value, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ",")
	return strings.ToLower(strings.TrimSpace(value))
}

func browserCSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSafeMethod(r.Method) || !hasBrowserSessionCookie(r) || hasExplicitAPICredential(r) || isSameOriginBrowserRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "csrf_rejected",
				"message": "The browser request did not originate from this Teldrive server.",
			},
		})
	})
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func hasBrowserSessionCookie(r *http.Request) bool {
	if r == nil {
		return false
	}
	if cookie, err := r.Cookie(browserAccessCookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
		return true
	}
	if cookie, err := r.Cookie(browserRefreshCookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
		return true
	}
	return false
}

func hasExplicitAPICredential(r *http.Request) bool {
	if r == nil {
		return false
	}
	return strings.TrimSpace(r.Header.Get("Authorization")) != "" || strings.TrimSpace(r.Header.Get("X-API-Key")) != ""
}

func isSameOriginBrowserRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))) {
	case "same-origin", "same-site", "none":
		return true
	case "cross-site":
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Host, r.Host) {
		return false
	}
	expectedScheme := "http"
	if requestIsSecure(r) {
		expectedScheme = "https"
	}
	return strings.EqualFold(parsed.Scheme, expectedScheme)
}
