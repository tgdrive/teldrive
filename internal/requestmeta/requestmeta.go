package requestmeta

import (
	"context"
	"net/http"
	"strings"
	"sync"
)

type contextKey struct{}

type state struct {
	secure  bool
	cookies []string
	mu      sync.Mutex
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st := &state{secure: isSecureRequest(r)}
		ctx := context.WithValue(r.Context(), contextKey{}, st)
		next.ServeHTTP(&responseWriter{ResponseWriter: w, state: st}, r.WithContext(ctx))
	})
}

func IsSecure(ctx context.Context) bool {
	st, ok := fromContext(ctx)
	if !ok {
		return false
	}
	return st.secure
}

func AddSetCookie(ctx context.Context, cookie string) {
	if cookie == "" {
		return
	}
	st, ok := fromContext(ctx)
	if !ok {
		return
	}
	st.mu.Lock()
	st.cookies = append(st.cookies, cookie)
	st.mu.Unlock()
}

func fromContext(ctx context.Context) (*state, bool) {
	if ctx == nil {
		return nil, false
	}
	st, ok := ctx.Value(contextKey{}).(*state)
	return st, ok
}

func isSecureRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

type responseWriter struct {
	http.ResponseWriter
	state   *state
	flushed bool
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.flushCookies()
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriter) Write(p []byte) (int, error) {
	w.flushCookies()
	return w.ResponseWriter.Write(p)
}

func (w *responseWriter) Flush() {
	w.flushCookies()
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *responseWriter) flushCookies() {
	if w.flushed || w.state == nil {
		return
	}
	w.state.mu.Lock()
	defer w.state.mu.Unlock()
	for _, cookie := range w.state.cookies {
		w.ResponseWriter.Header().Add("Set-Cookie", cookie)
	}
	w.flushed = true
	w.state.cookies = nil
}
