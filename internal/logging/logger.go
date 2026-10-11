package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

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
