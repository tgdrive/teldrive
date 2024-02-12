package logging

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type contextKey = string

const loggerKey = contextKey("logger")

var (
	defaultLogger     *zap.SugaredLogger
	defaultLoggerOnce sync.Once
)

var conf = &Config{
	Level:       zapcore.InfoLevel,
	Development: true,
}

type Config struct {
	Level       zapcore.Level
	Development bool
	FilePath    string
}

func SetConfig(c *Config) {
	conf = &Config{
		Level:       c.Level,
		Development: c.Development,
		FilePath:    c.FilePath,
	}
}

func SetLevel(l zapcore.Level) {
	conf.Level = l
}

func NewLogger(conf *Config) *zap.SugaredLogger {

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

	options := []zap.Option{}
	if conf.Development {
		options = append(options, zap.Development())

	}
	return zap.New(zapcore.NewTee(cores...), options...).Sugar()
}

func DefaultLogger() *zap.SugaredLogger {
	defaultLoggerOnce.Do(func() {
		defaultLogger = NewLogger(conf)
	})
	return defaultLogger
}

func WithLogger(ctx context.Context, logger *zap.SugaredLogger) context.Context {
	if gCtx, ok := ctx.(*gin.Context); ok {
		ctx = gCtx.Request.Context()
	}
	return context.WithValue(ctx, loggerKey, logger)
}

func FromContext(ctx context.Context) *zap.SugaredLogger {
	if ctx == nil {
		return DefaultLogger()
	}
	if gCtx, ok := ctx.(*gin.Context); ok && gCtx != nil {
		ctx = gCtx.Request.Context()
	}
	if logger, ok := ctx.Value(loggerKey).(*zap.SugaredLogger); ok {
		return logger
	}
	return DefaultLogger()
}
