package logging

import (
	"context"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type contextKey = string

const loggerKey = contextKey("logger")

var (
	defaultLogger     *zap.Logger
	defaultLoggerOnce sync.Once
)

var conf = &Config{
	Level: zapcore.InfoLevel,
}

type Config struct {
	Level    zapcore.Level
	FilePath string
}

func SetConfig(c *Config) {
	conf = &Config{
		Level:    c.Level,
		FilePath: c.FilePath,
	}
}

func SetLevel(l zapcore.Level) {
	conf.Level = l
}

func NewLogger(conf *Config) *zap.Logger {

	ec := zap.NewProductionEncoderConfig()
	ec.EncodeTime = zapcore.ISO8601TimeEncoder
	ec.EncodeLevel = zapcore.CapitalColorLevelEncoder
	ec.CallerKey = ""
	ec.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("02/01/2006 03:04 PM"))
	}

	var cores []zapcore.Core

	cores = append(cores, zapcore.NewCore(zapcore.NewConsoleEncoder(ec),
		zapcore.AddSync(os.Stdout), zap.NewAtomicLevelAt(conf.Level)))

	if conf.FilePath != "" {
		lumberjackLogger := &lumberjack.Logger{
			Filename:   conf.FilePath,
			MaxSize:    10,
			MaxBackups: 3,
			MaxAge:     15,
			Compress:   true,
		}
		cores = append(cores, zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(lumberjackLogger), zap.NewAtomicLevelAt(conf.Level)))
	}

	return zap.New(zapcore.NewTee(cores...))
}

func DefaultLogger() *zap.Logger {
	defaultLoggerOnce.Do(func() {
		defaultLogger = NewLogger(conf)
	})
	return defaultLogger
}

func WithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func FromContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return DefaultLogger()
	}
	if logger, ok := ctx.Value(loggerKey).(*zap.Logger); ok {
		return logger
	}

	return DefaultLogger()
}
