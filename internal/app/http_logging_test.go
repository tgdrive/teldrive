package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

type recordHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, record.Clone())
	return nil
}
func (h *recordHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordHandler) WithGroup(string) slog.Handler      { return h }

func TestHTTPRequestLoggerLevelsAndAttributes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		status int
		level  slog.Level
	}{
		{name: "success", status: http.StatusOK, level: slog.LevelInfo},
		{name: "client error", status: http.StatusBadRequest, level: slog.LevelWarn},
		{name: "server error", status: http.StatusInternalServerError, level: slog.LevelError},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := &recordHandler{}
			logger := slog.New(handler)
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
			})
			wrapped := middleware.RequestID(httpRequestLogger(logger)(next))
			request := httptest.NewRequest(http.MethodGet, "/v1/test?q=value", nil)
			request.RemoteAddr = "127.0.0.1:1234"
			request.Header.Set("User-Agent", "test-agent")
			response := httptest.NewRecorder()

			wrapped.ServeHTTP(response, request)

			if len(handler.records) != 1 {
				t.Fatalf("records = %d, want 1", len(handler.records))
			}
			record := handler.records[0]
			if record.Level != test.level || record.Message != "http.request" {
				t.Fatalf("record = (%s, %q), want (%s, %q)", record.Level, record.Message, test.level, "http.request")
			}
			attrs := map[string]any{}
			record.Attrs(func(attr slog.Attr) bool {
				attrs[attr.Key] = attr.Value.Any()
				return true
			})
			if attrs["status"] != int64(test.status) || attrs["path"] != "/v1/test" || attrs["query"] != "q=value" {
				t.Fatalf("attrs = %#v", attrs)
			}
			if attrs["request_id"] == "" {
				t.Fatalf("request_id is empty: %#v", attrs)
			}
		})
	}
}


func TestHTTPRequestLoggerSkipsUIRequests(t *testing.T) {
	t.Parallel()

	handler := &recordHandler{}
	logger := slog.New(handler)
	wrapped := httpRequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	wrapped.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/files", nil))

	if len(handler.records) != 0 {
		t.Fatalf("records = %d, want 0", len(handler.records))
	}
}
