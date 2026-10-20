package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/pressly/goose/v3"
)

const schemaTemplateMarker = "/* TEMPLATE: schema */"

var schemaNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Files contains every versioned TelDrive SQL migration.
//
//go:embed *.sql
var Files embed.FS

// Up renders the configured schema into every migration and applies all pending
// versions with explicit object qualification.
func Up(ctx context.Context, db *sql.DB, schema string) error {
	provider, err := newProvider(db, schema)
	if err != nil {
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// Down rolls back exactly the most recently applied application migration.
// River and RiverPro migrations are managed separately and are not rolled back.
func Down(ctx context.Context, db *sql.DB, schema string) error {
	provider, err := newProvider(db, schema)
	if err != nil {
		return err
	}
	if _, err := provider.Down(ctx); err != nil {
		return fmt.Errorf("roll back migration: %w", err)
	}
	return nil
}

func newProvider(db *sql.DB, schema string) (*goose.Provider, error) {
	schema = strings.TrimSpace(schema)
	rendered, err := renderedFiles(schema)
	if err != nil {
		return nil, err
	}
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		rendered,
		goose.WithTableName(schema+".migrations"),
	)
	if err != nil {
		return nil, fmt.Errorf("create migration provider: %w", err)
	}
	return provider, nil
}

func renderedFiles(schema string) (fs.FS, error) {
	schema = strings.TrimSpace(schema)
	if !schemaNamePattern.MatchString(schema) {
		return nil, fmt.Errorf("invalid database schema %q", schema)
	}
	prefix := pgx.Identifier{schema}.Sanitize() + "."
	result := fstest.MapFS{}
	entries, err := fs.ReadDir(Files, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := Files.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		result[entry.Name()] = &fstest.MapFile{
			Data: []byte(strings.ReplaceAll(string(data), schemaTemplateMarker, prefix)),
		}
	}
	return result, nil
}
