package sftp

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/url"
	"path"
	"strings"
	"time"

	pkgsftp "github.com/pkg/sftp"
	"github.com/tgdrive/teldrive/pkg/remotes"
	"golang.org/x/crypto/ssh"
)

type FS struct {
	host string
	user string
	pass string
	root string
}

func New(source string) (*FS, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, err
	}
	if u.Host == "" {
		return nil, fmt.Errorf("missing sftp host")
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
	return &FS{host: u.Host, user: user, pass: pass, root: root}, nil
}

func (s *FS) dial() (*ssh.Client, *pkgsftp.Client, error) {
	if s.user == "" {
		return nil, nil, fmt.Errorf("sftp username is required")
	}
	conf := &ssh.ClientConfig{
		User:            s.user,
		Auth:            []ssh.AuthMethod{ssh.Password(s.pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}
	conn, err := ssh.Dial("tcp", s.host, conf)
	if err != nil {
		return nil, nil, err
	}
	client, err := pkgsftp.NewClient(conn)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return conn, client, nil
}

func (s *FS) List(_ context.Context, nameOverride string, _ map[string]string, _ string) ([]remotes.Entry, error) {
	conn, client, err := s.dial()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = client.Close()
		_ = conn.Close()
	}()

	st, err := client.Stat(s.root)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		name := path.Base(s.root)
		if strings.TrimSpace(nameOverride) != "" {
			name = strings.TrimSpace(nameOverride)
		}
		return []remotes.Entry{{
			RelPath:    name,
			SourcePath: s.root,
			Name:       name,
			Size:       st.Size(),
			MimeType:   mime.TypeByExtension(path.Ext(name)),
			ModifiedAt: st.ModTime().UTC(),
		}}, nil
	}

	rootClean := path.Clean(s.root)
	out := make([]remotes.Entry, 0, 128)
	w := client.Walk(rootClean)
	for w.Step() {
		if err := w.Err(); err != nil {
			return nil, err
		}
		stat := w.Stat()
		if stat == nil || stat.IsDir() {
			continue
		}
		full := w.Path()
		rel := strings.TrimPrefix(path.Clean(full), rootClean)
		rel = strings.TrimPrefix(rel, "/")
		out = append(out, remotes.Entry{
			RelPath:    rel,
			SourcePath: full,
			Name:       path.Base(rel),
			Size:       stat.Size(),
			MimeType:   mime.TypeByExtension(path.Ext(rel)),
			ModifiedAt: stat.ModTime().UTC(),
		})
	}
	return out, nil
}

func (s *FS) Open(_ context.Context, sourcePath string, _ map[string]string, _ string, sizeHint int64) (io.ReadCloser, int64, string, error) {
	if sourcePath == "" {
		sourcePath = s.root
	}
	conn, client, err := s.dial()
	if err != nil {
		return nil, 0, "", err
	}
	f, err := client.Open(sourcePath)
	if err != nil {
		_ = client.Close()
		_ = conn.Close()
		return nil, 0, "", err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		_ = client.Close()
		_ = conn.Close()
		return nil, 0, "", err
	}
	size := sizeHint
	if size <= 0 {
		size = st.Size()
	}
	return &compoundReadCloser{Reader: f, closers: []io.Closer{f, client, conn}}, size, mime.TypeByExtension(path.Ext(sourcePath)), nil
}

type compoundReadCloser struct {
	io.Reader
	closers []io.Closer
}

func (c *compoundReadCloser) Close() error {
	var firstErr error
	for i := len(c.closers) - 1; i >= 0; i-- {
		if err := c.closers[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
