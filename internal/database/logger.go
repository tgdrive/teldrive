package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/logging"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

type Logger struct {
	cfg    glogger.Config
	lg     *zap.Logger
	logCfg *config.DBLoggingConfig
}

func NewLogger(lg *zap.Logger, slowThreshold time.Duration, ignoreRecordNotFoundError bool, level zapcore.Level, logCfg *config.DBLoggingConfig) *Logger {
	cfg := glogger.Config{
		SlowThreshold:             slowThreshold,
		Colorful:                  false,
		IgnoreRecordNotFoundError: ignoreRecordNotFoundError,
	}

	// Map zap level to gorm level - be conservative in production
	switch level {
	case zapcore.DebugLevel:
		cfg.LogLevel = glogger.Info
	case zapcore.InfoLevel:
		cfg.LogLevel = glogger.Warn // Production: only warnings and errors
	case zapcore.WarnLevel:
		cfg.LogLevel = glogger.Warn
	case zapcore.ErrorLevel:
		cfg.LogLevel = glogger.Error
	default:
		cfg.LogLevel = glogger.Silent
	}

	// Use component logger instead of raw logger with prefix
	return &Logger{
		cfg:    cfg,
		lg:     logging.Component("DB").WithOptions(zap.AddCallerSkip(3)),
		logCfg: logCfg,
	}
}

func (l *Logger) LogMode(level glogger.LogLevel) glogger.Interface {
	newlogger := *l
	newlogger.cfg.LogLevel = level
	return &newlogger
}

func (l *Logger) Info(ctx context.Context, msg string, args ...any) {
	if l.cfg.LogLevel >= glogger.Info {
		l.lg.Info("db.info", zap.String("message", fmt.Sprintf(msg, args...)))
	}
}

func (l *Logger) Warn(ctx context.Context, msg string, args ...any) {
	if l.cfg.LogLevel >= glogger.Warn {
		l.lg.Warn("db.warning", zap.String("message", fmt.Sprintf(msg, args...)))
	}
}

func (l *Logger) Error(ctx context.Context, msg string, args ...any) {
	if l.cfg.LogLevel >= glogger.Error {
		l.lg.Error("db.error", zap.String("message", fmt.Sprintf(msg, args...)))
	}
}

func (l *Logger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.cfg.LogLevel <= glogger.Silent {
		return
	}

	elapsed := time.Since(begin)

	// Handle negative elapsed time (system clock changes)
	if elapsed < 0 {
		elapsed = 0
	}
	sql, rows := fc()

	// Extract operation type and table
	opType := extractOperationType(sql)
	tableName := extractTableName(sql)

	// Truncate SQL to prevent huge logs and potential sensitive data exposure
	sqlPreview := truncateSQL(sql, 200)

	fields := []zap.Field{
		zap.String("operation", opType),
		zap.String("table", tableName),
		zap.Float64("duration_ms", float64(elapsed.Nanoseconds())/1e6),
	}

	if rows != -1 {
		fields = append(fields, zap.Int64("rows_affected", rows))
	}

	if l.logCfg != nil && l.logCfg.LogSQLPreview {
		fields = append(fields, zap.String("sql_preview", sqlPreview))
	}

	switch {
	case err != nil && l.cfg.LogLevel >= glogger.Error && (!errors.Is(err, gorm.ErrRecordNotFound) || !l.cfg.IgnoreRecordNotFoundError):
		l.lg.Error("db.query_failed", append(fields, zap.Error(err))...)

	case elapsed > l.cfg.SlowThreshold && l.cfg.SlowThreshold != 0 && l.cfg.LogLevel >= glogger.Warn:
		fields = append(fields,
			zap.Float64("threshold_ms", float64(l.cfg.SlowThreshold.Nanoseconds())/1e6),
			zap.Float64("excess_ms", float64(elapsed.Nanoseconds()-l.cfg.SlowThreshold.Nanoseconds())/1e6),
		)
		l.lg.Warn("db.slow_query", fields...)

	case l.cfg.LogLevel == glogger.Info:
		// Only log successful queries in debug mode
		l.lg.Debug("db.query", fields...)
	}
}

