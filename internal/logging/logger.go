package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

func NewLoggerWithFile(output io.Writer, levelText, format, path string) (*slog.Logger, func() error, error) {
	closeOutput := func() error { return nil }
	// Validate before opening the file so invalid options have no side effects.
	logger, err := NewLogger(output, levelText, format)
	if err != nil || path == "" {
		return logger, closeOutput, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, closeOutput, fmt.Errorf("open log file %q: %w", path, err)
	}
	fileLogger, err := NewLogger(file, levelText, format)
	if err != nil {
		_ = file.Close()
		return nil, closeOutput, err
	}
	return slog.New(teeHandler{logger.Handler(), fileLogger.Handler()}), file.Close, nil
}

type teeHandler []slog.Handler

func (h teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h teeHandler) Handle(ctx context.Context, record slog.Record) error {
	var err error
	for _, handler := range h {
		if handler.Enabled(ctx, record.Level) {
			err = errors.Join(err, handler.Handle(ctx, record.Clone()))
		}
	}
	return err
}

func (h teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	result := make(teeHandler, len(h))
	for i, handler := range h {
		result[i] = handler.WithAttrs(attrs)
	}
	return result
}

func (h teeHandler) WithGroup(name string) slog.Handler {
	result := make(teeHandler, len(h))
	for i, handler := range h {
		result[i] = handler.WithGroup(name)
	}
	return result
}

func NewLogger(output io.Writer, levelText, format string) (*slog.Logger, error) {
	if output == nil {
		return nil, fmt.Errorf("logger output is required")
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.TrimSpace(levelText))); err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		handler = slog.NewJSONHandler(output, opts)
	case "text":
		handler = newPrettyHandler(output, level, supportsColor(output))
	default:
		return nil, fmt.Errorf("unsupported log format %q", format)
	}
	return slog.New(handler), nil
}
