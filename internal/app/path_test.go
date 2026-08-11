package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func TestExpandHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := expandHomePath("~/cache")
	if err != nil {
		t.Fatalf("expandHomePath() error = %v", err)
	}
	if want := filepath.Join(home, "cache"); got != want {
		t.Fatalf("expandHomePath() = %q, want %q", got, want)
	}
	if got, err := expandHomePath("/var/cache/teldrive"); err != nil || got != "/var/cache/teldrive" {
		t.Fatalf("plain path = %q, %v", got, err)
	}
	if _, err := expandHomePath("~other/cache"); err == nil {
		t.Fatal("unsupported home-relative path was accepted")
	}
}

func TestRequestIDHeaderMiddlewareOwnsResponseHeader(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(requestIDHeader)
	router.Get("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("response request ID header is missing")
	}
}
