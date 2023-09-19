package database

import (
	"embed"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/divyam234/teldrive/utils"
	"github.com/divyam234/teldrive/utils/kv"
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
var BoltDB *bbolt.DB
var KV kv.KV

func InitDB() {

	var err error

	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Silent,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
			Colorful:                  false,
		},
	)

	DB, err = gorm.Open(postgres.Open(utils.GetConfig().DatabaseUrl), &gorm.Config{
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
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	sqlDB.SetConnMaxLifetime(time.Hour)
	go func() {
		DB.Exec(`create collation if not exists numeric (provider = icu, locale = 'en@colnumeric=yes');`)
		if utils.GetConfig().RunMigrations {
			migrate()
		}
	}()

	config := utils.GetConfig()
	BoltDB, err = bbolt.Open(filepath.Join(config.ExecDir, "teldrive.db"), 0666, &bbolt.Options{
		Timeout:    time.Second,
		NoGrowSync: false,
	})
	if err != nil {
		panic(err)
	}
	KV, err = kv.New(kv.Options{Bucket: "teldrive", DB: BoltDB})

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
