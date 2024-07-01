package database

import (
	"time"

	"github.com/divyam234/teldrive/internal/config"
	"github.com/divyam234/teldrive/internal/logging"

	extraClausePlugin "github.com/WinterYukky/gorm-extra-clause-plugin"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// NewDatabase creates a new database with given config
func NewDatabase(cfg *config.Config) (*gorm.DB, error) {
	var (
		db     *gorm.DB
		err    error
		logger = NewLogger(time.Second, true, zapcore.Level(cfg.DB.LogLevel))
	)
	for i := 0; i <= 5; i++ {
		db, err = gorm.Open(postgres.New(postgres.Config{
			DSN:                  cfg.DB.DataSource,
			PreferSimpleProtocol: !cfg.DB.PrepareStmt,
		}), &gorm.Config{
			Logger: logger,
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
		logging.DefaultLogger().Warnf("failed to open database: %v", err)
		time.Sleep(500 * time.Millisecond)
	}

	if err != nil {
		logging.DefaultLogger().Fatalf("database: %v", err)
	}

	db.Use(extraClausePlugin.New())

	if cfg.DB.Pool.Enable {
		rawDB, err := db.DB()
		if err != nil {
			return nil, err
		}
		rawDB.SetMaxOpenConns(cfg.DB.Pool.MaxOpenConnections)
		rawDB.SetMaxIdleConns(cfg.DB.Pool.MaxIdleConnections)
		rawDB.SetConnMaxLifetime(cfg.DB.Pool.MaxLifetime)
	}

	sqlDb, _ := db.DB()
	err = migrateDB(sqlDb)
	if err != nil {
		panic(err)
	}

	return db, nil
}
