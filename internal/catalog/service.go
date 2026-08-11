package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

var (
	ErrNotFound      = errors.New("file not found")
	ErrConflict      = errors.New("file name conflict")
	ErrInvalidOwner  = errors.New("invalid owner")
	ErrInvalidParent = errors.New("invalid parent folder")
	ErrNotAFile      = errors.New("catalog entry is not an active file")
	ErrCycle         = errors.New("folder move would create a cycle")
	ErrPrecondition  = errors.New("generation precondition failed")
)

type Service struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
	now     func() time.Time
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, queries: sqlcgen.New(pool), now: time.Now}
}

type CreateFolderInput struct {
	UserID   int64
	ParentID *uuid.UUID
	Name     string
	ModTime  time.Time
}

func (s *Service) CreateFolder(ctx context.Context, in CreateFolderInput) (*sqlcgen.File, error) {
	if in.UserID <= 0 {
		return nil, ErrInvalidOwner
	}
	name, normalized, err := NormalizeName(in.Name)
	if err != nil {
		return nil, err
	}
	if in.ParentID != nil {
		if _, err := s.queries.GetActiveFolderForUser(ctx, sqlcgen.GetActiveFolderForUserParams{
			FolderID: dbtypes.UUID(*in.ParentID),
			UserID:   in.UserID,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrInvalidParent
			}
			return nil, fmt.Errorf("get parent folder: %w", err)
		}
	}
	modTime := in.ModTime
	if modTime.IsZero() {
		modTime = s.now().UTC()
	}
	file, err := s.queries.CreateFolder(ctx, sqlcgen.CreateFolderParams{
		ID:             dbtypes.UUID(uuid.New()),
		UserID:         in.UserID,
		ParentID:       dbtypes.OptionalUUID(in.ParentID),
		Name:           name,
		NormalizedName: normalized,
		ModTime:        dbtypes.Time(modTime.UTC()),
	})
	if err != nil {
		return nil, classifyWriteError("create folder", err)
	}
	return file, nil
}

