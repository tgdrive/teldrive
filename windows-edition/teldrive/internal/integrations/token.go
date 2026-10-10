// Package integrations provides explicit session export for desktop clients.
package integrations

import (
	"encoding/json"
	"net/http"
	"net/url"
)

type Verify func(*http.Request, string) (int64, error)

// TokenHandler only exports a verified session after a same-origin UI request.
func TokenHandler(verify Verify) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		origin, err := url.Parse(r.Header.Get("Origin"))
		if err != nil || origin.Host != r.Host || (origin.Scheme != "http" && origin.Scheme != "https") || r.Header.Get("X-Teldrive-Intent") != "export-rclone" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "Solicitud de origen inválido", http.StatusForbidden)
			return
		}
		cookie, err := r.Cookie("access_token")
		if err != nil {
			http.Error(w, "Inicia sesión para conectar rclone", http.StatusUnauthorized)
			return
		}
		userID, err := verify(r, cookie.Value)
		if err != nil || userID == 0 {
			http.Error(w, "La sesión ya no es válida", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"token": cookie.Value}); err != nil {
			return
		}
	}
}
