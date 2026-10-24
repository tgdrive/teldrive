//go:build integration

package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tgdrive/teldrive/v2/db/migrations"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestPreferencesDocumentCleanStart(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, `INSERT INTO users(user_id) VALUES(1001); INSERT INTO user_preferences(user_id,preferences) VALUES(1001,'{"appearance":{"theme":"dark"}}')`); err != nil {
		t.Fatal(err)
	}
	data, err := migrations.Files.ReadFile("000014_preferences_document.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(strings.Split(string(data), "-- +goose Up")[1], "-- +goose Down")[0]
	if _, err := db.Pool.Exec(ctx, strings.ReplaceAll(up, "/* TEMPLATE: schema */", "")); err != nil {
		t.Fatal(err)
	}
	var preferences, users int
	if err := db.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM user_preferences), (SELECT count(*) FROM users WHERE user_id=1001)`).Scan(&preferences, &users); err != nil || preferences != 0 || users != 1 {
		t.Fatalf("reset scope: preferences=%d users=%d err=%v", preferences, users, err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO user_preferences(user_id) VALUES(1001); UPDATE user_preferences SET preferences='{"newSetting":{"value":true}}'`); err != nil {
		t.Fatal(err)
	}
	var value bool
	if err := db.Pool.QueryRow(ctx, `SELECT (preferences #>> '{newSetting,value}')::boolean FROM user_preferences`).Scan(&value); err != nil || !value {
		t.Fatalf("new fields require no migration: %v %v", value, err)
	}
}
