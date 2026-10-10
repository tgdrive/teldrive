package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
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

func TestRequestIDMiddlewareGeneratesAndPreservesRequestID(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	router.Use(requestIDMiddleware)
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		if middleware.GetReqID(r.Context()) == "" {
			t.Fatal("request ID missing from context")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	generated := response.Header().Get("X-Request-ID")
	if _, err := uuid.Parse(generated); err != nil {
		t.Fatalf("generated request ID = %q, want UUID: %v", generated, err)
	}

	const incoming = "upstream-request-id"
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Request-ID", incoming)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if got := response.Header().Get("X-Request-ID"); got != incoming {
		t.Fatalf("response request ID = %q, want %q", got, incoming)
	}
}
