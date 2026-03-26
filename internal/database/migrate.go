package database

import (
	"context"
	"embed"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

func NewTestDatabase(tb testing.TB, migration bool) *pgxpool.Pool {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		tb.Fatalf("failed to init db %v", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		tb.Fatalf("failed to ping db %v", err)
	}

	if migration {
		if err := MigrateDB(pool, true); err != nil {
			tb.Fatalf("migration failed %v", err)
		}
	}

	return pool

}
func MigrateDB(pool *pgxpool.Pool, migrateRiver bool) error {
	ctx := context.Background()
	std := stdlib.OpenDBFromPool(pool)
	defer std.Close()

	goose.SetBaseFS(embedMigrations)
	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.Up(std, "migrations"); err != nil {
		return err
	}

	if migrateRiver {
		migrator, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{Schema: "teldrive"})
		if err != nil {
			return err
		}
		if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
			return err
		}
	}

	return nil
}
