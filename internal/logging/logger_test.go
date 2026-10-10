package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestPrettyHandlerColorsLevels(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	handler := newPrettyHandler(&out, slog.LevelDebug, true)
	logger := slog.New(handler)
	logger.Log(context.Background(), slog.LevelWarn, "request failed", "status", 401)

	got := out.String()
	if !strings.Contains(got, ansiYellow+"WARN "+ansiReset) {
		t.Fatalf("colored WARN level missing from %q", got)
	}
	if !strings.Contains(got, ansiGray+"status="+ansiReset+"401") {
		t.Fatalf("colored attribute key missing from %q", got)
	}
}

func TestPrettyHandlerWithoutColor(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	handler := newPrettyHandler(&out, slog.LevelInfo, false)
	record := slog.NewRecord(time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC), slog.LevelInfo, "started", 0)
	record.Add("version", "1.2.3")
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	got := out.String()
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("unexpected ANSI color in %q", got)
	}
	if !strings.Contains(got, "✓ INFO") || !strings.Contains(got, "version=1.2.3") {
		t.Fatalf("pretty output = %q", got)
	}
}

func TestJSONLoggerRemainsUncolored(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger, err := NewLogger(&out, "info", "json")
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	logger.Info("started", "status", 200)

	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("JSON output contains ANSI color: %q", out.String())
	}
}
