package telegramstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/transport"
)

func TestProxyResolverIPv4Only(t *testing.T) {
	for _, kind := range []string{"primary", "media", "cdn"} {
		t.Run(kind, func(t *testing.T) {
			var mu sync.Mutex
			var targets []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				targets = append(targets, r.Method+" "+r.Host)
				mu.Unlock()
				http.Error(w, "test proxy refuses upstream connections", http.StatusBadGateway)
			}))
			defer server.Close()

			resolver, err := resolverFromConfig(FactoryConfig{
				Proxy:       server.URL,
				DialTimeout: time.Second,
			})
			if err != nil {
				t.Fatal(err)
			}
			var connect func(context.Context, int, dcs.List) (transport.Conn, error)
			switch kind {
			case "primary":
				connect = resolver.Primary
			case "media":
				connect = resolver.MediaOnly
			case "cdn":
				connect = resolver.CDN
			}

			list := dcs.List{Options: []tg.DCOption{
				{ID: 2, IPAddress: "149.154.167.50", Port: 443, MediaOnly: kind == "media", CDN: kind == "cdn"},
				{ID: 2, IPAddress: "2001:b28:f23f:f005::a", Port: 443, Ipv6: true, MediaOnly: kind == "media", CDN: kind == "cdn"},
			}}
			original := slices.Clone(list.Options)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if conn, err := connect(ctx, 2, list); err == nil {
				_ = conn.Close()
				t.Fatal("connection succeeded despite proxy refusing the tunnel")
			}
			mu.Lock()
			got := slices.Clone(targets)
			mu.Unlock()
			if !reflect.DeepEqual(got, []string{"CONNECT 149.154.167.50:443"}) {
				t.Fatalf("proxy requests = %v, want only IPv4 CONNECT", got)
			}
			if !reflect.DeepEqual(list.Options, original) {
				t.Fatalf("resolver mutated shared DC options: %v", list.Options)
			}

			list.Options = original[1:]
			_, err = connect(ctx, 2, list)
			if err == nil || !strings.Contains(err.Error(), "no addresses for DC 2") {
				t.Fatalf("IPv6-only DC error = %v, want no addresses for DC 2", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if !reflect.DeepEqual(targets, got) {
				t.Fatalf("IPv6-only DC contacted proxy: %v", targets)
			}
		})
	}
}
