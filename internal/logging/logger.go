package logging

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type contextKey string

const loggerKey = contextKey("logger")

var (
	defaultLogger     *zap.Logger
	defaultLoggerOnce sync.Once
)

var conf = &Config{
	Level:      zapcore.InfoLevel,
	TimeFormat: "2006-01-02 15:04:05",
}

// Config holds logging configuration
type Config struct {
	Level      zapcore.Level
	FilePath   string
	TimeFormat string // e.g., "2006-01-02 15:04:05" or "02/01/2006 03:04 PM"
}

// SetConfig updates the global logging configuration
func SetConfig(c *Config) {
	conf = &Config{
		Level:      c.Level,
		FilePath:   c.FilePath,
		TimeFormat: c.TimeFormat,
	}
}

// SetLevel updates just the log level
func SetLevel(l zapcore.Level) {
	conf.Level = l
}

// prettyCore is a custom zapcore.Core for human-readable, colored, non-structured logs
type prettyCore struct {
	level  zapcore.Level
	out    zapcore.WriteSyncer
	fields []zapcore.Field
}

func (c *prettyCore) Enabled(l zapcore.Level) bool {
	return l >= c.level
}

func (c *prettyCore) With(fields []zapcore.Field) zapcore.Core {
	return &prettyCore{
		level:  c.level,
		out:    c.out,
		fields: append(c.fields[:len(c.fields):len(c.fields)], fields...),
	}
}

func (c *prettyCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *prettyCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	var component string
	var otherFields []string

	// Combine baked-in fields and call-site fields
	allFields := append(c.fields[:len(c.fields):len(c.fields)], fields...)

	for _, f := range allFields {
		if f.Key == "component" {
			component = f.String
		} else {
			// Format other fields as key=value
			var val string
			switch f.Type {
			case zapcore.StringType:
				val = f.String
			case zapcore.Int64Type, zapcore.Int32Type:
				val = fmt.Sprintf("%d", f.Integer)
			case zapcore.ErrorType:
				val = f.Interface.(error).Error()
			case zapcore.BoolType:
				val = fmt.Sprintf("%v", f.Integer != 0)
			case zapcore.DurationType:
				d := time.Duration(f.Integer)
				val = (&d).String()
			default:
				if f.Interface != nil {
					val = fmt.Sprintf("%v", f.Interface)
				} else {
					val = fmt.Sprintf("%v", f.Integer)
				}
			}
			otherFields = append(otherFields, fmt.Sprintf("\x1b[90m%s=\x1b[0m%v", f.Key, val))
		}
	}

	// Prepare Level Icon and Color
	var icon, color string
	switch ent.Level {
	case zapcore.DebugLevel:
		icon, color = "🐛", "\x1b[35m" // Magenta
	case zapcore.InfoLevel:
		icon, color = "✓", "\x1b[32m" // Green
	case zapcore.WarnLevel:
		icon, color = "⚠", "\x1b[33m" // Yellow
	case zapcore.ErrorLevel, zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		icon, color = "✗", "\x1b[31m" // Red
	default:
		icon, color = "·", "\x1b[37m" // White
	}

	// Format Component
	compStr := ""
	if component != "" {
		compStr = fmt.Sprintf("\x1b[36m[%s]\x1b[0m ", strings.ToUpper(component))
	}

	// Build the line
	// Format: HH:MM:SS  ICON LEVEL  [COMPONENT] MESSAGE   key=val key=val
	line := fmt.Sprintf("%s  %s %s%-5s\x1b[0m  %s%s",
		ent.Time.Format("2006-01-02 15:04:05"),
		icon,
		color,
		strings.ToUpper(ent.Level.String()),
		compStr,
		ent.Message,
	)

	if len(otherFields) > 0 {
		line += "  " + strings.Join(otherFields, " ")
	}
	line += "\n"

	_, err := c.out.Write([]byte(line))
	return err
}

func (c *prettyCore) Sync() error {
	return c.out.Sync()
}

// Component returns a logger with a component field for identification
func Component(name string) *zap.Logger {
	return DefaultLogger().With(zap.String("component", name))
}

// NewLogger creates a new logger with dual output (console + file)
func NewLogger(cfg *Config) *zap.Logger {
	// Use configured time format or default
	timeFmt := cfg.TimeFormat
	if timeFmt == "" {
		timeFmt = "2006-01-02 15:04:05"
	}

	var cores []zapcore.Core

	consoleCore := &prettyCore{
		level: cfg.Level,
		out:   zapcore.Lock(os.Stderr),
	}

	cores = append(cores, consoleCore)

	// File output (structured JSON with rotation)
	if cfg.FilePath != "" {
		lumberjackLogger := &lumberjack.Logger{
			Filename:   cfg.FilePath,
			MaxSize:    10, // megabytes
			MaxBackups: 3,
			MaxAge:     15, // days
			Compress:   true,
		}
		fileEncoderConfig := zap.NewProductionEncoderConfig()
		fileEncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		fileCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(fileEncoderConfig),
			zapcore.AddSync(lumberjackLogger),
			zap.NewAtomicLevelAt(cfg.Level),
		)
		cores = append(cores, fileCore)
	}

	return zap.New(
		zapcore.NewTee(cores...),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
}

// DefaultLogger returns the singleton logger instance
func DefaultLogger() *zap.Logger {
	defaultLoggerOnce.Do(func() {
		defaultLogger = NewLogger(conf)
	})
	return defaultLogger
}

// WithContext stores a logger in the context
func WithContext(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext retrieves a logger from context, or returns default
func FromContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return DefaultLogger()
	}
	if logger, ok := ctx.Value(loggerKey).(*zap.Logger); ok {
		return logger
	}
	return DefaultLogger()
}

// WithRequest creates a logger with request context fields
func WithRequest(ctx context.Context, requestID, userID, endpoint string) *zap.Logger {
	return FromContext(ctx).With(
		zap.String("request_id", requestID),
		zap.String("user_id", userID),
		zap.String("endpoint", endpoint),
	)
}
