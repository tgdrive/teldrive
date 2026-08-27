package legacymigrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/database"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
)

type Config struct {
	SourceURL            string
	Target               database.Config
	LegacySchema         string
	FinalSchema          string
	BackupSchema         string
	DataKey              string
	EncryptionKeyVersion int
	Apply                bool
}

type Report struct {
	Users        int64
	Channels     int64
	Bots         int64
	Files        int64
	Folders      int64
	FileParts    int64
	Encrypted    int64
	SkippedZero  int64
	BackupSchema string
}

const migrationLockID int64 = 0x54454c4452495645

func MigrateIfNeeded(ctx context.Context, cfg database.Config, dataKey string) (Report, bool, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return Report{}, false, errors.New("database URL is required")
	}
	conn, err := pgx.Connect(ctx, cfg.URL)
	if err != nil {
		return Report{}, false, fmt.Errorf("connect for legacy migration detection: %w", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		return Report{}, false, fmt.Errorf("acquire legacy migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockID)
	}()

	var legacy bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass('public.goose_db_version') IS NOT NULL`).Scan(&legacy); err != nil {
		return Report{}, false, fmt.Errorf("inspect legacy migration table: %w", err)
	}
	if !legacy {
		return Report{}, false, nil
	}
	if strings.TrimSpace(dataKey) == "" {
		return Report{}, false, errors.New("legacy database detected but security.data-key is empty; set security.data-key or TELDRIVE_SECURITY_DATA_KEY before starting Teldrive")
	}
	if cfg.Schema != "" && cfg.Schema != database.DefaultSchema {
		return Report{}, false, fmt.Errorf("legacy database migration requires database.schema=%q", database.DefaultSchema)
	}
	suffix := time.Now().UTC().Format("20060102_150405_000000000")
	report, err := Run(ctx, Config{
		SourceURL: cfg.URL,
		Target: database.Config{
			URL:    cfg.URL,
			Schema: database.DefaultSchema + "_v2_staging_" + suffix,
		},
		LegacySchema:         database.DefaultSchema,
		FinalSchema:          database.DefaultSchema,
		BackupSchema:         database.DefaultSchema + "_legacy_backup_" + suffix,
		DataKey:              dataKey,
		EncryptionKeyVersion: 1,
		Apply:                true,
	})
	if err != nil {
		return Report{}, false, err
	}
	report.BackupSchema = database.DefaultSchema + "_legacy_backup_" + suffix
	return report, true, nil
}

type legacyPart struct {
	ID   int64  `json:"id"`
	Salt string `json:"salt,omitempty"`
}

type legacyFile struct {
	ID        uuid.UUID
	Name      string
	Kind      string
	MimeType  string
	Size      *int64
	UserID    int64
	ParentID  *uuid.UUID
	Status    string
	ChannelID *int64
	Parts     []legacyPart
	Encrypted bool
	Hash      *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type legacyReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func Run(ctx context.Context, cfg Config) (Report, error) {
	if strings.TrimSpace(cfg.SourceURL) == "" || strings.TrimSpace(cfg.Target.URL) == "" {
		return Report{}, errors.New("database URL is required")
	}
	if cfg.SourceURL != cfg.Target.URL {
		return Report{}, errors.New("legacy migration must use one PostgreSQL database")
	}
	cipher, err := secureblob.New(cfg.DataKey)
	if err != nil {
		return Report{}, fmt.Errorf("initialize data-key cipher: %w", err)
	}
	if cfg.EncryptionKeyVersion <= 0 {
		return Report{}, errors.New("encryption key version must be greater than zero")
	}

	source, err := pgx.Connect(ctx, cfg.SourceURL)
	if err != nil {
		return Report{}, fmt.Errorf("connect source database: %w", err)
	}
	defer source.Close(ctx)
	if err := verifyLegacy(ctx, source); err != nil {
		return Report{}, err
	}

	if !cfg.Apply {
		report, _, err := inspect(ctx, source)
		if err != nil {
			return Report{}, err
		}
		return report, nil
	}

	cfg = withSchemaDefaults(cfg)
	if cfg.LegacySchema != database.DefaultSchema || cfg.FinalSchema != database.DefaultSchema {
		return Report{}, fmt.Errorf("legacy and final schema must be %q", database.DefaultSchema)
	}
	if cfg.Target.Schema == cfg.LegacySchema || cfg.Target.Schema == cfg.FinalSchema || cfg.BackupSchema == cfg.Target.Schema {
		return Report{}, errors.New("staging, legacy, final, and backup schemas must be distinct")
	}
	promoted := false
	defer func() {
		if promoted {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = source.Exec(cleanupCtx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{cfg.Target.Schema}.Sanitize()+" CASCADE")
	}()
	sourceTx, err := source.Begin(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("begin legacy migration: %w", err)
	}
	defer sourceTx.Rollback(ctx)
	if _, err := sourceTx.Exec(ctx, `LOCK TABLE
teldrive.users,
teldrive.channels,
teldrive.bots,
teldrive.files,
public.goose_db_version
IN ACCESS EXCLUSIVE MODE`); err != nil {
		return Report{}, fmt.Errorf("lock legacy database: %w", err)
	}
	report, files, err := inspect(ctx, sourceTx)
	if err != nil {
		return Report{}, err
	}
	if err := ensureSchemaAbsent(ctx, sourceTx, cfg.Target.Schema); err != nil {
		return Report{}, err
	}
	if err := ensureSchemaAbsent(ctx, sourceTx, cfg.BackupSchema); err != nil {
		return Report{}, err
	}
	cfg.Target.AllowLegacySchema = true
	if err := database.Migrate(ctx, cfg.Target); err != nil {
		return Report{}, fmt.Errorf("prepare target database: %w", err)
	}
	target, err := database.Open(ctx, cfg.Target)
	if err != nil {
		return Report{}, fmt.Errorf("open target database: %w", err)
	}
	defer target.Close()

	tx, err := target.Begin(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("begin target transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := ensureEmpty(ctx, tx, cfg.Target.Schema); err != nil {
		return Report{}, err
	}
	if err := migrateUsers(ctx, sourceTx, tx, cfg.Target.Schema); err != nil {
		return Report{}, err
	}
	if err := migrateChannels(ctx, sourceTx, tx, cfg.Target.Schema); err != nil {
		return Report{}, err
	}
	if err := migrateBots(ctx, sourceTx, tx, cipher, cfg.Target.Schema); err != nil {
		return Report{}, err
	}
	if err := migrateFiles(ctx, tx, files, cfg); err != nil {
		return Report{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Report{}, fmt.Errorf("commit target migration: %w", err)
	}
	target.Close()
	if err := swapSchemas(ctx, sourceTx, cfg); err != nil {
		return Report{}, err
	}
	if err := sourceTx.Commit(ctx); err != nil {
		return Report{}, fmt.Errorf("commit schema cutover: %w", err)
	}
	promoted = true
	return report, nil
}

func withSchemaDefaults(cfg Config) Config {
	if cfg.LegacySchema == "" {
		cfg.LegacySchema = database.DefaultSchema
	}
	if cfg.FinalSchema == "" {
		cfg.FinalSchema = database.DefaultSchema
	}
	if cfg.BackupSchema == "" {
		cfg.BackupSchema = cfg.LegacySchema + "_legacy_backup"
	}
	return cfg
}

func ensureSchemaAbsent(ctx context.Context, conn legacyReader, schema string) error {
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname=$1)`, schema).Scan(&exists); err != nil {
		return fmt.Errorf("inspect schema %s: %w", schema, err)
	}
	if exists {
		return fmt.Errorf("schema %s already exists", schema)
	}
	return nil
}

