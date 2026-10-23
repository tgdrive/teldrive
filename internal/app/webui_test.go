package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestWebUIHandlerServesAssetsAndHistoryFallback(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<main>Teldrive UI</main>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app-deadbeef.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := webUIHandler{files: os.DirFS(root)}

	for _, test := range []struct {
		path        string
		status      int
		body        string
		cachePrefix string
	}{
		{path: "/", status: http.StatusOK, body: "Teldrive UI", cachePrefix: "no-cache"},
		{path: "/files/deep/folder", status: http.StatusOK, body: "Teldrive UI", cachePrefix: "no-cache"},
		{path: "/assets/app-deadbeef.js", status: http.StatusOK, body: "console.log", cachePrefix: "public, max-age=31536000"},
		{path: "/assets/missing.js", status: http.StatusNotFound, body: "404 page not found"},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			result := response.Result()
			defer result.Body.Close()
			content, _ := io.ReadAll(result.Body)
			if result.StatusCode != test.status || !strings.Contains(string(content), test.body) {
				t.Fatalf("GET %s = %d %q", test.path, result.StatusCode, content)
			}
			if test.cachePrefix != "" && !strings.HasPrefix(result.Header.Get("Cache-Control"), test.cachePrefix) {
				t.Fatalf("GET %s cache control = %q", test.path, result.Header.Get("Cache-Control"))
			}
			if test.status == http.StatusOK && !strings.Contains(result.Header.Get("Content-Security-Policy"), "script-src 'self'") {
				t.Fatalf("GET %s CSP = %q", test.path, result.Header.Get("Content-Security-Policy"))
			}
			if test.status == http.StatusOK {
				csp := result.Header.Get("Content-Security-Policy")
				// WebAssembly may be compiled (the player's audio decoder), but string eval may not.
				if !strings.Contains(csp, "'wasm-unsafe-eval'") {
					t.Fatalf("GET %s CSP lacks 'wasm-unsafe-eval': %q", test.path, csp)
				}
				if strings.Contains(strings.ReplaceAll(csp, "'wasm-unsafe-eval'", ""), "'unsafe-eval'") {
					t.Fatalf("GET %s CSP allows 'unsafe-eval': %q", test.path, csp)
				}
			}
		})
	}
}

func TestWebUIHandlerUsesEmbeddedSPA(t *testing.T) {
	t.Parallel()
	if handler, err := newWebUIHandler(); err != nil || handler == nil {
		t.Fatalf("embedded UI = %T, %v", handler, err)
	}
}

func TestRouteApplicationKeepsAPIAndSPASeparate(t *testing.T) {
	t.Parallel()
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "api:"+r.URL.Path)
	})
	ui := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ui:"+r.URL.Path)
	})
	mux := chi.NewRouter()
	routeApplication(mux, api, ui)

	tests := map[string]string{
		"/api/v1/files": "api:/v1/files",
		"/v1/files":     "api:/v1/files",
		"/health/live":  "api:/health/live",
		"/files":        "ui:/files",
	}
	for requestPath, want := range tests {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestPath, nil))
		if got := strings.TrimSpace(response.Body.String()); got != want {
			t.Fatalf("GET %s = %q, want %q", requestPath, got, want)
		}
	}
}
