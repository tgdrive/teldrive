package database

import (
	"embed"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

func NewTestDatabase(tb testing.TB, migration bool) *gorm.DB {
	url := ""
	db, err := gorm.Open(postgres.Open(url), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   "teldrive.",
			SingularTable: false,
		},
		PrepareStmt: true,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		tb.Fatalf("failed to init db %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		tb.Fatalf("failed to init db %v", err)
	}
	sqlDB.SetMaxIdleConns(25)
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	if migration {
		MigrateDB(db)
	}

	return db

}
func DeleteRecordAll(_ testing.TB, db *gorm.DB, tableWhereClauses []string) error {
	if len(tableWhereClauses)%2 != 0 {
		return errors.New("must exist table and where clause")
	}

	for i := 0; i < len(tableWhereClauses)-1; i += 2 {
		rowDB, err := db.DB()
		if err != nil {
			return err
		}
		query := fmt.Sprintf("DELETE FROM %s WHERE %s", tableWhereClauses[i], tableWhereClauses[i+1])
		_, err = rowDB.Exec(query)
		if err != nil {
			return err
		}
	}
	return nil
}

func MigrateDB(db *gorm.DB) error {
	sqlDb, _ := db.DB()
	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.Up(sqlDb, "migrations"); err != nil {
		return err
	}
	return nil
}