func swapSchemas(ctx context.Context, tx pgx.Tx, cfg Config) error {
	legacy := pgx.Identifier{cfg.LegacySchema}.Sanitize()
	backup := pgx.Identifier{cfg.BackupSchema}.Sanitize()
	staging := pgx.Identifier{cfg.Target.Schema}.Sanitize()
	final := pgx.Identifier{cfg.FinalSchema}.Sanitize()
	if _, err := tx.Exec(ctx, "ALTER SCHEMA "+legacy+" RENAME TO "+backup); err != nil {
		return fmt.Errorf("rename legacy schema to backup: %w", err)
	}
	var gooseExists bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('public.goose_db_version') IS NOT NULL`).Scan(&gooseExists); err != nil {
		return fmt.Errorf("inspect legacy goose table: %w", err)
	}
	if gooseExists {
		if _, err := tx.Exec(ctx, "ALTER TABLE public.goose_db_version SET SCHEMA "+backup); err != nil {
			return fmt.Errorf("move legacy goose table to backup schema: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, "ALTER SCHEMA "+staging+" RENAME TO "+final); err != nil {
		return fmt.Errorf("promote staging schema: %w", err)
	}
	return nil
}

func verifyLegacy(ctx context.Context, conn *pgx.Conn) error {
	var legacy bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass('public.goose_db_version') IS NOT NULL`).Scan(&legacy); err != nil {
		return fmt.Errorf("inspect legacy migration table: %w", err)
	}
	if !legacy {
		return errors.New("source is not a legacy TelDrive production database")
	}
	return nil
}

