package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	extraClausePlugin "github.com/WinterYukky/gorm-extra-clause-plugin"
	"github.com/tgdrive/teldrive/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func NewDatabase(ctx context.Context, cfg *config.DBConfig, logCfg *config.DBLoggingConfig, lg *zap.Logger) (*gorm.DB, error) {
	level, err := zapcore.ParseLevel(logCfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	var db *gorm.DB
	maxRetries := 5
	retryDelay := 500 * time.Millisecond
	connectTimeout := 10 * time.Second

	// Add connect_timeout to DSN if not present
	dsn := cfg.DataSource
	if !strings.Contains(dsn, "connect_timeout") {
		if strings.Contains(dsn, "?") {
			dsn = dsn + fmt.Sprintf("&connect_timeout=%d", int(connectTimeout.Seconds()))
		} else {
			dsn = dsn + fmt.Sprintf("?connect_timeout=%d", int(connectTimeout.Seconds()))
		}
	}

	for i := 0; i <= maxRetries; i++ {
		// Create a timeout context for this attempt so it can be cancelled
		attemptCtx, attemptCancel := context.WithTimeout(ctx, connectTimeout+5*time.Second)

		// Run gorm.Open in a goroutine so we can cancel it via context
		type result struct {
			db  *gorm.DB
			err error
		}
		resultCh := make(chan result, 1)

		go func() {
			db, err := gorm.Open(postgres.New(postgres.Config{
				DSN:                  dsn,
				PreferSimpleProtocol: !cfg.PrepareStmt,
			}), &gorm.Config{
				Logger: NewLogger(lg, logCfg.SlowThreshold, logCfg.IgnoreRecordNotFound, level, logCfg),
				NamingStrategy: schema.NamingStrategy{
					TablePrefix:   "teldrive.",
					SingularTable: false,
				},
				NowFunc: func() time.Time {
					return time.Now().UTC()
				},
			})
			resultCh <- result{db: db, err: err}
		}()

		// Wait for either the result or context cancellation
		select {
		case <-attemptCtx.Done():
			attemptCancel()
			return nil, attemptCtx.Err()
		case res := <-resultCh:
			attemptCancel()
			db = res.db
			err = res.err
		}

		if err == nil {
			if i > 0 {
				lg.Info("db.connection.success", zap.Int("attempts", i+1))
			}
			break
		}

		if i < maxRetries {
			lg.Warn("db.connection.failed",
				zap.Int("attempt", i+1),
				zap.Int("max_retries", maxRetries),
				zap.Error(err),
				zap.Duration("retry_in", retryDelay))

			// Wait for retry delay but check context
			timer := time.NewTimer(retryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		} else {
			lg.Error("db.connection.failed_all_retries",
				zap.Int("max_retries", maxRetries),
				zap.Error(err))
			return nil, fmt.Errorf("database connection failed after %d attempts: %w", maxRetries, err)
		}
	}

	db.Use(extraClausePlugin.New())

	if cfg.Pool.Enable {
		rawDB, err := db.DB()
		if err != nil {
			return nil, err
		}
		rawDB.SetMaxOpenConns(cfg.Pool.MaxOpenConnections)
		rawDB.SetMaxIdleConns(cfg.Pool.MaxIdleConnections)
		rawDB.SetConnMaxLifetime(cfg.Pool.MaxLifetime)
	}

	return db, nil
}
