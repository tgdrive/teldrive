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
		f, err := spaFS.Open(strings.TrimPrefix(path.Clean(r.URL.Path), "/"))
		if err == nil {
			defer f.Close()
		}
		if os.IsNotExist(err) {
			r.URL.Path = "/"
		}
		http.FileServer(http.FS(spaFS)).ServeHTTP(w, r)
	}
}
