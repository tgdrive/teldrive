package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
)

func TestPublicFileContentRequestScope(t *testing.T) {
	const id = "123e4567-e89b-12d3-a456-426614174000"
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodGet, "/v1/files/" + id + "/content", true},
		{http.MethodHead, "/v1/files/" + id + "/content/video.mp4", true},
		{http.MethodGet, "/v1/files/" + id + "/content/video.mp4?download=1", true},
		{http.MethodGet, "/v1/files/" + id + "/content/", false},
		{http.MethodGet, "/v1/files/" + id + "/content/video.mp4/extra", false},
		{http.MethodGet, "/v1/files/invalid/content", false},
		{http.MethodGet, "/v1/public/shares/token/files/" + id + "/content", false},
		{http.MethodGet, "/v1/files/" + id, false},
		{http.MethodGet, "/v1/files/" + id + "/playback/statistics", false},
		{http.MethodPost, "/v1/files/" + id + "/content", false},
	}
	for _, test := range tests {
		r := httptest.NewRequest(test.method, test.path, nil)
		if got := isFileContentRequest(r); got != test.want {
			t.Errorf("%s %s: got %v, want %v", test.method, test.path, got, test.want)
		}
	}
}

func TestPublicFileContentSecurityRequiresInternalMarker(t *testing.T) {
	security := NewSecurity(nil).WithPublicFileContent(true)
	marked := context.WithValue(context.Background(), publicFileContentContextKey{}, true)
	for _, operation := range []gen.OperationName{
		gen.DownloadFileOperation,
		gen.DownloadFileLegacyOperation,
		gen.HeadFileOperation,
		gen.HeadFileLegacyOperation,
	} {
		_, err := security.HandleBearerAuth(marked, operation, gen.BearerAuth{Token: publicFileContentToken})
		if err != nil {
			t.Errorf("marked %s: %v", operation, err)
		}
		_, err = security.HandleBearerAuth(context.Background(), operation, gen.BearerAuth{Token: publicFileContentToken})
		if !errors.Is(err, ErrUnauthenticated) {
			t.Errorf("unmarked %s: got %v", operation, err)
		}
	}
	_, err := security.HandleBearerAuth(marked, gen.GetCurrentUserOperation, gen.BearerAuth{Token: publicFileContentToken})
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("unrelated operation: got %v", err)
	}
	security.WithPublicFileContent(false)
	_, err = security.HandleBearerAuth(marked, gen.DownloadFileOperation, gen.BearerAuth{Token: publicFileContentToken})
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("disabled: got %v", err)
	}
}

func TestPublicFileContentRoutingDefaultAndOptIn(t *testing.T) {
	const id = "123e4567-e89b-12d3-a456-426614174000"
	for _, enabled := range []bool{false, true} {
		security := NewSecurity(nil).WithPublicFileContent(enabled)
		server, err := NewServer(NewHandler(nil, nil, nil, nil, nil, 0, nil), security)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{
			"/v1/files/" + id + "/content",
			"/v1/files/" + id + "/content/video.mp4",
			"/v1/files/" + id + "/content/video.mp4?download=1",
		} {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				r := httptest.NewRequest(method, path, nil)
				r.Header.Set("Authorization", "Bearer invalid")
				w := httptest.NewRecorder()
				server.ServeHTTP(w, r)
				want := http.StatusUnauthorized
				if enabled {
					want = http.StatusServiceUnavailable // Reached handler; services not configured.
				}
				if w.Code != want {
					t.Errorf("enabled=%v %s %s: status %d, want %d: %s", enabled, method, path, w.Code, want, w.Body.String())
				}
			}
		}
		r := httptest.NewRequest(http.MethodGet, "/v1/files/"+id, nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("enabled=%v unrelated route: status %d", enabled, w.Code)
		}
	}
}
