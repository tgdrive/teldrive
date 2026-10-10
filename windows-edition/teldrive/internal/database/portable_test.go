package database

import (
	"net/url"
	"os"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMigrationsOnStandardPostgres(t *testing.T) {
	dsn := os.Getenv("TELDRIVE_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set an isolated migration-test database DSN")
	}
	u, e := url.Parse(dsn)
	if e != nil || !strings.HasSuffix(u.Path, "_migration_test") {
		t.Fatal("requires an isolated *_migration_test database")
	}
	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	sqlDB, e := db.DB()
	if e != nil {
		t.Fatal(e)
	}
	defer sqlDB.Close()
	if e = MigrateDB(db); e != nil {
		t.Fatal(e)
	}
	value, ok := db.Get("teldrive.pgroonga")
	if !ok || value != false {
		t.Fatalf("standard PostgreSQL should select portable search, got %v", value)
	}
	var found bool
	if e = db.Raw("SELECT position(teldrive.clean_name(?) in teldrive.clean_name(?)) > 0", "informe", "Informe_Final.pdf").Scan(&found).Error; e != nil || !found {
		t.Fatalf("portable text search %v %v", found, e)
	}
	if e = db.Raw("SELECT ? ~ ?", "Informe.pdf", "(?i)informe.*").Scan(&found).Error; e != nil || !found {
		t.Fatalf("portable regex search %v %v", found, e)
	}
	if e = MigrateDB(db); e != nil {
		t.Fatalf("repeat migration: %v", e)
	}
}
