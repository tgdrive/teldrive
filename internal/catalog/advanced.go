package catalog

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

var validCategories = map[string]struct{}{
	"archive": {}, "audio": {}, "document": {}, "image": {}, "video": {}, "other": {},
}

// ResolveFolderPath resolves a slash-separated folder path below rootID. A nil
// rootID means the user's drive root; an empty path returns rootID unchanged.
func (s *Service) ResolveFolderPath(ctx context.Context, userID int64, rootID *uuid.UUID, rawPath string) (*uuid.UUID, error) {
	if userID <= 0 {
		return nil, ErrInvalidOwner
	}
	path := strings.Trim(strings.TrimSpace(rawPath), "/")
	if path == "" {
		if rootID == nil {
			return nil, nil
		}
		copyID := *rootID
		return &copyID, nil
	}
	if strings.Contains(path, "\\") {
		return nil, ErrInvalidParent
	}
	current := rootID
	for _, component := range strings.Split(path, "/") {
		if component == "" || component == "." || component == ".." {
			return nil, ErrInvalidParent
		}
		_, normalized, err := NormalizeName(component)
		if err != nil {
			return nil, ErrInvalidParent
		}
		id, err := s.queries.ResolveActiveChildFolder(ctx, sqlcgen.ResolveActiveChildFolderParams{
			UserID: userID, ParentID: dbtypes.OptionalUUID(current), NormalizedName: normalized,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidParent
		}
		if err != nil {
			return nil, fmt.Errorf("resolve folder path: %w", err)
		}
		resolved, ok := dbtypes.GoogleUUID(id)
		if !ok {
			return nil, ErrInvalidParent
		}
		current = &resolved
	}
	return current, nil
}

// EnsureFolderPath resolves a slash-separated folder path and creates missing folders.
func (s *Service) EnsureFolderPath(ctx context.Context, userID int64, rootID *uuid.UUID, rawPath string) (*uuid.UUID, error) {
	if userID <= 0 {
		return nil, ErrInvalidOwner
	}
	path := strings.Trim(strings.TrimSpace(rawPath), "/")
	if path == "" {
		if rootID == nil {
			return nil, nil
		}
		copyID := *rootID
		return &copyID, nil
	}
	if strings.Contains(path, "\\") {
		return nil, ErrInvalidParent
	}
	components := strings.Split(path, "/")
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			return nil, ErrInvalidParent
		}
		if _, _, err := NormalizeName(component); err != nil {
			return nil, ErrInvalidParent
		}
	}

	current := rootID
	for _, component := range components {
		_, normalized, _ := NormalizeName(component)
		id, err := s.queries.ResolveActiveChildFolder(ctx, sqlcgen.ResolveActiveChildFolderParams{
			UserID: userID, ParentID: dbtypes.OptionalUUID(current), NormalizedName: normalized,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			folder, createErr := s.CreateFolder(ctx, CreateFolderInput{UserID: userID, ParentID: current, Name: component})
			if errors.Is(createErr, ErrConflict) {
				id, err = s.queries.ResolveActiveChildFolder(ctx, sqlcgen.ResolveActiveChildFolderParams{
					UserID: userID, ParentID: dbtypes.OptionalUUID(current), NormalizedName: normalized,
				})
			} else if createErr != nil {
				return nil, createErr
			} else {
				id = folder.ID
				err = nil
			}
		}
		if err != nil {
			return nil, fmt.Errorf("ensure folder path: %w", err)
		}
		resolved, ok := dbtypes.GoogleUUID(id)
		if !ok {
			return nil, ErrInvalidParent
		}
		current = &resolved
	}
	return current, nil
}

