package remotes

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/tgdrive/teldrive/internal/utils"
)

func HTTPClient(proxyURL string, timeout time.Duration) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL != "" {
		dialer, err := utils.Proxy.GetDial(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid remote proxy: %w", err)
		}
		transport.DialContext = dialer.DialContext
	}
	return &http.Client{Timeout: timeout, Transport: transport}, nil
}

func ParseContentLength(v string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	return n
}

func ParseMimeType(v string) string {
	if v == "" {
		return ""
	}
	parts := strings.Split(v, ";")
	return strings.TrimSpace(parts[0])
}

func ParseHTTPTime(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}
	}
	t, err := http.ParseTime(v)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func ParseFileNameFromHeadersOrURL(contentDisposition, rawURL string) string {
	if _, params, err := mime.ParseMediaType(contentDisposition); err == nil {
		if name := strings.TrimSpace(params["filename"]); name != "" {
			return name
		}
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	base := path.Base(u.Path)
	if base == "." || base == "/" {
		return ""
	}
	return base
}
