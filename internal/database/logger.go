package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/divyam234/teldrive/pkg/logging"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
)

const msgPrefix = "[DB] "

type Logger struct {
	cfg glogger.Config
}

// NewLogger returns a new logger for gorm. *zap.SugaredLogger will use from context.Context.
func NewLogger(slowThreshold time.Duration, ignoreRecordNotFoundError bool, level zapcore.Level) *Logger {
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
	return &Logger{cfg: cfg}
}

func (l *Logger) LogMode(level glogger.LogLevel) glogger.Interface {
	newlogger := *l
	newlogger.cfg.LogLevel = level
	return &newlogger
}

func (l *Logger) Info(ctx context.Context, s string, i ...interface{}) {
	if l.cfg.LogLevel >= glogger.Info {
		l.fromContext(ctx).Infof(msgPrefix+s, i)
	}
}

func (l *Logger) Warn(ctx context.Context, s string, i ...interface{}) {
	if l.cfg.LogLevel >= glogger.Warn {
		l.fromContext(ctx).Warnf(msgPrefix+s, i)
	}
}

func (l *Logger) Error(ctx context.Context, s string, i ...interface{}) {
	if l.cfg.LogLevel >= glogger.Error {
		l.fromContext(ctx).Errorf(msgPrefix+s, i)
	}
}

func (l *Logger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.cfg.LogLevel <= glogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	logger := l.fromContext(ctx)

	const (
		traceStr     = msgPrefix + "%s\n[%.3fms] [rows:%v] %s"
		traceWarnStr = msgPrefix + "%s %s\n[%.3fms] [rows:%v] %s"
		traceErrStr  = msgPrefix + "%s %s\n[%.3fms] [rows:%v] %s"
	)

	switch {
	case err != nil && l.cfg.LogLevel >= glogger.Error && (!errors.Is(err, gorm.ErrRecordNotFound) || !l.cfg.IgnoreRecordNotFoundError):
		sql, rows := fc()
		if rows == -1 {
			logger.Errorf(traceErrStr, utils.FileWithLineNum(), err, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			logger.Errorf(traceErrStr, utils.FileWithLineNum(), err, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	case elapsed > l.cfg.SlowThreshold && l.cfg.SlowThreshold != 0 && l.cfg.LogLevel >= glogger.Warn:
		sql, rows := fc()
		slowLog := fmt.Sprintf("SLOW SQL >= %v", l.cfg.SlowThreshold)
		if rows == -1 {
			logger.Warnf(traceWarnStr, utils.FileWithLineNum(), slowLog, float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			logger.Warnf(traceWarnStr, utils.FileWithLineNum(), slowLog, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	case l.cfg.LogLevel == glogger.Info:
		sql, rows := fc()
		if rows == -1 {
			logger.Infof(traceStr, utils.FileWithLineNum(), float64(elapsed.Nanoseconds())/1e6, "-", sql)
		} else {
			logger.Infof(traceStr, utils.FileWithLineNum(), float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	}
}

func (l *Logger) fromContext(ctx context.Context) *zap.SugaredLogger {
	return logging.FromContext(ctx).WithOptions(zap.AddCallerSkip(3))
}
