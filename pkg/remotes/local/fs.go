package local

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/tgdrive/teldrive/pkg/remotes"
)

type FS struct {
	root string
}

func New(source string) (*FS, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, err
	}
	root := pathFromURI(u)
	if root == "" {
		return nil, fmt.Errorf("invalid local source")
	}
	return &FS{root: root}, nil
}

func (l *FS) List(_ context.Context, nameOverride string, _ map[string]string, _ string) ([]remotes.Entry, error) {
	info, err := os.Stat(l.root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		name := info.Name()
		if strings.TrimSpace(nameOverride) != "" {
			name = strings.TrimSpace(nameOverride)
		}
		return []remotes.Entry{{
			RelPath:    name,
			SourcePath: l.root,
			Name:       name,
			Size:       info.Size(),
			MimeType:   mime.TypeByExtension(filepath.Ext(name)),
			ModifiedAt: info.ModTime().UTC(),
		}}, nil
	}

	out := make([]remotes.Entry, 0, 128)
	err = filepath.WalkDir(l.root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(l.root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		out = append(out, remotes.Entry{
			RelPath:    rel,
			SourcePath: p,
			Name:       path.Base(rel),
			Size:       fi.Size(),
			MimeType:   mime.TypeByExtension(filepath.Ext(rel)),
			ModifiedAt: fi.ModTime().UTC(),
		})
		return nil
	})
	return out, err
}

func (l *FS) Open(_ context.Context, sourcePath string, _ map[string]string, _ string, sizeHint int64) (io.ReadCloser, int64, string, error) {
	fullPath := sourcePath
	if fullPath == "" {
		fullPath = l.root
	}
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, 0, "", err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, "", err
	}
	size := sizeHint
	if size <= 0 {
		size = fi.Size()
	}
	return f, size, mime.TypeByExtension(filepath.Ext(fullPath)), nil
}

func pathFromURI(u *url.URL) string {
	host := strings.TrimSpace(u.Host)
	p := strings.TrimSpace(u.Path)
	if host != "" && p != "" {
		p = "/" + path.Join(host, strings.TrimPrefix(p, "/"))
	} else if host != "" {
		p = host
	}
	if p == "" {
		return ""
	}
	if decoded, err := url.PathUnescape(p); err == nil {
		p = decoded
	}
	return filepath.Clean(p)
}