func (s *Service) Get(ctx context.Context, userID int64, fileID uuid.UUID) (*sqlcgen.File, error) {
	if userID <= 0 {
		return nil, ErrInvalidOwner
	}
	file, err := s.queries.GetFileForUser(ctx, sqlcgen.GetFileForUserParams{
		FileID: dbtypes.UUID(fileID),
		UserID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get file: %w", err)
	}
	return file, nil
}

func (s *Service) GetViewState(ctx context.Context, userID int64, fileID uuid.UUID) (*sqlcgen.FileViewState, error) {
	state, err := s.queries.GetFileViewState(ctx, sqlcgen.GetFileViewStateParams{
		UserID: userID, FileID: dbtypes.UUID(fileID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get file view state: %w", err)
	}
	return state, nil
}

func (s *Service) UpsertViewState(ctx context.Context, userID int64, fileID uuid.UUID, kind string, position, preferences, bookmarks []byte) (*sqlcgen.FileViewState, error) {
	state, err := s.queries.UpsertFileViewState(ctx, sqlcgen.UpsertFileViewStateParams{
		UserID: userID, FileID: dbtypes.UUID(fileID), ViewerKind: kind,
		Position: position, Preferences: preferences, Bookmarks: bookmarks,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("upsert file view state: %w", err)
	}
	return state, nil
}

func (s *Service) DeleteViewState(ctx context.Context, userID int64, fileID uuid.UUID) error {
	if _, err := s.queries.DeleteFileViewState(ctx, sqlcgen.DeleteFileViewStateParams{
		UserID: userID, FileID: dbtypes.UUID(fileID),
	}); err != nil {
		return fmt.Errorf("delete file view state: %w", err)
	}
	return nil
}

// Parts returns finalized Telegram parts for an active file owned by userID.
func (s *Service) Parts(ctx context.Context, userID int64, fileID uuid.UUID) ([]*sqlcgen.FilePart, error) {
	file, err := s.Get(ctx, userID, fileID)
	if err != nil {
		return nil, err
	}
	if file.Kind != sqlcgen.FileKindFile || file.Status != sqlcgen.FileStatusActive {
		return nil, ErrNotAFile
	}
	parts, err := s.queries.ListFileParts(ctx, dbtypes.UUID(fileID))
	if err != nil {
		return nil, fmt.Errorf("list file parts: %w", err)
	}
	return parts, nil
}

func (s *Service) UpdatePartSizes(ctx context.Context, fileID uuid.UUID, partNo int32, plainSize, storedSize int64) error {
	_, err := s.queries.UpdateFilePartSizes(ctx, sqlcgen.UpdateFilePartSizesParams{
		FileID: dbtypes.UUID(fileID), PartNo: partNo,
		PlainSize: dbtypes.Int8(plainSize), StoredSize: dbtypes.Int8(storedSize),
	})
	if err != nil {
		return fmt.Errorf("update file part sizes: %w", err)
	}
	return nil
}

type ListInput struct {
	UserID        int64
	ParentID      *uuid.UUID
	Path          string
	Status        sqlcgen.FileStatus
	Kind          *sqlcgen.FileKind
	Search        string
	SearchType    string
	Categories    []string
	UpdatedAfter  *time.Time
	UpdatedBefore *time.Time
	Sort          string
	Order         string
	AfterName     string
	AfterValue    string
	AfterID       *uuid.UUID
	Limit         int32
}

func (s *Service) List(ctx context.Context, in ListInput) ([]*sqlcgen.File, error) {
	if in.UserID <= 0 {
		return nil, ErrInvalidOwner
	}
	if in.ParentID != nil && strings.TrimSpace(in.Path) != "" {
		return nil, ErrInvalidParent
	}
	if strings.TrimSpace(in.Path) != "" {
		resolved, err := s.ResolveFolderPath(ctx, in.UserID, nil, in.Path)
		if err != nil {
			return nil, err
		}
		in.ParentID = resolved
	}
	if in.Status == "" {
		in.Status = sqlcgen.FileStatusActive
	}
	if in.Limit <= 0 {
		in.Limit = 100
	}
	if in.Limit > 200 {
		in.Limit = 200
	}
	if in.SearchType == "" {
		in.SearchType = "text"
	}
	if in.Sort == "" {
		in.Sort = "name"
	}
	if in.Order == "" {
		in.Order = "asc"
	}
	if len(in.Categories) > 0 || in.UpdatedAfter != nil || in.UpdatedBefore != nil || in.SearchType != "text" || in.Sort != "name" || in.Order != "asc" || in.AfterValue != "" {
		return s.listAdvanced(ctx, in)
	}
	var kind sqlcgen.NullFileKind
	if in.Kind != nil {
		kind = sqlcgen.NullFileKind{FileKind: *in.Kind, Valid: true}
	}
	var search pgtype.Text
	if strings.TrimSpace(in.Search) != "" {
		_, folded, err := NormalizeName(in.Search)
		if err != nil {
			return nil, err
		}
		search = dbtypes.Text(folded)
	}
	var afterName pgtype.Text
	var afterID pgtype.UUID
	if in.AfterName != "" && in.AfterID != nil {
		afterName = dbtypes.Text(in.AfterName)
		afterID = dbtypes.UUID(*in.AfterID)
	}
	items, err := s.queries.ListFiles(ctx, sqlcgen.ListFilesParams{
		UserID:    in.UserID,
		ParentID:  dbtypes.OptionalUUID(in.ParentID),
		Status:    in.Status,
		Kind:      kind,
		Search:    search,
		AfterName: afterName,
		AfterID:   afterID,
		PageSize:  in.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	return items, nil
}

func (s *Service) Rename(ctx context.Context, userID int64, fileID uuid.UUID, expectedGeneration *int64, rawName string) (*sqlcgen.File, error) {
	if userID <= 0 {
		return nil, ErrInvalidOwner
	}
	name, normalized, err := NormalizeName(rawName)
	if err != nil {
		return nil, err
	}
	file, err := s.queries.UpdateFileMetadata(ctx, sqlcgen.UpdateFileMetadataParams{
		Name:               dbtypes.Text(name),
		NormalizedName:     dbtypes.Text(normalized),
		FileID:             dbtypes.UUID(fileID),
		UserID:             userID,
		ExpectedGeneration: dbtypes.OptionalInt8(expectedGeneration),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		if expectedGeneration != nil {
			return nil, ErrPrecondition
		}
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, classifyWriteError("rename file", err)
	}
	return file, nil
}

func (s *Service) Move(ctx context.Context, userID int64, fileID uuid.UUID, parentID *uuid.UUID, expectedGeneration *int64) (*sqlcgen.File, error) {
	return s.MoveWithPolicy(ctx, userID, fileID, parentID, expectedGeneration, "fail")
}

func (s *Service) Trash(ctx context.Context, userID int64, fileID uuid.UUID) (*sqlcgen.File, error) {
	items, err := s.BulkTrash(ctx, userID, []uuid.UUID{fileID})
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if id, ok := fileUUID(item); ok && id == fileID {
			return item, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Service) Restore(ctx context.Context, userID int64, fileID uuid.UUID) (*sqlcgen.File, error) {
	if userID <= 0 {
		return nil, ErrInvalidOwner
	}
	file, err := s.queries.RestoreFile(ctx, sqlcgen.RestoreFileParams{FileID: dbtypes.UUID(fileID), UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, classifyWriteError("restore file", err)
	}
	return file, nil
}

func classifyWriteError(action string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return fmt.Errorf("%s: %w", action, err)
}
