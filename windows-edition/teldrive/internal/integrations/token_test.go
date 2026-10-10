package integrations

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTokenExport(t *testing.T) {
	for _, tt := range []struct {
		name, method, origin, intent, cookie string
		invalid                              bool
		status                               int
	}{
		{"valid", "POST", "http://localhost:8080", "export-rclone", "session", false, 200},
		{"cross origin", "POST", "https://other.example", "export-rclone", "session", false, 403},
		{"no intent", "POST", "http://localhost:8080", "", "session", false, 403},
		{"no origin", "POST", "", "export-rclone", "session", false, 403},
		{"no cookie", "POST", "http://localhost:8080", "export-rclone", "", false, 401},
		{"revoked", "POST", "http://localhost:8080", "export-rclone", "session", true, 401},
		{"get forbidden", "GET", "http://localhost:8080", "export-rclone", "session", false, 405},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://localhost:8080/api/integrations/rclone/token", nil)
			req.Header.Set("Origin", tt.origin)
			req.Header.Set("X-Teldrive-Intent", tt.intent)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "access_token", Value: tt.cookie})
			}
			recorder := httptest.NewRecorder()
			TokenHandler(func(_ *http.Request, token string) (int64, error) {
				if tt.invalid || token != "session" {
					return 0, errors.New("invalid")
				}
				return 1, nil
			})(recorder, req)
			if recorder.Code != tt.status {
				t.Fatalf("status %d, want %d", recorder.Code, tt.status)
			}
			if recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("export must not be cached")
			}
			if tt.status == 200 && !strings.Contains(recorder.Body.String(), `"token":"session"`) {
				t.Fatal("missing token")
			}
		})
	}
}
