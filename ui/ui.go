package ui

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
)

//go:embed all:teldrive-ui/dist
var staticFS embed.FS

func AddRoutes(router gin.IRouter) {
	embeddedBuildFolder := newStaticFileSystem()
	fallbackFileSystem := newFallbackFileSystem(embeddedBuildFolder)

	router.Use(func(c *gin.Context) {
		isStatic, _ := path.Match("/assets/*", c.Request.URL.Path)
		isImg, _ := path.Match("/img/*", c.Request.URL.Path)
		if isStatic || isImg {
			c.Writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			gzip.Gzip(gzip.DefaultCompression)(c)
		} else {
			c.Writer.Header().Set("Cache-Control", "public, max-age=0, s-maxage=0, must-revalidate")
		}
		c.Next()
	})
	router.Use(static.Serve("/", embeddedBuildFolder))
	router.Use(static.Serve("/", fallbackFileSystem))
}

type staticFileSystem struct {
	http.FileSystem
}

var _ static.ServeFileSystem = (*staticFileSystem)(nil)

func newStaticFileSystem() *staticFileSystem {
	sub, err := fs.Sub(staticFS, "teldrive-ui/dist")

	if err != nil {
		panic(err)
	}

	return &staticFileSystem{
		FileSystem: http.FS(sub),
	}
}

func (s *staticFileSystem) Exists(prefix string, path string) bool {
	buildpath := fmt.Sprintf("teldrive-ui/dist%s", path)

	if strings.HasSuffix(path, "/") {
		_, err := staticFS.ReadDir(strings.TrimSuffix(buildpath, "/"))
		return err == nil
	}

	f, err := staticFS.Open(buildpath)
	if f != nil {
		_ = f.Close()
	}
	return err == nil
}

type fallbackFileSystem struct {
	staticFileSystem *staticFileSystem
}

var _ static.ServeFileSystem = (*fallbackFileSystem)(nil)
var _ http.FileSystem = (*fallbackFileSystem)(nil)

func newFallbackFileSystem(staticFileSystem *staticFileSystem) *fallbackFileSystem {
	return &fallbackFileSystem{
		staticFileSystem: staticFileSystem,
	}
}

func (f *fallbackFileSystem) Open(path string) (http.File, error) {
	return f.staticFileSystem.Open("/index.html")
}

func (f *fallbackFileSystem) Exists(prefix string, path string) bool {
	return true
}
