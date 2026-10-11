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
			wrapped := middleware.RequestID(httpRequestLogger(logger, nil)(next))
			request := httptest.NewRequest(http.MethodGet, "/api/v1/test?q=value", nil)
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
			if attrs["status"] != int64(test.status) || attrs["path"] != "/api/v1/test" || attrs["query"] != "q=value" {
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
	wrapped := httpRequestLogger(logger, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	wrapped.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/files", nil))

	if len(handler.records) != 0 {
		t.Fatalf("records = %d, want 0", len(handler.records))
	}
}

func TestHTTPRequestLoggerResolvesClientIPThroughTrustedProxies(t *testing.T) {
	t.Parallel()

	security, err := newRequestSecurity([]string{"172.30.0.0/24", "10.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name       string
		remoteAddr string
		forwarded  []string
		want       string
	}{
		{name: "untrusted peer ignores header", remoteAddr: "203.0.113.9:4000", forwarded: []string{"198.51.100.1"}, want: "203.0.113.9"},
		{name: "trusted peer without header", remoteAddr: "172.30.0.11:50264", want: "172.30.0.11"},
		{name: "trusted peer uses forwarded client", remoteAddr: "172.30.0.11:50264", forwarded: []string{"213.35.116.148"}, want: "213.35.116.148"},
		{name: "spoofed leftmost entry ignored", remoteAddr: "172.30.0.11:50264", forwarded: []string{"1.2.3.4, 213.35.116.148"}, want: "213.35.116.148"},
		{name: "skips chained trusted hops across headers", remoteAddr: "172.30.0.11:50264", forwarded: []string{"1.2.3.4, 213.35.116.148", "10.0.0.1"}, want: "213.35.116.148"},
		{name: "malformed hop stops at last trusted", remoteAddr: "172.30.0.11:50264", forwarded: []string{"213.35.116.148, garbage"}, want: "172.30.0.11"},
		{name: "ipv4-mapped peer is trusted", remoteAddr: "[::ffff:172.30.0.11]:50264", forwarded: []string{"213.35.116.148"}, want: "213.35.116.148"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := &recordHandler{}
			wrapped := httpRequestLogger(slog.New(handler), security)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
			request := httptest.NewRequest(http.MethodGet, "/api/files", nil)
			request.RemoteAddr = test.remoteAddr
			for _, value := range test.forwarded {
				request.Header.Add("X-Forwarded-For", value)
			}

			wrapped.ServeHTTP(httptest.NewRecorder(), request)

			if len(handler.records) != 1 {
				t.Fatalf("records = %d, want 1", len(handler.records))
			}
			var ip any
			handler.records[0].Attrs(func(attr slog.Attr) bool {
				if attr.Key == "ip" {
					ip = attr.Value.Any()
				}
				return true
			})
			if ip != test.want {
				t.Fatalf("ip = %v, want %s", ip, test.want)
			}
		})
	}
}
