package shares

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

var (
	ErrInvalidInput    = errors.New("invalid share input")
	ErrNotFound        = errors.New("share not found")
	ErrExpired         = errors.New("share expired or exhausted")
	ErrPasswordNeeded  = errors.New("share password is required")
	ErrInvalidPassword = errors.New("share password is invalid")
	ErrForbidden       = errors.New("share permission denied")
)

type Created struct {
	Row       *sqlcgen.FileShare
	Token     string
	PublicURL url.URL
}

type Public struct {
	Share *sqlcgen.GetActiveShareByTokenHashRow
	File  *sqlcgen.File
}

type CreateInput struct {
	OwnerID      int64
	FileID       uuid.UUID
	Password     *string
	ExpiresAt    *time.Time
	MaxDownloads *int64
	Permission   sqlcgen.SharePermission
}

type ListInput struct {
	OwnerID        int64
	FileID         uuid.UUID
	AfterCreatedAt *time.Time
	AfterID        *uuid.UUID
	Limit          int32
}

type UpdateInput struct {
	OwnerID           int64
	ShareID           uuid.UUID
	Password          *string
	ClearPassword     bool
	ExpiresAt         *time.Time
	ClearExpiresAt    bool
	MaxDownloads      *int64
	ClearMaxDownloads bool
	Permission        *sqlcgen.SharePermission
}

type GrantCreateInput struct {
	OwnerID    int64
	FileID     uuid.UUID
	GranteeID  int64
	Permission sqlcgen.SharePermission
	ExpiresAt  *time.Time
}

type GrantUpdateInput struct {
	OwnerID        int64
	GrantID        uuid.UUID
	Permission     *sqlcgen.SharePermission
	ExpiresAt      *time.Time
	ClearExpiresAt bool
}

type Access struct {
	OwnerID    int64
	RootFileID uuid.UUID
	Permission sqlcgen.SharePermission
	Owned      bool
}

type PublicListInput struct {
	Token     string
	Password  string
	Path      string
	Search    string
	AfterName string
	AfterID   *uuid.UUID
	Limit     int32
}

type Service struct {
	queries *sqlcgen.Queries
	catalog *catalog.Service
	random  io.Reader
	now     func() time.Time
}

func NewService(pool *pgxpool.Pool, catalogService *catalog.Service) (*Service, error) {
	if pool == nil || catalogService == nil {
		return nil, ErrInvalidInput
	}
	return &Service{queries: sqlcgen.New(pool), catalog: catalogService, random: rand.Reader, now: time.Now}, nil
}

func normalizePermission(permission sqlcgen.SharePermission) sqlcgen.SharePermission {
	if permission == "" {
		return sqlcgen.SharePermissionRead
	}
	return permission
}

func validPermission(permission sqlcgen.SharePermission) bool {
	return permission == sqlcgen.SharePermissionRead || permission == sqlcgen.SharePermissionEdit
}

func optionalPermission(permission *sqlcgen.SharePermission) sqlcgen.NullSharePermission {
	if permission == nil {
		return sqlcgen.NullSharePermission{}
	}
	return sqlcgen.NullSharePermission{SharePermission: *permission, Valid: true}
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*Created, error) {
	in.Permission = normalizePermission(in.Permission)
	if in.OwnerID <= 0 || in.FileID == uuid.Nil || !validPermission(in.Permission) || (in.ExpiresAt != nil && !in.ExpiresAt.After(s.now())) || (in.MaxDownloads != nil && *in.MaxDownloads <= 0) {
		return nil, ErrInvalidInput
	}
	file, err := s.catalog.Get(ctx, in.OwnerID, in.FileID)
	if err != nil {
		return nil, err
	}
	if file.Status != sqlcgen.FileStatusActive {
		return nil, ErrInvalidInput
	}
	secret, hash, err := s.newToken()
	if err != nil {
		return nil, err
	}
	prefix := secret
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}
	var passwordHash *string
	if in.Password != nil {
		password := strings.TrimSpace(*in.Password)
		if password == "" {
			return nil, ErrInvalidInput
		}
		digest, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash share password: %w", err)
		}
		value := string(digest)
		passwordHash = &value
	}
	row, err := s.queries.CreateFileShare(ctx, sqlcgen.CreateFileShareParams{
		ID: dbtypes.UUID(uuid.New()), FileID: dbtypes.UUID(in.FileID), OwnerID: in.OwnerID,
		TokenPrefix: prefix, TokenHash: hash, PasswordHash: dbtypes.OptionalText(passwordHash),
		ExpiresAt: dbtypes.OptionalTime(in.ExpiresAt), MaxDownloads: dbtypes.OptionalInt8(in.MaxDownloads),
		Permission: in.Permission,
	})
	if err != nil {
		return nil, fmt.Errorf("create file share: %w", err)
	}
	publicURL := url.URL{Path: "/share/" + secret}
	return &Created{Row: row, Token: secret, PublicURL: publicURL}, nil
}

