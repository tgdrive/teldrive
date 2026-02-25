package httpremote

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tgdrive/teldrive/pkg/remotes"
)

type FS struct {
	source string
}

func New(source string) (*FS, error) {
	return &FS{source: source}, nil
}

func (h *FS) List(ctx context.Context, nameOverride string, headers map[string]string, proxyURL string) ([]remotes.Entry, error) {
	meta, err := probe(ctx, h.source, headers, proxyURL)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(nameOverride)
	if name == "" {
		name = meta.name
	}
	if name == "" {
		name = "remote-file"
	}
	return []remotes.Entry{{
		RelPath:    name,
		SourcePath: "",
		Name:       name,
		Size:       meta.size,
		MimeType:   meta.mimeType,
		ModifiedAt: meta.modified,
	}}, nil
}

func (h *FS) Open(ctx context.Context, _ string, headers map[string]string, proxyURL string, sizeHint int64) (io.ReadCloser, int64, string, error) {
	client, err := remotes.HTTPClient(strings.TrimSpace(proxyURL), 0)
	if err != nil {
		return nil, 0, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.source, nil)
	if err != nil {
		return nil, 0, "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		_ = resp.Body.Close()
		return nil, 0, "", fmt.Errorf("http source returned status %d", resp.StatusCode)
	}
	size := sizeHint
	if size <= 0 {
		size = remotes.ParseContentLength(resp.Header.Get("Content-Length"))
	}
	return resp.Body, size, remotes.ParseMimeType(resp.Header.Get("Content-Type")), nil
}

type meta struct {
	size     int64
	name     string
	mimeType string
	modified time.Time
}

func probe(ctx context.Context, link string, headers map[string]string, proxyURL string) (*meta, error) {
	client, err := remotes.HTTPClient(strings.TrimSpace(proxyURL), 60*time.Second)
	if err != nil {
		return nil, err
	}
	out := &meta{}
	headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, link, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		headReq.Header.Set(k, v)
	}
	if resp, err := client.Do(headReq); err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			out.size = remotes.ParseContentLength(resp.Header.Get("Content-Length"))
			out.mimeType = remotes.ParseMimeType(resp.Header.Get("Content-Type"))
			out.name = remotes.ParseFileNameFromHeadersOrURL(resp.Header.Get("Content-Disposition"), link)
			out.modified = remotes.ParseHTTPTime(resp.Header.Get("Last-Modified"))
		}
	}
	if out.size > 0 {
		return out, nil
	}
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		getReq.Header.Set(k, v)
	}
	getReq.Header.Set("Range", "bytes=0-0")
	resp, err := client.Do(getReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if out.mimeType == "" {
		out.mimeType = remotes.ParseMimeType(resp.Header.Get("Content-Type"))
	}
	if out.name == "" {
		out.name = remotes.ParseFileNameFromHeadersOrURL(resp.Header.Get("Content-Disposition"), link)
	}
	if out.modified.IsZero() {
		out.modified = remotes.ParseHTTPTime(resp.Header.Get("Last-Modified"))
	}
	if out.size == 0 {
		out.size = parseContentRangeTotal(resp.Header.Get("Content-Range"))
	}
	return out, nil
}

func parseContentRangeTotal(v string) int64 {
	idx := strings.LastIndex(v, "/")
	if idx < 0 || idx+1 >= len(v) {
		return 0
	}
	return remotes.ParseContentLength(v[idx+1:])
}
