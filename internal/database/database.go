package database

import (
	"context"
	"time"

	extraClausePlugin "github.com/WinterYukky/gorm-extra-clause-plugin"
	"github.com/tgdrive/teldrive/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func NewDatabase(ctx context.Context, cfg *config.DBConfig, logCfg *config.DBLoggingConfig, lg *zap.Logger) (*gorm.DB, error) {
	level, err := zapcore.ParseLevel(logCfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	var db *gorm.DB

	for i := 0; i <= 5; i++ {
		db, err = gorm.Open(postgres.New(postgres.Config{
			DSN:                  cfg.DataSource,
			PreferSimpleProtocol: !cfg.PrepareStmt,
		}), &gorm.Config{
			Logger: NewLogger(lg, logCfg.SlowThreshold, logCfg.IgnoreRecordNotFound, level, logCfg),
			NamingStrategy: schema.NamingStrategy{
				TablePrefix:   "teldrive.",
				SingularTable: false,
			},
			NowFunc: func() time.Time {
				return time.Now().UTC()
			},
		})
		if err == nil {
			break
		}
		lg.Warn("failed to open database", zap.Error(err))
		if i < 5 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
	if err != nil {
		lg.Fatal("database", zap.Error(err))
	}

	db.Use(extraClausePlugin.New())

	if cfg.Pool.Enable {
		rawDB, err := db.DB()
		if err != nil {
			return nil, err
		}
		rawDB.SetMaxOpenConns(cfg.Pool.MaxOpenConnections)
		rawDB.SetMaxIdleConns(cfg.Pool.MaxIdleConnections)
		rawDB.SetConnMaxLifetime(cfg.Pool.MaxLifetime)
	}

	return db, nil
}
