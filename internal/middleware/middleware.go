package middleware

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/tgdrive/teldrive/internal/logging"
	"go.uber.org/zap"
)

type Middleware = func(http.Handler) http.Handler

func InjectLogger(lg *zap.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req := r.WithContext(logging.WithLogger(r.Context(), lg))
			next.ServeHTTP(w, req)
		})
	}
}

func SPAHandler(filesystem fs.FS) http.HandlerFunc {
	spaFS, err := fs.Sub(filesystem, "dist")
	if err != nil {
		logging.DefaultLogger().Fatal(err.Error())
	}
	return func(w http.ResponseWriter, r *http.Request) {
		filePath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		f, err := spaFS.Open(filePath)
		if err == nil {
			defer f.Close()
		}
		if os.IsNotExist(err) {
			r.URL.Path = "/"
			filePath = "index.html"
		}
		if filePath == "index.html" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}

		http.FileServer(http.FS(spaFS)).ServeHTTP(w, r)
	}
}
