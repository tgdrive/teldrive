package database

import (
	"context"
	"strings"
	"testing"
	"time"

	jetpostgres "github.com/go-jet/jet/v2/postgres"
	"github.com/tgdrive/teldrive/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestParseDBLogLevel(t *testing.T) {
	tests := map[string]string{
		"debug": "debug",
		"info":  "info",
		"warn":  "warn",
		"error": "error",
		"":      "error",
	}

	for input, want := range tests {
		if got := parseDBLogLevel(input).String(); got != want {
			t.Fatalf("parseDBLogLevel(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNewJetQueryLogger_UsesDebugSQL(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	queryLogger := newJetQueryLogger(&config.DBLoggingConfig{
		LogSQL:        true,
		Level:         "debug",
		SlowThreshold: time.Hour,
	}, logger)

	stmt := jetpostgres.RawStatement("select 1")
	queryLogger(context.Background(), jetpostgres.QueryInfo{Statement: stmt, Duration: 10 * time.Millisecond})

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Message != "db.query" {
		t.Fatalf("unexpected log message: %s", entries[0].Message)
	}
	query, ok := entries[0].ContextMap()["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		t.Fatalf("expected non-empty query field, got %v", entries[0].ContextMap()["query"])
	}
	if !strings.Contains(query, "select 1") {
		t.Fatalf("expected query field to contain SQL text, got %q", query)
	}
}

func TestNewJetQueryLogger_SlowQueriesWarn(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	queryLogger := newJetQueryLogger(&config.DBLoggingConfig{
		LogSQL:        true,
		Level:         "debug",
		SlowThreshold: 5 * time.Millisecond,
	}, logger)

	stmt := jetpostgres.RawStatement("select 1")
	queryLogger(context.Background(), jetpostgres.QueryInfo{Statement: stmt, Duration: 10 * time.Millisecond})

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Message != "db.query.slow" {
		t.Fatalf("unexpected log message: %s", entries[0].Message)
	}
}
