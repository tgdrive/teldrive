package logger

import (
	"os"
	"time"

	"github.com/divyam234/teldrive/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

func InitLogger() *zap.Logger {
	customTimeEncoder := func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("02/01/2006 03:04 PM"))
	}
	var (
		consoleConfig zapcore.EncoderConfig
		logLevel      zapcore.Level
	)

	if config.GetConfig().Dev {
		consoleConfig = zap.NewDevelopmentEncoderConfig()
		logLevel = zap.DebugLevel
	} else {

		consoleConfig = zap.NewProductionEncoderConfig()
		logLevel = zap.InfoLevel
	}
	consoleConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleConfig.EncodeTime = customTimeEncoder
	consoleEncoder := zapcore.NewConsoleEncoder(consoleConfig)

	fileEncoderConfig := zap.NewProductionEncoderConfig()
	fileEncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(fileEncoderConfig)

	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   "logs/teldrive.log",
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     7,
		Compress:   true,
	})

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), logLevel),
		zapcore.NewCore(fileEncoder, fileWriter, zapcore.DebugLevel),
	)

	return zap.New(core, zap.AddStacktrace(zapcore.FatalLevel))
}
