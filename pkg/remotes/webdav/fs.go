package webdav

import (
	"context"
	"io"
	"mime"
	"net/url"
	"path"
	"strings"

	"github.com/studio-b12/gowebdav"
	"github.com/tgdrive/teldrive/pkg/remotes"
)

type FS struct {
	root   string
	client *gowebdav.Client
}

func New(source string) (*FS, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, err
	}
	server := "https://" + u.Host
	if u.Query().Get("insecure") == "true" {
		server = "http://" + u.Host
	}
	user := ""
	pass := ""
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	}
	root := strings.TrimSpace(u.Path)
	if root == "" {
		root = "/"
	}
	return &FS{root: root, client: gowebdav.NewClient(server, user, pass)}, nil
}

func (w *FS) List(_ context.Context, nameOverride string, _ map[string]string, _ string) ([]remotes.Entry, error) {
	st, err := w.client.Stat(w.root)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		name := st.Name()
		if strings.TrimSpace(nameOverride) != "" {
			name = strings.TrimSpace(nameOverride)
		}
		return []remotes.Entry{{
			RelPath:    name,
			SourcePath: w.root,
			Name:       name,
			Size:       st.Size(),
			MimeType:   mime.TypeByExtension(path.Ext(name)),
			ModifiedAt: st.ModTime().UTC(),
		}}, nil
	}

	rootClean := path.Clean(w.root)
	out := make([]remotes.Entry, 0, 128)
	var walk func(string) error
	walk = func(curr string) error {
		items, err := w.client.ReadDir(curr)
		if err != nil {
			return err
		}
		for _, item := range items {
			full := path.Join(curr, item.Name())
			if item.IsDir() {
				if err := walk(full); err != nil {
					return err
				}
				continue
			}
			rel := strings.TrimPrefix(path.Clean(full), rootClean)
			rel = strings.TrimPrefix(rel, "/")
			out = append(out, remotes.Entry{
				RelPath:    rel,
				SourcePath: full,
				Name:       path.Base(rel),
				Size:       item.Size(),
				MimeType:   mime.TypeByExtension(path.Ext(rel)),
				ModifiedAt: item.ModTime().UTC(),
			})
		}
		return nil
	}
	if err := walk(rootClean); err != nil {
		return nil, err
	}
	return out, nil
}

func (w *FS) Open(_ context.Context, sourcePath string, _ map[string]string, _ string, sizeHint int64) (io.ReadCloser, int64, string, error) {
	if sourcePath == "" {
		sourcePath = w.root
	}
	rc, err := w.client.ReadStream(sourcePath)
	if err != nil {
		return nil, 0, "", err
	}
	st, err := w.client.Stat(sourcePath)
	if err != nil {
		_ = rc.Close()
		return nil, 0, "", err
	}
	size := sizeHint
	if size <= 0 {
		size = st.Size()
	}
	return rc, size, mime.TypeByExtension(path.Ext(sourcePath)), nil
}
