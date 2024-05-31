package database

import (
	"time"

	"github.com/divyam234/teldrive/internal/config"
	"github.com/divyam234/teldrive/pkg/logging"

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

	for i := 0; i <= 10; i++ {
		db, err = gorm.Open(postgres.Open(cfg.DB.DataSource), &gorm.Config{
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
	db.Use(extraClausePlugin.New())
	if err != nil {
		logging.DefaultLogger().Fatalf("database: %v", err)
	}

	rawDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	rawDB.SetMaxOpenConns(cfg.DB.Pool.MaxOpenConnections)
	rawDB.SetMaxIdleConns(cfg.DB.Pool.MaxIdleConnections)
	rawDB.SetConnMaxLifetime(cfg.DB.Pool.MaxLifetime)

	if cfg.DB.Migrate.Enable {
		err := migrateDB(rawDB)
		if err != nil {
			return nil, err
		}
	}
	return db, nil
}
