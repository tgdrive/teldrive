package app

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func httpRequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/v1/") && !strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/health/") {
				next.ServeHTTP(w, r)
				return
			}
			started := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				status := ww.Status()
				level := slog.LevelInfo
				if status >= http.StatusInternalServerError {
					level = slog.LevelError
				} else if status >= http.StatusBadRequest {
					level = slog.LevelWarn
				}

				logger.LogAttrs(r.Context(), level, "http.request",
					slog.Int("status", status),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("query", redactedHTTPQuery(r.URL.RawQuery)),
					slog.String("ip", r.RemoteAddr),
					slog.String("user_agent", r.UserAgent()),
					slog.Duration("latency", time.Since(started)),
					slog.String("request_id", middleware.GetReqID(r.Context())),
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

func redactedHTTPQuery(raw string) string {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return "[invalid query]"
	}
	for _, key := range []string{"ticket", "token", "password", "key", "access_token"} {
		if values.Has(key) {
			values.Set(key, "[redacted]")
		}
	}
	return values.Encode()
}
