package telegramstore

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestHTTPConnectDialerIgnoresChunkedHeaderOnSuccessfulTunnel(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		request, err := http.ReadRequest(bufio.NewReader(conn))
		if err != nil {
			serverErr <- err
			return
		}
		if request.Method != http.MethodConnect || request.Host != "91.108.56.191:443" {
			serverErr <- fmt.Errorf("unexpected CONNECT request: method=%q host=%q", request.Method, request.Host)
			return
		}
		if _, err := io.WriteString(conn, "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\ntelegram"); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	proxyURL, err := url.Parse("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	dialer, err := newHTTPConnectDialer(proxyURL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp", "91.108.56.191:443")
	if err != nil {
		t.Fatalf("DialContext() error = %v", err)
	}
	defer conn.Close()

	got := make([]byte, len("telegram"))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read tunneled bytes: %v", err)
	}
	if string(got) != "telegram" {
		t.Fatalf("tunneled bytes = %q, want %q", got, "telegram")
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}
