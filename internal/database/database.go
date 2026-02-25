package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tgdrive/teldrive/internal/config"
	"go.uber.org/zap"
)

func NewDatabase(ctx context.Context, cfg *config.DBConfig, _ *config.DBLoggingConfig, lg *zap.Logger) (*pgxpool.Pool, error) {
	var db *pgxpool.Pool
	var err error
	maxRetries := 5
	retryDelay := 500 * time.Millisecond
	connectTimeout := 10 * time.Second
	dsn := cfg.DataSource

	for i := 0; i <= maxRetries; i++ {
		attemptCtx, attemptCancel := context.WithTimeout(ctx, connectTimeout+5*time.Second)

		type result struct {
			db  *pgxpool.Pool
			err error
		}
		resultCh := make(chan result, 1)

		go func() {
			poolCfg, err := pgxpool.ParseConfig(dsn)
			if err != nil {
				resultCh <- result{db: nil, err: err}
				return
			}
			poolCfg.ConnConfig.ConnectTimeout = connectTimeout
			poolCfg.MaxConns = int32(cfg.Pool.MaxOpenConnections)
			poolCfg.MinConns = int32(cfg.Pool.MaxIdleConnections)
			poolCfg.MaxConnLifetime = cfg.Pool.MaxLifetime
			pool, err := pgxpool.NewWithConfig(attemptCtx, poolCfg)
			if err != nil {
				resultCh <- result{db: nil, err: err}
				return
			}
			if err := pool.Ping(attemptCtx); err != nil {
				pool.Close()
				resultCh <- result{db: nil, err: err}
				return
			}
			resultCh <- result{db: pool, err: nil}
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

	return db, nil
}
