//go:build integration

package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/database"
)

const testDatabaseEnv = "TEST_DATABASE_URL"

type Database struct {
	Pool *pgxpool.Pool
	URL  string
}

// New creates a dedicated PostgreSQL database, applies all embedded migrations,
// and returns a verified pool. The database is force-dropped during cleanup.
func New(t testing.TB) *Database {
	t.Helper()

	baseURL := os.Getenv(testDatabaseEnv)
	if baseURL == "" {
		t.Fatalf("%s is required; run tests through scripts/test-postgres.sh", testDatabaseEnv)
	}

	adminURL, err := withDatabase(baseURL, "postgres")
	if err != nil {
		t.Fatalf("build admin database URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL admin database: %v", err)
	}

	databaseName := "teldrive_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	identifier := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		admin.Close(ctx)
		t.Fatalf("create test database: %v", err)
	}

	targetURL, err := withDatabase(baseURL, databaseName)
	if err != nil {
		admin.Close(ctx)
		t.Fatalf("build test database URL: %v", err)
	}
	if err := database.Migrate(ctx, database.Config{URL: targetURL}); err != nil {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+identifier+" WITH (FORCE)")
		admin.Close(context.Background())
		t.Fatalf("migrate test database: %v", err)
	}

	pool, err := database.Open(ctx, database.Config{
		URL:             targetURL,
		ApplicationName: "teldrive-integration-test",
		MaxConnections:  8,
		ConnectTimeout:  10 * time.Second,
	})
	if err != nil {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+identifier+" WITH (FORCE)")
		admin.Close(context.Background())
		t.Fatalf("open test database: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database %s: %v", databaseName, err)
		}
		if err := admin.Close(cleanupCtx); err != nil {
			t.Errorf("close admin database connection: %v", err)
		}
	})

	return &Database{Pool: pool, URL: targetURL}
}

func withDatabase(rawURL, databaseName string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse database URL: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("unsupported database URL scheme %q", u.Scheme)
	}
	u.Path = "/" + databaseName
	return u.String(), nil
}
