package remotes

import (
	"context"
	"io"
	"time"
)

type Entry struct {
	RelPath    string
	SourcePath string
	Name       string
	Size       int64
	MimeType   string
	Hash       string
	ModifiedAt time.Time
}

type FS interface {
	List(ctx context.Context, nameOverride string, headers map[string]string, proxyURL string) ([]Entry, error)
	Open(ctx context.Context, sourcePath string, headers map[string]string, proxyURL string, sizeHint int64) (io.ReadCloser, int64, string, error)
}