func (s *Service) listAdvanced(ctx context.Context, in ListInput) ([]*sqlcgen.File, error) {
	if in.SearchType != "text" && in.SearchType != "regex" {
		return nil, ErrInvalidParent
	}
	if in.Sort != "name" && in.Sort != "updatedAt" && in.Sort != "size" && in.Sort != "id" {
		return nil, ErrInvalidParent
	}
	if in.Order != "asc" && in.Order != "desc" {
		return nil, ErrInvalidParent
	}
	if in.UpdatedAfter != nil && in.UpdatedBefore != nil && !in.UpdatedAfter.Before(*in.UpdatedBefore) {
		return nil, ErrInvalidParent
	}
	for _, category := range in.Categories {
		if _, ok := validCategories[category]; !ok {
			return nil, ErrInvalidParent
		}
	}

	var search *string
	if value := strings.TrimSpace(in.Search); value != "" {
		if in.SearchType == "regex" {
			if _, err := regexp.Compile(value); err != nil {
				return nil, ErrInvalidParent
			}
			search = &value
		} else {
			_, normalized, err := NormalizeName(value)
			if err != nil {
				return nil, err
			}
			search = &normalized
		}
	}

	var kind sqlcgen.NullFileKind
	if in.Kind != nil {
		kind = sqlcgen.NullFileKind{FileKind: *in.Kind, Valid: true}
	}
	categories := in.Categories
	if categories == nil {
		categories = []string{}
	}
	var afterName *string
	var afterUpdatedAt *time.Time
	var afterSize *int64
	if in.AfterID != nil && in.Sort != "id" {
		value := in.AfterValue
		if value == "" {
			value = in.AfterName
		}
		switch in.Sort {
		case "name":
			afterName = &value
		case "updatedAt":
			parsed, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return nil, ErrInvalidParent
			}
			afterUpdatedAt = &parsed
		case "size":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, ErrInvalidParent
			}
			afterSize = &parsed
		}
	}

	files, err := s.queries.ListFilesAdvanced(ctx, sqlcgen.ListFilesAdvancedParams{
		UserID: in.UserID, ParentID: dbtypes.OptionalUUID(in.ParentID), Status: in.Status,
		Kind: kind, Search: dbtypes.OptionalText(search), SearchType: in.SearchType,
		Categories: categories, UpdatedAfter: dbtypes.OptionalTime(in.UpdatedAfter),
		UpdatedBefore: dbtypes.OptionalTime(in.UpdatedBefore), AfterID: dbtypes.OptionalUUID(in.AfterID),
		SortBy: in.Sort, AfterName: dbtypes.OptionalText(afterName), SortOrder: in.Order,
		AfterUpdatedAt: dbtypes.OptionalTime(afterUpdatedAt), AfterSize: dbtypes.OptionalInt8(afterSize),
		PageSize: in.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("advanced file list: %w", err)
	}
	return files, nil
}

func FileCursorValue(file *sqlcgen.File, sortBy string) string {
	if file == nil {
		return ""
	}
	switch sortBy {
	case "updatedAt":
		return file.UpdatedAt.Time.UTC().Format(time.RFC3339Nano)
	case "size":
		if file.Size.Valid {
			return strconv.FormatInt(file.Size.Int64, 10)
		}
		return "-1"
	case "id":
		if id, ok := dbtypes.GoogleUUID(file.ID); ok {
			return id.String()
		}
		return ""
	default:
		return file.NormalizedName
	}
}

type CategoryStatistic struct {
	Category   string
	TotalFiles int64
	TotalSize  int64
}

func (s *Service) CategoryStatistics(ctx context.Context, userID int64) ([]CategoryStatistic, error) {
	if userID <= 0 {
		return nil, ErrInvalidOwner
	}
	rows, err := s.queries.ListFileCategoryStatistics(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("file category statistics: %w", err)
	}
	items := make([]CategoryStatistic, 0, len(rows))
	for _, row := range rows {
		items = append(items, CategoryStatistic{
			Category: row.Category, TotalFiles: row.TotalFiles, TotalSize: row.TotalSize,
		})
	}
	return items, nil
}

type DriveStatistic struct {
	TotalFiles   int64
	TotalFolders int64
	TotalBytes   int64
	TrashedFiles int64
	ActiveShares int64
	OpenUploads  int64
}

func (s *Service) DriveStatistics(ctx context.Context, userID int64) (DriveStatistic, error) {
	if userID <= 0 {
		return DriveStatistic{}, ErrInvalidOwner
	}
	row, err := s.queries.GetDriveStatistics(ctx, userID)
	if err != nil {
		return DriveStatistic{}, fmt.Errorf("drive statistics: %w", err)
	}
	return DriveStatistic{
		TotalFiles: row.TotalFiles, TotalFolders: row.TotalFolders, TotalBytes: row.TotalBytes,
		TrashedFiles: row.TrashedFiles, ActiveShares: row.ActiveShares, OpenUploads: row.OpenUploads,
	}, nil
}
