package sqlcgen

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const schemaTemplateMarker = "/* TEMPLATE: schema */"

var (
	schemaNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	configuredSchema  atomic.Value
)

func init() {
	configuredSchema.Store("teldrive")
}

// ConfigureSchema sets the explicit PostgreSQL schema rendered into generated queries.
func ConfigureSchema(schema string) error {
	schema = strings.TrimSpace(schema)
	if !schemaNamePattern.MatchString(schema) {
		return fmt.Errorf("invalid database schema %q", schema)
	}
	configuredSchema.Store(schema)
	return nil
}

// QualifiedName returns a safely quoted object name in the configured schema.
func QualifiedName(name string) string {
	return pgx.Identifier{configuredSchema.Load().(string), name}.Sanitize()
}

func wrapDBTX(db DBTX) DBTX {
	if _, ok := db.(*schemaDBTX); ok {
		return db
	}
	return &schemaDBTX{DBTX: db}
}

type schemaDBTX struct {
	DBTX
}

func renderSchema(query string) string {
	schema := configuredSchema.Load().(string)
	return strings.ReplaceAll(query, schemaTemplateMarker, pgx.Identifier{schema}.Sanitize()+".")
}

func (db *schemaDBTX) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	return db.DBTX.Exec(ctx, renderSchema(query), args...)
}

func (db *schemaDBTX) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	return db.DBTX.Query(ctx, renderSchema(query), args...)
}

func (db *schemaDBTX) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	return db.DBTX.QueryRow(ctx, renderSchema(query), args...)
}
