package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestHTTPPublicURLValidation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		input string
		want  string
	}{
		{"", ""},
		{"  ", ""},
		{"https://drive.example.com", "https://drive.example.com"},
		{" https://drive.example.com/ ", "https://drive.example.com"},
		{"http://localhost:8080", "http://localhost:8080"},
		{"https://[2001:db8::1]:8443/", "https://[2001:db8::1]:8443"},
	} {
		t.Run(test.input, func(t *testing.T) {
			cfg := validTestConfig()
			cfg.HTTP.PublicURL = test.input
			if err := cfg.Validate(); err != nil {
				t.Fatal(err)
			}
			base, err := cfg.HTTP.PublicBaseURL()
			if err != nil || base.String() != test.want {
				t.Fatalf("public origin = %s, %v; want %s", base.String(), err, test.want)
			}
		})
	}
	for _, input := range []string{
		"drive.example.com", "//drive.example.com", "ftp://drive.example.com", "https:///missing-host",
		"https://user:do-not-leak@drive.example.com", "https://drive.example.com/drive", "https://drive.example.com/%2F",
		"https://drive.example.com?token=do-not-leak", "https://drive.example.com?", "https://drive.example.com#fragment", "https://drive.example.com#",
		"https://drive.example.com:0", "https://drive.example.com:65536", "https://drive.example.com:invalid", "https://drive.example.com/%",
	} {
		t.Run(input, func(t *testing.T) {
			cfg := validTestConfig()
			cfg.HTTP.PublicURL = input
			err := cfg.Validate()
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "http.public-url") {
				t.Fatalf("invalid origin accepted: %v", err)
			}
			if strings.Contains(err.Error(), "do-not-leak") {
				t.Fatal("configuration error exposed credentials")
			}
		})
	}
}

func TestPublicURLTOMLAndFlagPrecedence(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.toml")
	contents := `[http]
public-url = "https://drive.example.com/"
[database]
url = "postgres://example/teldrive"
[security]
signing-key = "0123456789abcdef0123456789abcdef"
data-key = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, flagURL := range []string{"", "https://other.example.com"} {
		loader := newLoader(func(string) (string, bool) { return "", false }, func() (string, error) { return "", nil })
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		loader.RegisterFlags(flags)
		args := []string{"--config", path}
		want := "https://drive.example.com/"
		if flagURL != "" {
			args = append(args, "--http-public-url", flagURL)
			want = flagURL
		}
		if err := flags.Parse(args); err != nil {
			t.Fatal(err)
		}
		cfg, err := loader.Load(flags)
		if err != nil || cfg.HTTP.PublicURL != want {
			t.Fatalf("public URL from TOML/flag = %s, %v; want %s", cfg.HTTP.PublicURL, err, want)
		}
	}
}

func TestLoadPublicURLFromEnvironment(t *testing.T) {
	t.Parallel()
	cfg, err := LoadFrom(func(key string) (string, bool) {
		switch key {
		case "TELDRIVE_HTTP_PUBLIC_URL":
			return "https://drive.example.com", true
		case "TELDRIVE_DATABASE_URL":
			return "postgres://example/teldrive", true
		case "TELDRIVE_SECURITY_SIGNING_KEY":
			return strings.Repeat("x", 32), true
		case "TELDRIVE_SECURITY_DATA_KEY":
			return "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", true
		}
		return "", false
	})
	if err != nil || cfg.HTTP.PublicURL != "https://drive.example.com" {
		t.Fatalf("public-url environment setting = %s, %v", cfg.HTTP.PublicURL, err)
	}
}