func (s *Service) List(ctx context.Context, in ListInput) ([]*sqlcgen.FileShare, error) {
	if in.OwnerID <= 0 || in.FileID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	if _, err := s.catalog.Get(ctx, in.OwnerID, in.FileID); err != nil {
		return nil, err
	}
	if in.Limit <= 0 {
		in.Limit = 100
	}
	if in.Limit > 200 {
		in.Limit = 200
	}
	rows, err := s.queries.ListFileShares(ctx, sqlcgen.ListFileSharesParams{
		OwnerID: in.OwnerID, FileID: dbtypes.UUID(in.FileID),
		AfterCreatedAt: dbtypes.OptionalTime(in.AfterCreatedAt), AfterID: dbtypes.OptionalUUID(in.AfterID),
		PageSize: in.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list file shares: %w", err)
	}
	return rows, nil
}

func (s *Service) Update(ctx context.Context, in UpdateInput) (*sqlcgen.FileShare, error) {
	if in.OwnerID <= 0 || in.ShareID == uuid.Nil ||
		(in.Password != nil && in.ClearPassword) ||
		(in.ExpiresAt != nil && in.ClearExpiresAt) ||
		(in.MaxDownloads != nil && in.ClearMaxDownloads) {
		return nil, ErrInvalidInput
	}
	if in.Password == nil && !in.ClearPassword && in.ExpiresAt == nil && !in.ClearExpiresAt && in.MaxDownloads == nil && !in.ClearMaxDownloads && in.Permission == nil {
		return nil, ErrInvalidInput
	}
	if in.Permission != nil && !validPermission(*in.Permission) {
		return nil, ErrInvalidInput
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(s.now()) {
		return nil, ErrInvalidInput
	}
	if in.MaxDownloads != nil && *in.MaxDownloads <= 0 {
		return nil, ErrInvalidInput
	}
	existing, err := s.queries.GetFileShareForOwner(ctx, sqlcgen.GetFileShareForOwnerParams{
		ID: dbtypes.UUID(in.ShareID), OwnerID: in.OwnerID,
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && existing.RevokedAt.Valid) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get share for update: %w", err)
	}
	if in.MaxDownloads != nil && *in.MaxDownloads < existing.DownloadCount {
		return nil, ErrInvalidInput
	}
	var passwordHash *string
	if in.Password != nil {
		password := strings.TrimSpace(*in.Password)
		if password == "" {
			return nil, ErrInvalidInput
		}
		digest, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash share password: %w", err)
		}
		value := string(digest)
		passwordHash = &value
	}
	updated, err := s.queries.UpdateFileShare(ctx, sqlcgen.UpdateFileShareParams{
		ClearPassword: in.ClearPassword, PasswordHash: dbtypes.OptionalText(passwordHash),
		ClearExpiresAt: in.ClearExpiresAt, ExpiresAt: dbtypes.OptionalTime(in.ExpiresAt),
		ClearMaxDownloads: in.ClearMaxDownloads, MaxDownloads: dbtypes.OptionalInt8(in.MaxDownloads),
		Permission: optionalPermission(in.Permission), ID: dbtypes.UUID(in.ShareID), OwnerID: in.OwnerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update share: %w", err)
	}
	return updated, nil
}

func (s *Service) ListPublicFiles(ctx context.Context, in PublicListInput) ([]*sqlcgen.File, error) {
	resolved, err := s.Resolve(ctx, in.Token, in.Password)
	if err != nil {
		return nil, err
	}
	if resolved.File.Kind != sqlcgen.FileKindFolder || resolved.File.Status != sqlcgen.FileStatusActive {
		return nil, ErrInvalidInput
	}
	rootID, ok := dbtypes.GoogleUUID(resolved.File.ID)
	if !ok {
		return nil, ErrNotFound
	}
	parentID, err := s.catalog.ResolveFolderPath(ctx, resolved.Share.OwnerID, &rootID, in.Path)
	if err != nil {
		return nil, err
	}
	return s.catalog.List(ctx, catalog.ListInput{
		UserID: resolved.Share.OwnerID, ParentID: parentID, Status: sqlcgen.FileStatusActive,
		Search: in.Search, AfterName: in.AfterName, AfterID: in.AfterID, Limit: in.Limit,
	})
}

func (s *Service) Revoke(ctx context.Context, ownerID int64, shareID uuid.UUID) error {
	if ownerID <= 0 || shareID == uuid.Nil {
		return ErrInvalidInput
	}
	count, err := s.queries.RevokeFileShare(ctx, sqlcgen.RevokeFileShareParams{ID: dbtypes.UUID(shareID), OwnerID: ownerID})
	if err != nil {
		return fmt.Errorf("revoke share: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) Resolve(ctx context.Context, token, password string) (*Public, error) {
	row, err := s.resolveRow(ctx, token, password)
	if err != nil {
		return nil, err
	}
	fileID, ok := dbtypes.GoogleUUID(row.FileID)
	if !ok {
		return nil, ErrNotFound
	}
	file, err := s.catalog.Get(ctx, row.OwnerID, fileID)
	if err != nil {
		return nil, err
	}
	return &Public{Share: row, File: file}, nil
}

func (s *Service) ResolveFile(ctx context.Context, token, password string, fileID uuid.UUID) (*Public, error) {
	if fileID == uuid.Nil {
		return nil, ErrNotFound
	}
	resolved, err := s.Resolve(ctx, token, password)
	if err != nil {
		return nil, err
	}
	rootID, ok := dbtypes.GoogleUUID(resolved.File.ID)
	if !ok {
		return nil, ErrNotFound
	}
	if rootID == fileID {
		return resolved, nil
	}
	if resolved.File.Kind != sqlcgen.FileKindFolder {
		return nil, ErrNotFound
	}
	ids, err := s.queries.ListFileSubtreeIDs(ctx, sqlcgen.ListFileSubtreeIDsParams{FileID: dbtypes.UUID(rootID), UserID: resolved.Share.OwnerID})
	if err != nil {
		return nil, fmt.Errorf("list shared subtree: %w", err)
	}
	allowed := false
	for _, id := range ids {
		if value, ok := dbtypes.GoogleUUID(id); ok && value == fileID {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, ErrNotFound
	}
	file, err := s.catalog.Get(ctx, resolved.Share.OwnerID, fileID)
	if err != nil {
		return nil, err
	}
	if file.Status != sqlcgen.FileStatusActive {
		return nil, ErrNotFound
	}
	return &Public{Share: resolved.Share, File: file}, nil
}

func (s *Service) ReserveFileDownload(ctx context.Context, token, password string, fileID uuid.UUID) (*Public, error) {
	resolved, err := s.ResolveFile(ctx, token, password, fileID)
	if err != nil {
		return nil, err
	}
	if _, err := s.queries.IncrementShareDownloadCount(ctx, resolved.Share.ID); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrExpired
	} else if err != nil {
		return nil, fmt.Errorf("reserve share download: %w", err)
	}
	return resolved, nil
}

// ReserveDownload atomically consumes one allowed download before bytes are
// exposed. A failed stream consumes the reservation, preventing concurrent
// requests from exceeding max_downloads.
func (s *Service) ReserveDownload(ctx context.Context, token, password string) (*Public, error) {
	resolved, err := s.Resolve(ctx, token, password)
	if err != nil {
		return nil, err
	}
	if _, err := s.queries.IncrementShareDownloadCount(ctx, resolved.Share.ID); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrExpired
	} else if err != nil {
		return nil, fmt.Errorf("reserve share download: %w", err)
	}
	return resolved, nil
}

func (s *Service) resolveRow(ctx context.Context, token, password string) (*sqlcgen.GetActiveShareByTokenHashRow, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrNotFound
	}
	row, err := s.queries.GetActiveShareByTokenHash(ctx, tokenHash(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrExpired
	}
	if err != nil {
		return nil, fmt.Errorf("resolve share: %w", err)
	}
	if row.ExpiresAt.Valid && !row.ExpiresAt.Time.After(s.now()) {
		return nil, ErrExpired
	}
	if row.MaxDownloads.Valid && row.DownloadCount >= row.MaxDownloads.Int64 {
		return nil, ErrExpired
	}
	if row.PasswordHash.Valid {
		if password == "" {
			return nil, ErrPasswordNeeded
		}
		if err := bcrypt.CompareHashAndPassword([]byte(row.PasswordHash.String), []byte(password)); err != nil {
			return nil, ErrInvalidPassword
		}
	}
	return row, nil
}

func (s *Service) CreateGrant(ctx context.Context, in GrantCreateInput) (*sqlcgen.FileAccessGrant, error) {
	in.Permission = normalizePermission(in.Permission)
	if in.OwnerID <= 0 || in.GranteeID <= 0 || in.OwnerID == in.GranteeID || in.FileID == uuid.Nil || !validPermission(in.Permission) || (in.ExpiresAt != nil && !in.ExpiresAt.After(s.now())) {
		return nil, ErrInvalidInput
	}
	if _, err := s.catalog.Get(ctx, in.OwnerID, in.FileID); err != nil {
		return nil, err
	}
	user, err := s.queries.GetUser(ctx, in.GranteeID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && user.DisabledAt.Valid) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get grant recipient: %w", err)
	}
	row, err := s.queries.CreateFileAccessGrant(ctx, sqlcgen.CreateFileAccessGrantParams{
		ID: dbtypes.UUID(uuid.New()), FileID: dbtypes.UUID(in.FileID), OwnerID: in.OwnerID, GranteeID: in.GranteeID,
		Permission: in.Permission, ExpiresAt: dbtypes.OptionalTime(in.ExpiresAt),
	})
	if err != nil {
		return nil, fmt.Errorf("create file access grant: %w", err)
	}
	return row, nil
}

func (s *Service) ListGrants(ctx context.Context, ownerID int64, fileID uuid.UUID) ([]*sqlcgen.ListFileAccessGrantsForOwnerRow, error) {
	if ownerID <= 0 || fileID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	if _, err := s.catalog.Get(ctx, ownerID, fileID); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListFileAccessGrantsForOwner(ctx, sqlcgen.ListFileAccessGrantsForOwnerParams{OwnerID: ownerID, FileID: dbtypes.UUID(fileID)})
	if err != nil {
		return nil, fmt.Errorf("list file access grants: %w", err)
	}
	return rows, nil
}

func (s *Service) UpdateGrant(ctx context.Context, in GrantUpdateInput) (*sqlcgen.FileAccessGrant, error) {
	if in.OwnerID <= 0 || in.GrantID == uuid.Nil || (in.Permission == nil && in.ExpiresAt == nil && !in.ClearExpiresAt) || (in.Permission != nil && !validPermission(*in.Permission)) || (in.ExpiresAt != nil && (!in.ExpiresAt.After(s.now()) || in.ClearExpiresAt)) {
		return nil, ErrInvalidInput
	}
	row, err := s.queries.UpdateFileAccessGrant(ctx, sqlcgen.UpdateFileAccessGrantParams{
		Permission: optionalPermission(in.Permission), ExpiresAt: dbtypes.OptionalTime(in.ExpiresAt), ClearExpiresAt: in.ClearExpiresAt,
		ID: dbtypes.UUID(in.GrantID), OwnerID: in.OwnerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update file access grant: %w", err)
	}
	return row, nil
}

func (s *Service) RevokeGrant(ctx context.Context, ownerID int64, grantID uuid.UUID) error {
	if ownerID <= 0 || grantID == uuid.Nil {
		return ErrInvalidInput
	}
	count, err := s.queries.RevokeFileAccessGrant(ctx, sqlcgen.RevokeFileAccessGrantParams{ID: dbtypes.UUID(grantID), OwnerID: ownerID})
	if err != nil {
		return fmt.Errorf("revoke file access grant: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ListShared(ctx context.Context, ownerID int64) ([]*sqlcgen.File, error) {
	if ownerID <= 0 {
		return nil, ErrInvalidInput
	}
	rows, err := s.queries.ListShared(ctx, sqlcgen.ListSharedParams{OwnerID: ownerID, PageSize: 500})
	if err != nil {
		return nil, fmt.Errorf("list shared files: %w", err)
	}
	return rows, nil
}

func (s *Service) ListSharedWithMe(ctx context.Context, granteeID int64) ([]*sqlcgen.File, error) {
	if granteeID <= 0 {
		return nil, ErrInvalidInput
	}
	rows, err := s.queries.ListSharedWithMe(ctx, sqlcgen.ListSharedWithMeParams{GranteeID: granteeID, PageSize: 500})
	if err != nil {
		return nil, fmt.Errorf("list files shared with user: %w", err)
	}
	return rows, nil
}

func (s *Service) ResolveAccess(ctx context.Context, actorID int64, fileID uuid.UUID, requireEdit bool) (*Access, error) {
	if actorID <= 0 || fileID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	file, err := s.queries.GetActiveFileAnyOwner(ctx, dbtypes.UUID(fileID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("resolve file owner: %w", err)
	}
	if file.UserID == actorID {
		return &Access{OwnerID: actorID, RootFileID: fileID, Permission: sqlcgen.SharePermissionEdit, Owned: true}, nil
	}
	grants, err := s.queries.ListActiveFileAccessGrantsForGrantee(ctx, sqlcgen.ListActiveFileAccessGrantsForGranteeParams{GranteeID: actorID, OwnerID: file.UserID})
	if err != nil {
		return nil, fmt.Errorf("list effective access grants: %w", err)
	}
	for _, grant := range grants {
		if requireEdit && grant.Permission != sqlcgen.SharePermissionEdit {
			continue
		}
		rootID, ok := dbtypes.GoogleUUID(grant.FileID)
		if !ok {
			continue
		}
		if rootID == fileID {
			return &Access{OwnerID: file.UserID, RootFileID: rootID, Permission: grant.Permission}, nil
		}
		ids, err := s.queries.ListFileSubtreeIDs(ctx, sqlcgen.ListFileSubtreeIDsParams{FileID: grant.FileID, UserID: file.UserID})
		if err != nil {
			return nil, fmt.Errorf("resolve grant subtree: %w", err)
		}
		for _, id := range ids {
			if candidate, ok := dbtypes.GoogleUUID(id); ok && candidate == fileID {
				return &Access{OwnerID: file.UserID, RootFileID: rootID, Permission: grant.Permission}, nil
			}
		}
	}
	return nil, ErrForbidden
}

func (s *Service) ResolvePublicEditableFile(ctx context.Context, token, password string, fileID uuid.UUID) (*Public, error) {
	resolved, err := s.ResolveFile(ctx, token, password, fileID)
	if err != nil {
		return nil, err
	}
	if resolved.Share.Permission != sqlcgen.SharePermissionEdit {
		return nil, ErrForbidden
	}
	return resolved, nil
}

func (s *Service) ResolvePublicEditableParent(ctx context.Context, token, password string, parentID *uuid.UUID) (*Public, uuid.UUID, error) {
	resolved, err := s.Resolve(ctx, token, password)
	if err != nil {
		return nil, uuid.Nil, err
	}
	if resolved.Share.Permission != sqlcgen.SharePermissionEdit || resolved.File.Kind != sqlcgen.FileKindFolder {
		return nil, uuid.Nil, ErrForbidden
	}
	rootID, ok := dbtypes.GoogleUUID(resolved.File.ID)
	if !ok {
		return nil, uuid.Nil, ErrNotFound
	}
	if parentID == nil || *parentID == uuid.Nil || *parentID == rootID {
		return resolved, rootID, nil
	}
	child, err := s.ResolvePublicEditableFile(ctx, token, password, *parentID)
	if err != nil {
		return nil, uuid.Nil, err
	}
	if child.File.Kind != sqlcgen.FileKindFolder {
		return nil, uuid.Nil, ErrInvalidInput
	}
	return child, *parentID, nil
}

func (s *Service) newToken() (string, []byte, error) {
	buffer := make([]byte, 32)
	if _, err := io.ReadFull(s.random, buffer); err != nil {
		return "", nil, err
	}
	token := "tds_" + base64.RawURLEncoding.EncodeToString(buffer)
	return token, tokenHash(token), nil
}

func tokenHash(token string) []byte {
	digest := sha256.Sum256([]byte(token))
	return digest[:]
}
