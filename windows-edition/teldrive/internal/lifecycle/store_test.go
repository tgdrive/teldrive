package lifecycle

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestLifecyclePostgres(t *testing.T) {
	dsn := os.Getenv("TELDRIVE_LIFECYCLE_TEST_DSN")
	if dsn == "" {
		t.Skip("set isolated PostgreSQL test DSN to run")
	}
	if !strings.Contains(dsn, "/teldrive_lifecycle_test") {
		t.Fatal("requires isolated teldrive_lifecycle_test database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.Exec(`CREATE SCHEMA IF NOT EXISTS teldrive;
	 DROP TABLE IF EXISTS teldrive.files;
	 CREATE TABLE teldrive.files(id uuid PRIMARY KEY, name text NOT NULL, type text NOT NULL, mime_type text, size bigint, user_id bigint NOT NULL, parent_id uuid, status text DEFAULT 'active', lifecycle_root uuid, lifecycle_at timestamptz, updated_at timestamptz DEFAULT NOW());
	 CREATE UNIQUE INDEX test_unique_active ON teldrive.files(user_id,parent_id,name) WHERE status = 'active';`).Error; err != nil {
		t.Fatal(err)
	}
	root := "00000000-0000-4000-8000-000000000001"
	folder := "00000000-0000-4000-8000-000000000002"
	file := "00000000-0000-4000-8000-000000000003"
	other := "00000000-0000-4000-8000-000000000004"
	ctx := context.Background()
	if err := db.Exec(`INSERT INTO teldrive.files(id,name,type,user_id,parent_id) VALUES (?, 'root','folder',1,NULL),(?, 'Fotos','folder',1,?),(?, 'foto.png','file',1,?),(?, 'ajeno','file',2,?)`, root, folder, root, file, folder, other, root).Error; err != nil {
		t.Fatal(err)
	}
	if err := Change(ctx, db, 1, []string{folder, file, folder}, "trash", ""); err != nil {
		t.Fatal(err)
	}
	items, err := List(ctx, db, 1, "trash")
	if err != nil || len(items) != 1 || items[0].ID != folder {
		t.Fatalf("trash root listing: %v %v", items, err)
	}
	var count int64
	db.Table("teldrive.files").Where("status = 'trash'").Count(&count)
	if count != 2 {
		t.Fatal("folder descendants were not moved")
	}
	if err := Change(ctx, db, 2, []string{folder}, "restore", "trash"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-account restore: %v", err)
	}
	if err := Change(ctx, db, 1, []string{folder}, "restore", "trash"); err != nil {
		t.Fatal(err)
	}
	db.Table("teldrive.files").Where("status = 'trash'").Count(&count)
	if count != 0 {
		t.Fatal("restore did not restore descendants")
	}
	if err := Change(ctx, db, 1, []string{root}, "trash", ""); !errors.Is(err, ErrInvalid) {
		t.Fatal("account root must be protected")
	}
	if err := Change(ctx, db, 1, []string{file, other}, "spam", ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("mixed-account batch must fail atomically")
	}
	db.Table("teldrive.files").Where("status = 'spam'").Count(&count)
	if count != 0 {
		t.Fatal("failed batch was not rolled back")
	}
	if err := Change(ctx, db, 1, []string{file}, "spam", ""); err != nil {
		t.Fatal(err)
	}
	// Restore when the original parent is inactive: place the item in Mi unidad.
	db.Exec("UPDATE teldrive.files SET status = 'trash' WHERE id = ?", folder)
	if err := Change(ctx, db, 1, []string{file}, "restore", "spam"); err != nil {
		t.Fatal(err)
	}
	var restored Item
	db.Table("teldrive.files").Where("id = ?", file).First(&restored)
	if restored.ParentID == nil || *restored.ParentID != root {
		t.Fatal("restore must fall back to the account root")
	}
	if err := Change(ctx, db, 1, []string{file}, "trash", ""); err != nil {
		t.Fatal(err)
	}
	if err := Expire(ctx, db, time.Now().Add(29*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	items, _ = List(ctx, db, 1, "trash")
	if len(items) != 1 {
		t.Fatal("must retain before 30 days")
	}
	if err := Expire(ctx, db, time.Now().Add(31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var status string
	db.Table("teldrive.files").Select("status").Where("id = ?", file).Scan(&status)
	if status != "pending_deletion" {
		t.Fatal("expiration must queue Telegram cleanup")
	}
	if err := Change(ctx, db, 1, []string{file}, "restore", "trash"); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired file must not be restorable")
	}
}