// extractOperationType extracts the SQL operation type (SELECT, INSERT, UPDATE, DELETE, etc.)
func extractOperationType(sql string) string {
	sql = strings.TrimSpace(strings.ToUpper(sql))

	// Handle CTEs (WITH clauses) - look for the main operation after the CTE
	if strings.HasPrefix(sql, "WITH") {
		// Find the main operation after the CTE definition
		// Look for patterns like ") SELECT", ") INSERT", etc.
		for _, op := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
			if idx := strings.LastIndex(sql, " "+op); idx != -1 {
				// Check if this is actually the main operation (comes after CTE)
				nextChar := idx + len(op) + 1
				if nextChar < len(sql) && (sql[nextChar] == ' ' || sql[nextChar] == '\t' || sql[nextChar] == '\n' || sql[nextChar] == '(') {
					return op
				}
			}
		}
	}

	// Standard operations
	switch {
	case strings.HasPrefix(sql, "SELECT") || strings.Contains(sql, " SELECT "):
		return "SELECT"
	case strings.HasPrefix(sql, "INSERT"):
		return "INSERT"
	case strings.HasPrefix(sql, "UPDATE"):
		return "UPDATE"
	case strings.HasPrefix(sql, "DELETE"):
		return "DELETE"
	case strings.HasPrefix(sql, "CREATE"):
		return "CREATE"
	case strings.HasPrefix(sql, "DROP"):
		return "DROP"
	case strings.HasPrefix(sql, "ALTER"):
		return "ALTER"
	case strings.HasPrefix(sql, "TRUNCATE"):
		return "TRUNCATE"
	case strings.HasPrefix(sql, "BEGIN") || strings.HasPrefix(sql, "START TRANSACTION"):
		return "TRANSACTION_BEGIN"
	case strings.HasPrefix(sql, "COMMIT"):
		return "TRANSACTION_COMMIT"
	case strings.HasPrefix(sql, "ROLLBACK"):
		return "TRANSACTION_ROLLBACK"
	default:
		return "OTHER"
	}
}

// extractTableName extracts the table name from SQL query
func extractTableName(sql string) string {
	upperSQL := strings.ToUpper(strings.TrimSpace(sql))

	// Handle CTEs (WITH clauses) - table is usually the main table in SELECT/INSERT/UPDATE
	if strings.HasPrefix(upperSQL, "WITH") {
		upperSQL = removeCTEs(upperSQL)
	}

	// Try to find table name after various SQL keywords
	patterns := []string{
		"FROM ",
		"INTO ",
		"UPDATE ",
		"JOIN ",
	}

	for _, pattern := range patterns {
		if idx := strings.Index(upperSQL, pattern); idx != -1 {
			start := idx + len(pattern)
			if start >= len(upperSQL) {
				continue
			}

			// Extract table name until we hit a delimiter
			remaining := upperSQL[start:]
			end := strings.IndexAny(remaining, " \n\r\t(),;+=<>!*")
			if end == -1 {
				end = len(remaining)
			}

			table := strings.TrimSpace(sql[start : start+end])

			// Remove schema prefix if present (e.g., "teldrive.files" -> "files")
			if dotIdx := strings.Index(table, "."); dotIdx != -1 {
				table = table[dotIdx+1:]
			}

			// Remove quotes if present
			table = strings.Trim(table, `"'`)

			if table != "" {
				return strings.ToLower(table)
			}
		}
	}

	return "unknown"
}

// removeCTEs removes Common Table Expressions (WITH clauses) to get to the main query
func removeCTEs(sql string) string {
	// Simple approach: find the last occurrence of a main operation after the CTEs
	// This is a heuristic - CTEs are complex to parse fully
	upperSQL := strings.ToUpper(sql)

	// Look for the main operation that comes after all CTE definitions
	// CTEs end with a SELECT/INSERT/UPDATE/DELETE that is not inside parentheses
	parenDepth := 0
	lastMainOp := -1

	for i := 0; i < len(upperSQL)-6; i++ {
		if upperSQL[i] == '(' {
			parenDepth++
		} else if upperSQL[i] == ')' {
			parenDepth--
		}

		// Only look for operations outside of parentheses
		if parenDepth == 0 {
			for _, op := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
				if i+len(op) <= len(upperSQL) && upperSQL[i:i+len(op)] == op {
					// Make sure it's a word boundary
					if i == 0 || upperSQL[i-1] == ' ' || upperSQL[i-1] == '\t' || upperSQL[i-1] == '\n' {
						lastMainOp = i
					}
				}
			}
		}
	}

	if lastMainOp != -1 {
		return sql[lastMainOp:]
	}

	return sql
}

// truncateSQL truncates SQL query to prevent huge logs
func truncateSQL(sql string, maxLen int) string {
	if len(sql) <= maxLen {
		return sanitizeSQL(sql)
	}
	return sanitizeSQL(sql[:maxLen]) + "..."
}

// sanitizeSQL removes potential sensitive data from SQL preview
func sanitizeSQL(sql string) string {
	// This is a basic sanitization - removes obvious sensitive patterns
	// In production, you might want more sophisticated sanitization

	// Replace values after IN ( with placeholder
	if idx := strings.Index(strings.ToUpper(sql), " IN ("); idx != -1 {
		// Find the closing parenthesis
		start := idx + 5
		depth := 1
		for i := start; i < len(sql) && depth > 0; i++ {
			if sql[i] == '(' {
				depth++
			} else if sql[i] == ')' {
				depth--
				if depth == 0 {
					// Replace the IN list with a count
					values := strings.Count(sql[start:i], ",") + 1
					sql = sql[:start] + fmt.Sprintf("/* %d values */", values) + sql[i:]
					break
				}
			}
		}
	}

	return sql
}
