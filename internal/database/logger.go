package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/logging"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

type Logger struct {
	cfg    glogger.Config
	lg     *zap.Logger
	logCfg *config.DBLoggingConfig
}

func NewLogger(lg *zap.Logger, slowThreshold time.Duration, ignoreRecordNotFoundError bool, level zapcore.Level, logCfg *config.DBLoggingConfig) *Logger {
	cfg := glogger.Config{
		SlowThreshold:             slowThreshold,
		Colorful:                  false,
		IgnoreRecordNotFoundError: ignoreRecordNotFoundError,
	}

	// Map zap level to gorm level - be conservative in production
	switch level {
	case zapcore.DebugLevel:
		cfg.LogLevel = glogger.Info
	case zapcore.InfoLevel:
		cfg.LogLevel = glogger.Warn // Production: only warnings and errors
	case zapcore.WarnLevel:
		cfg.LogLevel = glogger.Warn
	case zapcore.ErrorLevel:
		cfg.LogLevel = glogger.Error
	default:
		cfg.LogLevel = glogger.Silent
	}

	// Use component logger instead of raw logger with prefix
	return &Logger{
		cfg:    cfg,
		lg:     logging.Component("DB").WithOptions(zap.AddCallerSkip(3)),
		logCfg: logCfg,
	}
}

func (l *Logger) LogMode(level glogger.LogLevel) glogger.Interface {
	newlogger := *l
	newlogger.cfg.LogLevel = level
	return &newlogger
}

func (l *Logger) Info(ctx context.Context, msg string, args ...any) {
	if l.cfg.LogLevel >= glogger.Info {
		l.lg.Info("db.info", zap.String("message", fmt.Sprintf(msg, args...)))
	}
}

func (l *Logger) Warn(ctx context.Context, msg string, args ...any) {
	if l.cfg.LogLevel >= glogger.Warn {
		l.lg.Warn("db.warning", zap.String("message", fmt.Sprintf(msg, args...)))
	}
}

func (l *Logger) Error(ctx context.Context, msg string, args ...any) {
	if l.cfg.LogLevel >= glogger.Error {
		l.lg.Error("db.error", zap.String("message", fmt.Sprintf(msg, args...)))
	}
}

func (l *Logger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.cfg.LogLevel <= glogger.Silent {
		return
	}

	elapsed := min(max(time.Since(begin), 0), time.Hour)
	sql, rows := fc()

	// Calculate duration in milliseconds with overflow protection
	durationMs := max(elapsed.Milliseconds(), 0)

	// Calculate threshold and excess in milliseconds
	thresholdMs := max(l.cfg.SlowThreshold.Milliseconds(), 0)
	excessMs := max(durationMs-thresholdMs, 0)

	fields := []zap.Field{
		zap.Int64("duration_ms", durationMs),
	}

	if rows != -1 {
		fields = append(fields, zap.Int64("rows_affected", rows))
	}

	if l.logCfg != nil && l.logCfg.LogSQL {
		fields = append(fields, zap.String("sql", sql))
	}

	switch {
	case err != nil && l.cfg.LogLevel >= glogger.Error && (!errors.Is(err, gorm.ErrRecordNotFound) || !l.cfg.IgnoreRecordNotFoundError):
		l.lg.Error("db.query_failed", append(fields, zap.Error(err))...)

	case elapsed > l.cfg.SlowThreshold && l.cfg.SlowThreshold != 0 && l.cfg.LogLevel >= glogger.Warn:
		fields = append(fields,
			zap.Int64("threshold_ms", thresholdMs),
			zap.Int64("excess_ms", excessMs),
		)
		l.lg.Warn("db.slow_query", fields...)

	case l.cfg.LogLevel == glogger.Info:
		// Only log successful queries in debug mode
		l.lg.Debug("db.query", fields...)
	}
}
