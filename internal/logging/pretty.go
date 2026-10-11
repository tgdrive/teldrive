package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
)

const (
	ansiReset   = "\x1b[0m"
	ansiGray    = "\x1b[90m"
	ansiMagenta = "\x1b[35m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiRed     = "\x1b[31m"
	ansiWhite   = "\x1b[37m"
)

type prettyHandler struct {
	out    io.Writer
	level  slog.Leveler
	color  bool
	attrs  []slog.Attr
	groups []string
	mu     *sync.Mutex
}

func newPrettyHandler(out io.Writer, level slog.Leveler, color bool) slog.Handler {
	return &prettyHandler{out: out, level: level, color: color, mu: &sync.Mutex{}}
}

func supportsColor(out io.Writer) bool {
	fd, ok := out.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	return isatty.IsTerminal(fd.Fd()) || isatty.IsCygwinTerminal(fd.Fd())
}

func (h *prettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *prettyHandler) Handle(_ context.Context, record slog.Record) error {
	icon, levelText, color := prettyLevel(record.Level)
	if !h.color {
		color = ""
	}

	var line strings.Builder
	line.WriteString(record.Time.Format("2006-01-02 15:04:05"))
	line.WriteString("  ")
	line.WriteString(icon)
	line.WriteString(" ")
	if color != "" {
		line.WriteString(color)
	}
	line.WriteString(fmt.Sprintf("%-5s", levelText))
	if color != "" {
		line.WriteString(ansiReset)
	}
	line.WriteString("  ")
	line.WriteString(record.Message)

	attrs := append([]slog.Attr(nil), h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})
	for _, attr := range attrs {
		h.appendAttr(&line, h.groups, attr)
	}
	line.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.out, line.String())
	return err
}

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &clone
}

func (h *prettyHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.groups = append(append([]string(nil), h.groups...), name)
	return &clone
}

func (h *prettyHandler) appendAttr(line *strings.Builder, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}
	if attr.Value.Kind() == slog.KindGroup {
		nested := append(append([]string(nil), groups...), attr.Key)
		for _, child := range attr.Value.Group() {
			h.appendAttr(line, nested, child)
		}
		return
	}

	keyParts := append([]string(nil), groups...)
	if attr.Key != "" {
		keyParts = append(keyParts, attr.Key)
	}
	key := strings.Join(keyParts, ".")
	if key == "" {
		return
	}

	line.WriteString("  ")
	if h.color {
		line.WriteString(ansiGray)
	}
	line.WriteString(key)
	line.WriteString("=")
	if h.color {
		line.WriteString(ansiReset)
	}
	line.WriteString(formatSlogValue(attr.Value))
}

func prettyLevel(level slog.Level) (icon, text, color string) {
	switch {
	case level <= slog.LevelDebug:
		return "🐛", "DEBUG", ansiMagenta
	case level < slog.LevelWarn:
		return "✓", "INFO", ansiGreen
	case level < slog.LevelError:
		return "⚠", "WARN", ansiYellow
	case level >= slog.LevelError:
		return "✗", "ERROR", ansiRed
	default:
		return "·", strings.ToUpper(level.String()), ansiWhite
	}
}

func formatSlogValue(value slog.Value) string {
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindBool:
		return fmt.Sprintf("%t", value.Bool())
	case slog.KindInt64:
		return fmt.Sprintf("%d", value.Int64())
	case slog.KindUint64:
		return fmt.Sprintf("%d", value.Uint64())
	case slog.KindFloat64:
		return fmt.Sprintf("%g", value.Float64())
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339)
	case slog.KindAny:
		if err, ok := value.Any().(error); ok {
			return err.Error()
		}
		return fmt.Sprint(value.Any())
	default:
		return value.String()
	}
}
