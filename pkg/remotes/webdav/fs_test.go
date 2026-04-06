package webdav

import "testing"

func TestNewSchemeMapping(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		server      string
		root        string
		insecureTLS bool
	}{
		{
			name:   "dav uses http",
			source: "dav://user:pass@example.com/root/path",
			server: "http://example.com",
			root:   "/root/path",
		},
		{
			name:   "davs uses https",
			source: "davs://user:pass@example.com/root/path",
			server: "https://example.com",
			root:   "/root/path",
		},
		{
			name:        "davs insecure keeps https",
			source:      "davs://example.com/root/path?insecure=true",
			server:      "https://example.com",
			root:        "/root/path",
			insecureTLS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs, err := New(tt.source)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if fs.server != tt.server {
				t.Fatalf("server = %q, want %q", fs.server, tt.server)
			}
			if fs.root != tt.root {
				t.Fatalf("root = %q, want %q", fs.root, tt.root)
			}
			if fs.insecureTLS != tt.insecureTLS {
				t.Fatalf("insecureTLS = %v, want %v", fs.insecureTLS, tt.insecureTLS)
			}
		})
	}
}

func TestNewRejectsUnsupportedScheme(t *testing.T) {
	if _, err := New("ftp://example.com/root"); err == nil {
		t.Fatal("expected error for unsupported scheme")
	}
}
