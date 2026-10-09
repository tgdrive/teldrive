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
			http.Error(w, "Sesión no válida", 401)
			return
		}
		if r.Method == "GET" {
			items, err := List(r.Context(), db, user, r.URL.Query().Get("state"))
			if err != nil {
				http.Error(w, "No se pudo consultar esta carpeta", 400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(items); err != nil {
				return
			}
			return
		}
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		// A custom header prevents cross-origin simple requests through cookies.
		if r.Header.Get("X-Teldrive-Intent") != "file-lifecycle" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "Origen inválido", 403)
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
			http.Error(w, "Solicitud no válida", 400)
			return
		}
		if err := Change(r.Context(), db, user, body.IDs, body.Action, body.State); err != nil {
			code := 409
			if errors.Is(err, ErrInvalid) {
				code = 400
			}
			if errors.Is(err, ErrNotFound) {
				code = 404
			}
			http.Error(w, "No se pudo completar el cambio; actualiza la lista e inténtalo de nuevo", code)
			return
		}
		w.WriteHeader(204)
	}
}
