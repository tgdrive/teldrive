package telegramstore

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/proxy"
)

type httpConnectDialer struct {
	proxyURL *url.URL
	forward  proxy.ContextDialer
	timeout  time.Duration
}

func newHTTPConnectDialer(proxyURL *url.URL, timeout time.Duration) (proxy.ContextDialer, error) {
	if proxyURL == nil || (proxyURL.Scheme != "http" && proxyURL.Scheme != "https") || proxyURL.Host == "" {
		return nil, errors.New("invalid HTTP proxy URL")
	}
	return &httpConnectDialer{proxyURL: proxyURL, forward: proxy.Direct, timeout: timeout}, nil
}

func (d *httpConnectDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

func (d *httpConnectDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("HTTP CONNECT proxy requires TCP network, got %q", network)
	}
	if d.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d.timeout)
		defer cancel()
	}

	conn, err := d.forward.DialContext(ctx, "tcp", d.proxyURL.Host)
	if err != nil {
		return nil, fmt.Errorf("connect to HTTP proxy: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = conn.Close()
		}
	}()

	if d.proxyURL.Scheme == "https" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: d.proxyURL.Hostname(), MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return nil, fmt.Errorf("TLS handshake with HTTP proxy: %w", err)
		}
		conn = tlsConn
	}

	header := make(http.Header)
	if d.proxyURL.User != nil {
		username := d.proxyURL.User.Username()
		password, _ := d.proxyURL.User.Password()
		token := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		header.Set("Proxy-Authorization", "Basic "+token)
	}
	request := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: address},
		Host:   address,
		Header: header,
	}
	if err := request.Write(conn); err != nil {
		return nil, fmt.Errorf("write HTTP CONNECT request: %w", err)
	}

	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return nil, fmt.Errorf("read HTTP CONNECT response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP CONNECT proxy returned %s", response.Status)
	}

	// A successful CONNECT response ends the HTTP protocol at the blank line.
	// Some proxies incorrectly include Transfer-Encoding or Content-Length on
	// the 2xx response. Do not close/drain response.Body here: those bytes are
	// the raw tunneled stream, and draining them can block forever waiting for
	// HTTP framing that does not exist.
	ok = true
	return &bufferedConn{Conn: conn, reader: reader}, nil
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}
