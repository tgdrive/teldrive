package database

import (
	"context"
	"strings"

	jetpostgres "github.com/go-jet/jet/v2/postgres"
	"github.com/tgdrive/teldrive/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func configureJetLogger(cfg *config.DBLoggingConfig, logger *zap.Logger) {
	if cfg == nil || !cfg.LogSQL {
		jetpostgres.SetQueryLogger(nil)
		return
	}

	jetpostgres.SetQueryLogger(newJetQueryLogger(cfg, logger))
}

func newJetQueryLogger(cfg *config.DBLoggingConfig, logger *zap.Logger) func(context.Context, jetpostgres.QueryInfo) {
	level := parseDBLogLevel(cfg.Level)

	return func(ctx context.Context, info jetpostgres.QueryInfo) {
		if logger == nil {
			return
		}

		query := strings.TrimSpace(info.Statement.DebugSql())
		fields := []zap.Field{
			zap.String("query", query),
			zap.Duration("duration", info.Duration),
			zap.Int64("rows_processed", info.RowsProcessed),
		}

		if file, line, function := info.Caller(); file != "" {
			fields = append(fields,
				zap.String("caller_file", file),
				zap.Int("caller_line", line),
				zap.String("caller_function", function),
			)
		}

		if info.Err != nil {
			logger.Error("db.query.failed", append(fields, zap.Error(info.Err))...)
			return
		}

		if cfg.SlowThreshold > 0 && info.Duration >= cfg.SlowThreshold {
			logger.Warn("db.query.slow", fields...)
			return
		}

		switch level {
		case zapcore.DebugLevel:
			logger.Debug("db.query", fields...)
		case zapcore.InfoLevel:
			logger.Info("db.query", fields...)
		case zapcore.WarnLevel:
			logger.Warn("db.query", fields...)
		case zapcore.ErrorLevel:
			logger.Error("db.query", fields...)
		default:
			logger.Debug("db.query", fields...)
		}
	}
}

func parseDBLogLevel(level string) zapcore.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.ErrorLevel
	}
}