func inspect(ctx context.Context, source legacyReader) (Report, []legacyFile, error) {
	var report Report
	if err := source.QueryRow(ctx, `SELECT
(SELECT count(*) FROM teldrive.users),
(SELECT count(*) FROM teldrive.channels),
(SELECT count(*) FROM teldrive.bots)`).Scan(&report.Users, &report.Channels, &report.Bots); err != nil {
		return Report{}, nil, fmt.Errorf("count legacy rows: %w", err)
	}

	rows, err := source.Query(ctx, `
SELECT id, name, type, mime_type, size, user_id, parent_id, status,
       channel_id, COALESCE(parts, '[]'::jsonb), COALESCE(encrypted, false),
       hash, created_at, updated_at
FROM teldrive.files
ORDER BY created_at, id`)
	if err != nil {
		return Report{}, nil, fmt.Errorf("read legacy files: %w", err)
	}
	defer rows.Close()

	var files []legacyFile
	for rows.Next() {
		var f legacyFile
		var raw []byte
		if err := rows.Scan(&f.ID, &f.Name, &f.Kind, &f.MimeType, &f.Size, &f.UserID, &f.ParentID, &f.Status, &f.ChannelID, &raw, &f.Encrypted, &f.Hash, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return Report{}, nil, fmt.Errorf("scan legacy file: %w", err)
		}
		if err := json.Unmarshal(raw, &f.Parts); err != nil {
			return Report{}, nil, fmt.Errorf("decode parts for file %s: %w", f.ID, err)
		}
		if _, _, err := catalog.NormalizeName(f.Name); err != nil {
			return Report{}, nil, fmt.Errorf("invalid name for file %s: %w", f.ID, err)
		}
		if f.Kind == "folder" {
			report.Folders++
		} else {
			report.Files++
			if f.Size == nil || *f.Size < 0 {
				return Report{}, nil, fmt.Errorf("invalid size for file %s", f.ID)
			}
			if *f.Size == 0 && len(f.Parts) == 0 {
				report.SkippedZero++
			} else if len(f.Parts) == 0 || f.ChannelID == nil {
				return Report{}, nil, fmt.Errorf("file %s has no usable Telegram parts", f.ID)
			}
			report.FileParts += int64(len(f.Parts))
			if f.Encrypted {
				report.Encrypted++
				for _, p := range f.Parts {
					if p.Salt == "" {
						return Report{}, nil, fmt.Errorf("encrypted file %s has a part without salt", f.ID)
					}
				}
			}
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return Report{}, nil, fmt.Errorf("iterate legacy files: %w", err)
	}
	ordered, err := orderFilesParentFirst(files)
	if err != nil {
		return Report{}, nil, err
	}
	return report, ordered, nil
}

func orderFilesParentFirst(files []legacyFile) ([]legacyFile, error) {
	byID := make(map[uuid.UUID]legacyFile, len(files))
	for _, file := range files {
		byID[file.ID] = file
	}
	state := make(map[uuid.UUID]uint8, len(files))
	ordered := make([]legacyFile, 0, len(files))
	var visit func(uuid.UUID) error
	visit = func(id uuid.UUID) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("legacy file hierarchy contains a cycle at %s", id)
		case 2:
			return nil
		}
		file, ok := byID[id]
		if !ok {
			return fmt.Errorf("legacy file %s was not found", id)
		}
		state[id] = 1
		if file.ParentID != nil {
			parent, ok := byID[*file.ParentID]
			if !ok || parent.Kind != "folder" || parent.UserID != file.UserID {
				return fmt.Errorf("invalid parent for file %s", file.ID)
			}
			if err := visit(parent.ID); err != nil {
				return err
			}
		}
		state[id] = 2
		ordered = append(ordered, file)
		return nil
	}
	for _, file := range files {
		if err := visit(file.ID); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func ensureEmpty(ctx context.Context, tx pgx.Tx, schema string) error {
	var users, channels, bots, files, parts int64
	prefix := pgx.Identifier{schema}.Sanitize()
	query := fmt.Sprintf(`SELECT
(SELECT count(*) FROM %s.users),
(SELECT count(*) FROM %s.channels),
(SELECT count(*) FROM %s.bots),
(SELECT count(*) FROM %s.files),
(SELECT count(*) FROM %s.file_parts)`, prefix, prefix, prefix, prefix, prefix)
	if err := tx.QueryRow(ctx, query).Scan(&users, &channels, &bots, &files, &parts); err != nil {
		return fmt.Errorf("inspect target tables: %w", err)
	}
	for table, count := range map[string]int64{"users": users, "channels": channels, "bots": bots, "files": files, "file_parts": parts} {
		if count != 0 {
			return fmt.Errorf("target table %s is not empty", table)
		}
	}
	return nil
}

func migrateUsers(ctx context.Context, source legacyReader, tx pgx.Tx, schema string) error {
	rows, err := source.Query(ctx, `
SELECT user_id, name, user_name, is_premium, created_at, updated_at,
       CASE WHEN row_number() OVER (ORDER BY created_at ASC, user_id ASC) = 1 THEN 'owner' ELSE 'user' END
FROM teldrive.users
ORDER BY user_id`)
	if err != nil {
		return fmt.Errorf("read users: %w", err)
	}
	defer rows.Close()
	_, err = tx.CopyFrom(ctx, pgx.Identifier{schema, "users"}, []string{"user_id", "display_name", "username", "premium", "created_at", "updated_at", "role"}, pgx.CopyFromFunc(func() ([]any, error) {
		if !rows.Next() {
			return nil, rows.Err()
		}
		var id int64
		var name, username *string
		var premium bool
		var created, updated time.Time
		var role string
		if err := rows.Scan(&id, &name, &username, &premium, &created, &updated, &role); err != nil {
			return nil, err
		}
		return []any{id, name, username, premium, created, updated, role}, nil
	}))
	if err != nil {
		return fmt.Errorf("copy users: %w", err)
	}
	return nil
}

func migrateChannels(ctx context.Context, source legacyReader, tx pgx.Tx, schema string) error {
	rows, err := source.Query(ctx, `SELECT channel_id,user_id,channel_name,COALESCE(selected,false),COALESCE(created_at,now()) FROM teldrive.channels ORDER BY user_id,channel_id`)
	if err != nil {
		return fmt.Errorf("read channels: %w", err)
	}
	defer rows.Close()
	_, err = tx.CopyFrom(ctx, pgx.Identifier{schema, "channels"}, []string{"channel_id", "user_id", "name", "selected", "created_at", "updated_at"}, pgx.CopyFromFunc(func() ([]any, error) {
		if !rows.Next() {
			return nil, rows.Err()
		}
		var channelID, userID int64
		var name string
		var selected bool
		var created time.Time
		if err := rows.Scan(&channelID, &userID, &name, &selected, &created); err != nil {
			return nil, err
		}
		return []any{channelID, userID, name, selected, created, created}, nil
	}))
	if err != nil {
		return fmt.Errorf("copy channels: %w", err)
	}
	return nil
}

func migrateBots(ctx context.Context, source legacyReader, tx pgx.Tx, cipher *secureblob.Cipher, schema string) error {
	rows, err := source.Query(ctx, `SELECT user_id,token,bot_id FROM teldrive.bots ORDER BY user_id,bot_id`)
	if err != nil {
		return fmt.Errorf("read bots: %w", err)
	}
	defer rows.Close()
	_, err = tx.CopyFrom(ctx, pgx.Identifier{schema, "bots"}, []string{"bot_id", "user_id", "token_ciphertext", "enabled"}, pgx.CopyFromFunc(func() ([]any, error) {
		if !rows.Next() {
			return nil, rows.Err()
		}
		var userID, botID int64
		var token string
		if err := rows.Scan(&userID, &token, &botID); err != nil {
			return nil, err
		}
		sealed, err := cipher.Seal("bot-token", []byte(token))
		if err != nil {
			return nil, fmt.Errorf("encrypt bot %d: %w", botID, err)
		}
		return []any{botID, userID, sealed, true}, nil
	}))
	if err != nil {
		return fmt.Errorf("copy bots: %w", err)
	}
	return nil
}

func migrateFiles(ctx context.Context, tx pgx.Tx, files []legacyFile, cfg Config) error {
	fileRows := make([][]any, 0, len(files))
	partRows := make([][]any, 0)
	for _, f := range files {
		display, normalized, err := catalog.NormalizeName(f.Name)
		if err != nil {
			return err
		}
		status := "active"
		var deletedAt *time.Time
		if f.Status != "active" {
			status = "deletion_pending"
			t := f.UpdatedAt
			deletedAt = &t
		}
		enc := false
		var keyVersion *int
		if f.Encrypted {
			enc = true
			v := cfg.EncryptionKeyVersion
			keyVersion = &v
		}
		var size *int64
		if f.Kind == "file" {
			size = f.Size
		}
		var hashAlg, hashValue *string
		if f.Hash != nil && *f.Hash != "" {
			alg := "blake3-tree"
			hashAlg, hashValue = &alg, f.Hash
		}
		fileRows = append(fileRows, []any{f.ID, f.UserID, f.ParentID, display, normalized, f.Kind, f.MimeType, size, hashAlg, hashValue, enc, keyVersion, status, f.UpdatedAt, int64(1), f.CreatedAt, f.UpdatedAt, deletedAt})
		if f.Kind != "file" || len(f.Parts) == 0 {
			continue
		}
		for i, p := range f.Parts {
			var salt *string
			if p.Salt != "" {
				salt = &p.Salt
			}
			partRows = append(partRows, []any{f.ID, int32(i + 1), *f.ChannelID, p.ID, nil, nil, salt, f.CreatedAt})
		}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{cfg.Target.Schema, "files"}, []string{"id", "user_id", "parent_id", "name", "normalized_name", "kind", "mime_type", "size", "hash_algorithm", "hash_value", "encryption", "encryption_key_version", "status", "mod_time", "generation", "created_at", "updated_at", "deleted_at"}, pgx.CopyFromRows(fileRows)); err != nil {
		return fmt.Errorf("copy files: %w", err)
	}
	if len(partRows) > 0 {
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{cfg.Target.Schema, "file_parts"}, []string{"file_id", "part_no", "channel_id", "message_id", "plain_size", "stored_size", "salt", "created_at"}, pgx.CopyFromRows(partRows)); err != nil {
			return fmt.Errorf("copy file parts: %w", err)
		}
	}
	return nil
}
