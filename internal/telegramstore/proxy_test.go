package telegramstore

import (
	"testing"
	"time"
)

func TestResolverFromConfigProxy(t *testing.T) {
	for _, proxyURL := range []string{
		"http://127.0.0.1:8080",
		"https://127.0.0.1:8443",
		"http://user:password@127.0.0.1:8080",
		"socks5://127.0.0.1:1080",
	} {
		t.Run(proxyURL, func(t *testing.T) {
			resolver, err := resolverFromConfig(FactoryConfig{
				Proxy:       proxyURL,
				DialTimeout: time.Second,
			})
			if err != nil {
				t.Fatalf("resolverFromConfig() error = %v", err)
			}
			if resolver == nil {
				t.Fatal("resolverFromConfig() returned nil resolver")
			}
		})
	}
}
