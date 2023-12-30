package database

import (
	"embed"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/divyam234/teldrive/config"
	"github.com/divyam234/teldrive/internal/kv"
	"github.com/pressly/goose/v3"
	"go.etcd.io/bbolt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS
var DB *gorm.DB
var KV kv.KV

func InitDB() {

	var err error

	logLevel := logger.Silent

	if config.GetConfig().LogSql {
		logLevel = logger.Info

	}

	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logLevel,
			ParameterizedQueries:      true,
			Colorful:                  true,
			IgnoreRecordNotFoundError: true,
		},
	)

	DB, err = gorm.Open(postgres.Open(config.GetConfig().DatabaseUrl), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   "teldrive.",
			SingularTable: false,
		},
		PrepareStmt: false,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		Logger: newLogger,
	})
	if err != nil {
		panic(err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		panic(err)
	}
	sqlDB.SetMaxIdleConns(25)
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	go func() {
		if config.GetConfig().RunMigrations {
			migrate()
		}
	}()

	boltDB, err := bbolt.Open(filepath.Join(config.GetConfig().ExecDir, "teldrive.db"), 0666, &bbolt.Options{
		Timeout:    time.Second,
		NoGrowSync: false,
	})
	if err != nil {
		panic(err)
	}
	KV, err = kv.New(kv.Options{Bucket: "teldrive", DB: boltDB})

	if err != nil {
		panic(err)
	}
}

func migrate() {

	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("postgres"); err != nil {
		panic(err)
	}
	db, _ := DB.DB()
	if err := goose.Up(db, "migrations"); err != nil {
		panic(err)
	}
}
