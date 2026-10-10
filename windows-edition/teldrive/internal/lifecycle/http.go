package lifecycle

import (
	"encoding/json"
	"errors"
	"net/http"

	"gorm.io/gorm"
)

type Authorize func(*http.Request) (int64, error)

func Handler(db *gorm.DB, authorize Authorize) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user, err := authorize(r)
		if err != nil || user == 0 {
			http.Error(w, "Sesión no válida", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodGet {
			items, err := List(r.Context(), db, user, r.URL.Query().Get("state"))
			if err != nil {
				http.Error(w, "No se pudo consultar esta carpeta", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(items); err != nil {
				return
			}
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// A custom header prevents cross-origin simple requests through cookies.
		if r.Header.Get("X-Teldrive-Intent") != "file-lifecycle" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "Origen inválido", http.StatusForbidden)
			return
		}
		var body struct {
			IDs    []string `json:"ids"`
			Action string   `json:"action"`
			State  string   `json:"state"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil {
			http.Error(w, "Solicitud no válida", http.StatusBadRequest)
			return
		}
		if err := Change(r.Context(), db, user, body.IDs, body.Action, body.State); err != nil {
			code := http.StatusConflict
			if errors.Is(err, ErrInvalid) {
				code = http.StatusBadRequest
			}
			if errors.Is(err, ErrNotFound) {
				code = http.StatusNotFound
			}
			http.Error(w, "No se pudo completar el cambio; actualiza la lista e inténtalo de nuevo", code)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
