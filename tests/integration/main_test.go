package integration

import (
	"log"
	"os"
	"testing"
	"time"

	extraClausePlugin "github.com/WinterYukky/gorm-extra-clause-plugin"
	"github.com/tgdrive/teldrive/internal/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	dsn := os.Getenv("TELDRIVE_DB_DATASOURCE")
	var err error
	testDB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   "teldrive.",
			SingularTable: false,
		},
		PrepareStmt: true,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}

	testDB.Use(extraClausePlugin.New())

	// Clean DB schema to ensure migrations run from scratch
	if err := testDB.Exec("DROP SCHEMA IF EXISTS teldrive CASCADE").Error; err != nil {
		log.Fatalf("failed to clean db: %v", err)
	}
	if err := testDB.Exec("DROP TABLE IF EXISTS goose_db_version").Error; err != nil {
		log.Fatalf("failed to clean goose table: %v", err)
	}

	if err := database.MigrateDB(testDB); err != nil {
		log.Fatalf("failed to migrate db: %v", err)
	}

	// Clean before start
	truncateTables(testDB)

	code := m.Run()

	// Clean after finish (optional)
	// truncateTables(testDB)

	os.Exit(code)
}

func truncateTables(db *gorm.DB) {
	tables := []string{
		"teldrive.files",
		"teldrive.users",
		"teldrive.sessions",
		"teldrive.channels",
		"teldrive.bots",
		"teldrive.uploads",
		"teldrive.file_shares",
	}
	for _, table := range tables {
		if err := db.Exec("TRUNCATE TABLE " + table + " CASCADE").Error; err != nil {
			log.Printf("failed to truncate %s: %v", table, err)
		}
	}
}
