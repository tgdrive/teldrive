package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	prodriver "github.com/divyam234/riverpro/driver"
	"github.com/divyam234/riverpro/driver/riverpropgxv5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/tgdrive/teldrive/v2/db/migrations"
)

const (
	defaultConnectTimeout = 10 * time.Second
	DefaultSchema         = "teldrive"
)

var schemaNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var ErrLegacySchema = errors.New("legacy TelDrive schema detected")

// Config controls the PostgreSQL connection pool used by TelDrive.
type Config struct {
	URL                 string        `koanf:"url" default:"" validate:"required" description:"PostgreSQL connection URL"`
	Schema              string        `koanf:"schema" default:"teldrive" validate:"required" description:"PostgreSQL schema for all TelDrive and River tables"`
	ApplicationName     string        `koanf:"application-name" default:"teldrive-v2" validate:"required" description:"PostgreSQL application name"`
	MaxConnections      int32         `koanf:"max-connections" default:"25" validate:"gte=1" description:"Maximum PostgreSQL connections"`
	MinConnections      int32         `koanf:"min-connections" default:"2" validate:"gte=0" description:"Minimum PostgreSQL connections"`
	MaxConnectionIdle   time.Duration `koanf:"max-connection-idle" default:"5m" validate:"gte=0" description:"Maximum idle time for a PostgreSQL connection"`
	MaxConnectionLife   time.Duration `koanf:"max-connection-life" default:"30m" validate:"gte=0" description:"Maximum lifetime for a PostgreSQL connection"`
	HealthCheckInterval time.Duration `koanf:"health-check-interval" default:"30s" validate:"gt=0" description:"PostgreSQL pool health-check interval"`
	ConnectTimeout      time.Duration `koanf:"connect-timeout" default:"10s" validate:"gt=0" description:"PostgreSQL connection timeout"`
	AllowLegacySchema   bool          `koanf:"-"`
}

func (c Config) withDefaults() Config {
	if c.Schema == "" {
		c.Schema = DefaultSchema
	}
	return c
}

func (c Config) validate() error {
	c = c.withDefaults()
	if c.URL == "" {
		return errors.New("database URL is required")
	}
	if !schemaNamePattern.MatchString(c.Schema) {
		return errors.New("database schema must be a valid PostgreSQL identifier")
	}
	if c.MaxConnections < 0 || c.MinConnections < 0 {
		return errors.New("database connection limits cannot be negative")
	}
	if c.MaxConnections > 0 && c.MinConnections > c.MaxConnections {
		return errors.New("database minimum connections cannot exceed maximum connections")
	}
	return nil
}

// Open creates and verifies a pgx pool. It returns only after PostgreSQL has
// accepted a ping or the configured connection timeout expires.
func Open(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	if cfg.ApplicationName != "" {
		poolConfig.ConnConfig.RuntimeParams["application_name"] = cfg.ApplicationName
	}
	if cfg.MaxConnections > 0 {
		poolConfig.MaxConns = cfg.MaxConnections
	}
	if cfg.MinConnections > 0 {
		poolConfig.MinConns = cfg.MinConnections
	}
	if cfg.MaxConnectionIdle > 0 {
		poolConfig.MaxConnIdleTime = cfg.MaxConnectionIdle
	}
	if cfg.MaxConnectionLife > 0 {
		poolConfig.MaxConnLifetime = cfg.MaxConnectionLife
	}
	if cfg.HealthCheckInterval > 0 {
		poolConfig.HealthCheckPeriod = cfg.HealthCheckInterval
	}

	timeout := cfg.ConnectTimeout
	if timeout <= 0 {
		timeout = defaultConnectTimeout
	}
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(connectCtx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate applies the embedded TelDrive, River, and RiverPro migrations. Every
// migration line is versioned and safe to call repeatedly.
func Migrate(ctx context.Context, cfg Config) error {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return err
	}
	dsn := cfg.URL
	if dsn == "" {
		return errors.New("database URL is required")
	}
	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse migration database URL: %w", err)
	}
	db := stdlib.OpenDB(*connConfig)
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(ctx, defaultConnectTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("ping migration database: %w", err)
	}
	legacy, err := hasLegacySchema(ctx, db)
	if err != nil {
		return fmt.Errorf("inspect database schema: %w", err)
	}
	if legacy && !cfg.AllowLegacySchema {
		return fmt.Errorf("%w: automatic legacy migration did not complete", ErrLegacySchema)
	}
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+pgx.Identifier{cfg.Schema}.Sanitize()); err != nil {
		return fmt.Errorf("create database schema: %w", err)
	}
	if err := migrations.Up(ctx, db, cfg.Schema); err != nil {
		return err
	}

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse River migration database URL: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("open River migration pool: %w", err)
	}
	defer pool.Close()
	driver := riverpropgxv5.New(pool)
	for _, line := range []string{"main", prodriver.MigrationLinePro} {
		migrator, err := rivermigrate.New(driver, &rivermigrate.Config{Line: line, Schema: cfg.Schema})
		if err != nil {
			return fmt.Errorf("create River migration line %q: %w", line, err)
		}
		if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
			return fmt.Errorf("apply River migration line %q: %w", line, err)
		}
	}
	return nil
}

func hasLegacySchema(ctx context.Context, db *sql.DB) (bool, error) {
	var legacy bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('public.goose_db_version') IS NOT NULL`).Scan(&legacy); err != nil {
		return false, err
	}
	return legacy, nil
}
