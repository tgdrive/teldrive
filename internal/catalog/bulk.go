package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

const maxBulkFiles = 500

func normalizeBulkIDs(ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 || len(ids) > maxBulkFiles {
		return nil, ErrNotFound
	}
	seen := make(map[uuid.UUID]struct{}, len(ids))
	result := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return nil, ErrNotFound
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

// BulkTrash moves every requested root and all of its descendants to trash in
// one transaction. It returns every affected entry, including descendants.
func (s *Service) BulkTrash(ctx context.Context, userID int64, rawIDs []uuid.UUID) ([]*sqlcgen.File, error) {
	if userID <= 0 {
		return nil, ErrInvalidOwner
	}
	ids, err := normalizeBulkIDs(rawIDs)
	if err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin bulk trash: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := s.queries.WithTx(tx)
	fileIDs := pgUUIDs(ids)

	roots, err := queries.LockActiveFiles(ctx, sqlcgen.LockActiveFilesParams{UserID: userID, FileIds: fileIDs})
	if err != nil {
		return nil, fmt.Errorf("lock bulk trash roots: %w", err)
	}
	if len(roots) != len(ids) {
		return nil, ErrNotFound
	}
	items, err := queries.TrashFileSubtrees(ctx, sqlcgen.TrashFileSubtreesParams{UserID: userID, FileIds: fileIDs})
	if err != nil {
		return nil, fmt.Errorf("bulk trash files: %w", err)
	}
	if err := queries.RevokeSharesForFileSubtrees(ctx, sqlcgen.RevokeSharesForFileSubtreesParams{
		UserID: userID, FileIds: fileIDs,
	}); err != nil {
		return nil, fmt.Errorf("revoke bulk trashed shares: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit bulk trash: %w", err)
	}
	s.InvalidateFiles(ctx, userID, StableIDs(items)...)
	return items, nil
}

// MoveWithPolicy moves one entry using the same conflict rules as bulk move.
func (s *Service) MoveWithPolicy(ctx context.Context, userID int64, fileID uuid.UUID, parentID *uuid.UUID, expectedGeneration *int64, policy string) (*sqlcgen.File, error) {
	items, err := s.bulkMove(ctx, userID, []uuid.UUID{fileID}, parentID, expectedGeneration, policy)
	if err != nil {
		return nil, err
	}
	if len(items) != 1 {
		return nil, ErrNotFound
	}
	return items[0], nil
}

func (s *Service) BulkMove(ctx context.Context, userID int64, ids []uuid.UUID, parentID *uuid.UUID, policy string) ([]*sqlcgen.File, error) {
	return s.bulkMove(ctx, userID, ids, parentID, nil, policy)
}

func (s *Service) bulkMove(ctx context.Context, userID int64, rawIDs []uuid.UUID, parentID *uuid.UUID, expectedGeneration *int64, policy string) ([]*sqlcgen.File, error) {
	if userID <= 0 {
		return nil, ErrInvalidOwner
	}
	ids, err := normalizeBulkIDs(rawIDs)
	if err != nil {
		return nil, err
	}
	if policy == "" {
		policy = "fail"
	}
	if policy != "fail" && policy != "replace" && policy != "rename" {
		return nil, ErrConflict
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin bulk move: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := s.queries.WithTx(tx)

	if parentID != nil {
		if _, err := queries.LockActiveFolder(ctx, sqlcgen.LockActiveFolderParams{
			FolderID: dbtypes.UUID(*parentID), UserID: userID,
		}); errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidParent
		} else if err != nil {
			return nil, fmt.Errorf("lock bulk move destination: %w", err)
		}
	}
	if err := queries.AcquireAdvisoryTransactionLock(ctx, catalogDestinationLockID(userID, parentID)); err != nil {
		return nil, fmt.Errorf("lock bulk move namespace: %w", err)
	}

	lockedRows, err := queries.LockActiveFiles(ctx, sqlcgen.LockActiveFilesParams{
		UserID: userID, FileIds: pgUUIDs(ids),
	})
	if err != nil {
		return nil, fmt.Errorf("lock bulk move files: %w", err)
	}
	locked := make(map[uuid.UUID]*sqlcgen.File, len(ids))
	for _, file := range lockedRows {
		id, ok := fileUUID(file)
		if !ok {
			return nil, ErrNotFound
		}
		locked[id] = file
	}
	if len(locked) != len(ids) {
		return nil, ErrNotFound
	}

	requested := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		requested[id] = struct{}{}
		if parentID != nil && id == *parentID {
			return nil, ErrCycle
		}
		if locked[id].Kind == sqlcgen.FileKindFolder && parentID != nil {
			cycle, err := subtreeContains(ctx, queries, userID, id, *parentID)
			if err != nil {
				return nil, err
			}
			if cycle {
				return nil, ErrCycle
			}
		}
	}

	result := make([]*sqlcgen.File, 0, len(ids))
	invalidated := make([]uuid.UUID, 0)
	for _, id := range ids {
		file := locked[id]
		name, normalized := file.Name, file.NormalizedName
		conflict, conflictErr := queries.LockActiveNameConflict(ctx, sqlcgen.LockActiveNameConflictParams{
			UserID: userID, ParentID: dbtypes.OptionalUUID(parentID), NormalizedName: normalized, ExcludeID: dbtypes.UUID(id),
		})
		if conflictErr != nil && !errors.Is(conflictErr, pgx.ErrNoRows) {
			return nil, fmt.Errorf("check bulk move conflict: %w", conflictErr)
		}
		if conflictErr == nil {
			conflictID, ok := fileUUID(conflict)
			if !ok {
				return nil, ErrConflict
			}
			switch policy {
			case "fail":
				return nil, ErrConflict
			case "replace":
				if _, partOfRequest := requested[conflictID]; partOfRequest {
					return nil, ErrConflict
				}
				replacedIDs, err := queries.ListFileSubtreeIDs(ctx, sqlcgen.ListFileSubtreeIDsParams{
					FileID: dbtypes.UUID(conflictID), UserID: userID,
				})
				if err != nil {
					return nil, fmt.Errorf("list replaced subtree: %w", err)
				}
				for _, replacedID := range replacedIDs {
					if value, ok := dbtypes.GoogleUUID(replacedID); ok {
						invalidated = append(invalidated, value)
					}
				}
				if err := queries.MarkFileSubtreeDeletionPending(ctx, sqlcgen.MarkFileSubtreeDeletionPendingParams{
					FileID: dbtypes.UUID(conflictID), UserID: userID,
				}); err != nil {
					return nil, fmt.Errorf("mark replaced subtree for deletion: %w", err)
				}
				if err := queries.RevokeSharesForFileSubtree(ctx, sqlcgen.RevokeSharesForFileSubtreeParams{
					FileID: dbtypes.UUID(conflictID), UserID: userID,
				}); err != nil {
					return nil, fmt.Errorf("revoke replaced subtree shares: %w", err)
				}
			case "rename":
				name, normalized, err = nextAvailableName(ctx, queries, userID, parentID, file.Name, id)
				if err != nil {
					return nil, err
				}
			}
		}
		updated, err := queries.MoveFileWithName(ctx, sqlcgen.MoveFileWithNameParams{
			ParentID: dbtypes.OptionalUUID(parentID), Name: name, NormalizedName: normalized,
			FileID: dbtypes.UUID(id), UserID: userID, ExpectedGeneration: dbtypes.OptionalInt8(expectedGeneration),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			if expectedGeneration != nil {
				return nil, ErrPrecondition
			}
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, classifyWriteError("move file", err)
		}
		result = append(result, updated)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, classifyWriteError("commit bulk move", err)
	}
	for _, file := range result {
		if id, ok := fileUUID(file); ok {
			invalidated = append(invalidated, id)
		}
	}
	s.InvalidateFiles(ctx, userID, invalidated...)
	return result, nil
}

func subtreeContains(ctx context.Context, queries *sqlcgen.Queries, userID int64, rootID, candidateID uuid.UUID) (bool, error) {
	ids, err := queries.ListFileSubtreeIDs(ctx, sqlcgen.ListFileSubtreeIDsParams{
		FileID: dbtypes.UUID(rootID), UserID: userID,
	})
	if err != nil {
		return false, fmt.Errorf("list file subtree: %w", err)
	}
	for _, id := range ids {
		if value, ok := dbtypes.GoogleUUID(id); ok && value == candidateID {
			return true, nil
		}
	}
	return false, nil
}

func nextAvailableName(ctx context.Context, queries *sqlcgen.Queries, userID int64, parentID *uuid.UUID, original string, excludeID uuid.UUID) (string, string, error) {
	names, err := queries.ListActiveNormalizedNames(ctx, sqlcgen.ListActiveNormalizedNamesParams{
		UserID: userID, ParentID: dbtypes.OptionalUUID(parentID), ExcludeID: dbtypes.UUID(excludeID),
	})
	if err != nil {
		return "", "", fmt.Errorf("list destination names: %w", err)
	}
	used := make(map[string]struct{}, len(names))
	for _, value := range names {
		used[value] = struct{}{}
	}
	base, extension := splitCatalogName(original)
	for sequence := 1; sequence <= 10000; sequence++ {
		candidate := boundedCatalogName(base, extension, fmt.Sprintf(" (%d)", sequence))
		name, normalized, err := NormalizeName(candidate)
		if err != nil {
			return "", "", err
		}
		if _, exists := used[normalized]; !exists {
			return name, normalized, nil
		}
	}
	return "", "", ErrConflict
}

func splitCatalogName(name string) (string, string) {
	index := strings.LastIndex(name, ".")
	if index <= 0 || index == len(name)-1 {
		return name, ""
	}
	return name[:index], name[index:]
}

func boundedCatalogName(base, extension, suffix string) string {
	const maxRunes = 255
	extensionRunes, suffixRunes := []rune(extension), []rune(suffix)
	available := maxRunes - len(extensionRunes) - len(suffixRunes)
	if available < 1 {
		extensionRunes = nil
		available = maxRunes - len(suffixRunes)
	}
	baseRunes := []rune(base)
	if len(baseRunes) > available {
		baseRunes = baseRunes[:available]
	}
	return string(baseRunes) + suffix + string(extensionRunes)
}

func catalogDestinationLockID(userID int64, parentID *uuid.UUID) int64 {
	input := []byte("teldrive/catalog-destination/")
	var user [8]byte
	binary.BigEndian.PutUint64(user[:], uint64(userID))
	input = append(input, user[:]...)
	if parentID != nil {
		input = append(input, parentID[:]...)
	} else {
		input = append(input, make([]byte, 16)...)
	}
	digest := sha256.Sum256(input)
	return int64(binary.BigEndian.Uint64(digest[:8]))
}

func pgUUIDs(ids []uuid.UUID) []pgtype.UUID {
	result := make([]pgtype.UUID, len(ids))
	for index, id := range ids {
		result[index] = dbtypes.UUID(id)
	}
	return result
}

func fileUUID(file *sqlcgen.File) (uuid.UUID, bool) {
	if file == nil || !file.ID.Valid {
		return uuid.Nil, false
	}
	return uuid.UUID(file.ID.Bytes), true
}

// StableIDs is useful to clients and tests that need deterministic ordering of
// an affected subtree returned by BulkTrash.
func StableIDs(files []*sqlcgen.File) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(files))
	for _, file := range files {
		if id, ok := fileUUID(file); ok {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return strings.Compare(ids[i].String(), ids[j].String()) < 0 })
	return ids
}
