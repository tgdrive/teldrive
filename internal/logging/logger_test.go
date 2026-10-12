package logging

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoggerWithFile(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "teldrive.log")
			var console bytes.Buffer
			for range 2 {
				logger, closeLog, err := NewLoggerWithFile(&console, "info", format, path)
				if err != nil {
					t.Fatal(err)
				}
				logger.Debug("filtered")
				logger.With("component", "test").Info("started")
				if err := closeLog(); err != nil {
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != console.String() || strings.Count(string(data), "started") != 2 || strings.Contains(string(data), "filtered") || strings.Contains(string(data), "\x1b[") {
				t.Fatalf("unexpected file output: %q; console: %q", data, console.String())
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm()&0o077 != 0 {
				t.Fatalf("log file permissions = %o", info.Mode().Perm())
			}
		})
	}
}

func TestLoggerWithFileDisabled(t *testing.T) {
	t.Parallel()
	var console bytes.Buffer
	logger, closeLog, err := NewLoggerWithFile(&console, "info", "text", "")
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("started")
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(console.String(), "started") {
		t.Fatal("console output missing")
	}
}

func TestLoggerWithFileErrors(t *testing.T) {
	t.Parallel()
	var console bytes.Buffer
	path := filepath.Join(t.TempDir(), "missing", "teldrive.log")
	if _, _, err := NewLoggerWithFile(&console, "info", "json", path); err == nil {
		t.Fatal("missing parent directory should fail")
	}
	path = filepath.Join(t.TempDir(), "teldrive.log")
	if _, _, err := NewLoggerWithFile(&console, "invalid", "json", path); err == nil {
		t.Fatal("invalid level should fail")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid options created file: %v", err)
	}
}

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
