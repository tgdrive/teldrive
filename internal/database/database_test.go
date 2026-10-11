package database

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "missing URL", cfg: Config{}},
		{name: "negative max", cfg: Config{URL: "postgres://example", MaxConnections: -1}},
		{name: "negative min", cfg: Config{URL: "postgres://example", MinConnections: -1}},
		{name: "min exceeds max", cfg: Config{URL: "postgres://example", MinConnections: 2, MaxConnections: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}

	if err := (Config{URL: "postgres://example", MinConnections: 1, MaxConnections: 2}).validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestOpenRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	if _, err := Open(context.Background(), Config{}); err == nil {
		t.Fatal("expected missing URL error")
	}
	if _, err := Open(context.Background(), Config{URL: "://bad"}); err == nil || !strings.Contains(err.Error(), "parse database URL") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestOpenReportsConnectionFailure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := Open(ctx, Config{
		URL:            "postgres://nobody:nobody@127.0.0.1:1/missing?sslmode=disable",
		ConnectTimeout: 100 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "ping database") {
		t.Fatalf("expected ping failure, got %v", err)
	}
}

func TestMigrateRejectsMissingURL(t *testing.T) {
	t.Parallel()
	if err := Migrate(context.Background(), Config{}); err == nil {
		t.Fatal("expected missing URL error")
	}
}
