package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
)

const msgPrefix = "[DB] "

type Logger struct {
	cfg glogger.Config
	lg  *zap.Logger
}

func NewLogger(lg *zap.Logger, slowThreshold time.Duration, ignoreRecordNotFoundError bool, level zapcore.Level) *Logger {
	cfg := glogger.Config{
		SlowThreshold:             slowThreshold,
		Colorful:                  false,
		IgnoreRecordNotFoundError: ignoreRecordNotFoundError,
	}
	switch level {
	case zapcore.DebugLevel, zapcore.InfoLevel:
		cfg.LogLevel = glogger.Info
	case zapcore.WarnLevel:
		cfg.LogLevel = glogger.Warn
	case zapcore.ErrorLevel:
		cfg.LogLevel = glogger.Error
	default:
		cfg.LogLevel = glogger.Silent
	}
	return &Logger{cfg: cfg, lg: lg.WithOptions(zap.AddCallerSkip(3))}
}

func (l *Logger) LogMode(level glogger.LogLevel) glogger.Interface {
	newlogger := *l
	newlogger.cfg.LogLevel = level
	return &newlogger
}

func (l *Logger) Info(ctx context.Context, s string, i ...any) {
	if l.cfg.LogLevel >= glogger.Info {
		l.lg.Info(fmt.Sprintf(msgPrefix+s, i...))
	}
}

func (l *Logger) Warn(ctx context.Context, s string, i ...any) {
	if l.cfg.LogLevel >= glogger.Warn {
		l.lg.Warn(fmt.Sprintf(msgPrefix+s, i...))
	}
}

func (l *Logger) Error(ctx context.Context, s string, i ...any) {
	if l.cfg.LogLevel >= glogger.Error {
		l.lg.Error(fmt.Sprintf(msgPrefix+s, i...))
	}
}

func (l *Logger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.cfg.LogLevel <= glogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	duration := float64(elapsed.Nanoseconds()) / 1e6

	sql, rows := fc()

	fields := []zap.Field{
		zap.String("source", utils.FileWithLineNum()),
		zap.Float64("duration_ms", duration),
		zap.String("sql", sql),
	}
	if rows != -1 {
		fields = append(fields, zap.Int64("rows", rows))
	}

	switch {
	case err != nil && l.cfg.LogLevel >= glogger.Error && (!errors.Is(err, gorm.ErrRecordNotFound) || !l.cfg.IgnoreRecordNotFoundError):
		l.lg.Error("trace error", append(fields, zap.Error(err))...)
	case elapsed > l.cfg.SlowThreshold && l.cfg.SlowThreshold != 0 && l.cfg.LogLevel >= glogger.Warn:
		l.lg.Warn("slow sql", append(fields, zap.Duration("threshold", l.cfg.SlowThreshold))...)
	case l.cfg.LogLevel == glogger.Info:
		l.lg.Info("trace", fields...)
	}
}
