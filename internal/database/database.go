package database

import (
	"time"

	"github.com/tgdrive/teldrive/internal/config"

	extraClausePlugin "github.com/WinterYukky/gorm-extra-clause-plugin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func NewDatabase(cfg *config.DBConfig, lg *zap.SugaredLogger) (*gorm.DB, error) {
	level, err := zapcore.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zapcore.InfoLevel
	}

	var db *gorm.DB

	for i := 0; i <= 5; i++ {
		db, err = gorm.Open(postgres.New(postgres.Config{
			DSN:                  cfg.DataSource,
			PreferSimpleProtocol: !cfg.PrepareStmt,
		}), &gorm.Config{
			Logger: NewLogger(lg, time.Second, true, level),
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
		lg.Warnf("failed to open database: %v", err)
		time.Sleep(500 * time.Millisecond)
	}

	if err != nil {
		lg.Fatalf("database: %v", err)
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
