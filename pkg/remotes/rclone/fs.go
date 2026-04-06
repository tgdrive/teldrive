package rclone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/tgdrive/teldrive/pkg/remotes"
)

type FS struct {
	rcURL  *url.URL
	remote string
	root   string
}

func New(source string) (*FS, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, err
	}
	host := strings.TrimSpace(u.Host)
	if host == "" {
		return nil, fmt.Errorf("rclone rc host is required")
	}
	if !strings.Contains(host, ":") {
		host += ":5572"
	}

	rawPath := strings.Trim(strings.TrimSpace(u.Path), "/")
	if rawPath == "" {
		return nil, fmt.Errorf("rclone remote path is required as /<remote>/<path>")
	}

	parts := strings.SplitN(rawPath, "/", 2)
	remote := strings.TrimSpace(parts[0])
	if remote == "" {
		return nil, fmt.Errorf("rclone remote name is required")
	}
	root := ""
	if len(parts) == 2 {
		root = strings.TrimPrefix(strings.TrimSpace(parts[1]), "/")
	}

	rcURL := &url.URL{Scheme: "http", Host: host, User: u.User}
	if strings.EqualFold(u.Scheme, "rclones") {
		rcURL.Scheme = "https"
	}
	if scheme := strings.TrimSpace(u.Query().Get("scheme")); strings.EqualFold(scheme, "https") {
		rcURL.Scheme = "https"
	}
	return &FS{rcURL: rcURL, remote: remote, root: root}, nil
}

func (r *FS) List(ctx context.Context, nameOverride string, _ map[string]string, _ string) ([]remotes.Entry, error) {
	type rcEntry struct {
		Path    string            `json:"Path"`
		Name    string            `json:"Name"`
		Size    int64             `json:"Size"`
		Mime    string            `json:"MimeType"`
		ModTime time.Time         `json:"ModTime"`
		IsDir   bool              `json:"IsDir"`
		Hashes  map[string]string `json:"Hashes"`
	}
	type listResp struct {
		List []rcEntry `json:"list"`
	}

	var out listResp
	if err := r.callJSON(ctx, "/operations/list", map[string]any{
		"fs":      r.remote + ":",
		"remote":  r.root,
		"recurse": true,
		"hashes":  true,
	}, &out); err != nil {
		return nil, err
	}

	files := make([]remotes.Entry, 0, len(out.List))
	for _, item := range out.List {
		if item.IsDir {
			continue
		}
		full := strings.TrimPrefix(path.Clean(path.Join(r.root, item.Path)), "/")
		name := item.Name
		if name == "" {
			name = path.Base(full)
		}
		rel := item.Path
		if rel == "" {
			rel = name
		}
		if nameOverride != "" && len(out.List) == 1 {
			name = strings.TrimSpace(nameOverride)
			rel = name
		}
		files = append(files, remotes.Entry{
			RelPath:    rel,
			SourcePath: full,
			Name:       name,
			Size:       item.Size,
			MimeType:   item.Mime,
			Hash:       pickHash(item.Hashes),
			ModifiedAt: item.ModTime.UTC(),
		})
	}
	return files, nil
}

func (r *FS) Open(ctx context.Context, sourcePath string, _ map[string]string, _ string, sizeHint int64) (io.ReadCloser, int64, string, error) {
	if sourcePath == "" {
		sourcePath = r.root
	}
	objectPath := r.relativeObjectPath(sourcePath)
	serveURL := r.serveObjectURL(r.serveBaseRemote(), objectPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serveURL, nil)
	if err != nil {
		return nil, 0, "", err
	}
	if r.rcURL.User != nil {
		pw, _ := r.rcURL.User.Password()
		req.SetBasicAuth(r.rcURL.User.Username(), pw)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, 0, "", fmt.Errorf("rclone serve get failed for %q: status=%d body=%s", serveURL, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	size := sizeHint
	if size <= 0 {
		size = remotes.ParseContentLength(resp.Header.Get("Content-Length"))
	}
	mimeType := remotes.ParseMimeType(resp.Header.Get("Content-Type"))
	if mimeType == "" {
		mimeType = mime.TypeByExtension(path.Ext(sourcePath))
	}
	return resp.Body, size, mimeType, nil
}

func (r *FS) callJSON(ctx context.Context, endpoint string, payload map[string]any, out any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint(endpoint), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.rcURL.User != nil {
		pw, _ := r.rcURL.User.Password()
		req.SetBasicAuth(r.rcURL.User.Username(), pw)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("rclone rc %s failed: %s", endpoint, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (r *FS) endpoint(p string) string {
	base := *r.rcURL
	base.Path = strings.TrimSuffix(base.Path, "/") + p
	base.RawQuery = ""
	return base.String()
}

func (r *FS) serveObjectURL(baseRemote, objectPath string) string {
	baseRemote = strings.TrimSpace(baseRemote)
	if !strings.Contains(baseRemote, ":") {
		baseRemote += ":"
	}
	objectPath = strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(objectPath)), "/")
	base := *r.rcURL
	if objectPath == "" {
		base.Path = "/[" + baseRemote + "]"
	} else {
		base.Path = "/[" + baseRemote + "]/" + objectPath
	}
	base.RawQuery = ""
	return base.String()
}

func (r *FS) serveBaseRemote() string {
	if strings.TrimSpace(r.root) == "" {
		return r.remote
	}
	return r.remote + ":" + r.root
}

func (r *FS) relativeObjectPath(sourcePath string) string {
	clean := strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(sourcePath)), "/")
	root := strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(r.root)), "/")
	if root == "" {
		return clean
	}
	for {
		if clean == root {
			return ""
		}
		prefix := root + "/"
		if !strings.HasPrefix(clean, prefix) {
			break
		}
		clean = strings.TrimPrefix(clean, prefix)
	}
	return clean
}

func pickHash(hashes map[string]string) string {
	for _, k := range []string{"blake3", "sha256", "md5", "sha1"} {
		if v := strings.TrimSpace(hashes[k]); v != "" {
			return v
		}
	}
	for _, v := range hashes {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
