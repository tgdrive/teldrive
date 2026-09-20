package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
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
	}
	if parentID != nil {
		ancestorIDs, err := queries.ListFileAncestorIDs(ctx, sqlcgen.ListFileAncestorIDsParams{
			FileID: dbtypes.UUID(*parentID), UserID: userID,
		})
		if err != nil {
			return nil, fmt.Errorf("list bulk move destination ancestors: %w", err)
		}
		for _, ancestorID := range ancestorIDs {
			id, ok := dbtypes.GoogleUUID(ancestorID)
			if ok && locked[id] != nil && locked[id].Kind == sqlcgen.FileKindFolder {
				return nil, ErrCycle
			}
		}
	}

	destination, err := queries.LockActiveDestinationEntries(ctx, sqlcgen.LockActiveDestinationEntriesParams{
		UserID: userID, ParentID: dbtypes.OptionalUUID(parentID),
	})
	if err != nil {
		return nil, fmt.Errorf("lock bulk move destination entries: %w", err)
	}
	usedNames := make(map[string]struct{}, len(destination)+len(ids))
	conflicts := make(map[string]uuid.UUID, len(destination))
	for _, entry := range destination {
		entryID, ok := dbtypes.GoogleUUID(entry.ID)
		if !ok {
			return nil, ErrConflict
		}
		usedNames[entry.Name] = struct{}{}
		conflicts[entry.Name] = entryID
	}

	names := make([]string, len(ids))
	replacedSet := make(map[uuid.UUID]struct{})
	for index, id := range ids {
		file := locked[id]
		name := file.Name
		if occupant, occupied := conflicts[name]; occupied && occupant == id {
			delete(usedNames, name)
			delete(conflicts, name)
		}
		_, nameUsed := usedNames[name]
		if nameUsed {
			switch policy {
			case "fail":
				return nil, ErrConflict
			case "replace":
				conflictID, exists := conflicts[name]
				if !exists {
					return nil, ErrConflict
				}
				if _, moving := requested[conflictID]; moving {
					return nil, ErrConflict
				}
				replacedSet[conflictID] = struct{}{}
				delete(usedNames, name)
				delete(conflicts, name)
			case "rename":
				var renameErr error
				name, renameErr = nextAvailableNameFromSet(file.Name, usedNames)
				if renameErr != nil {
					return nil, renameErr
				}
			}
		}
		usedNames[name] = struct{}{}
		conflicts[name] = id
		names[index] = name
	}

	invalidated := make([]uuid.UUID, 0)
	if len(replacedSet) > 0 {
		replacedRoots := make([]uuid.UUID, 0, len(replacedSet))
		for id := range replacedSet {
			replacedRoots = append(replacedRoots, id)
		}
		replacedRows, err := queries.LoadFileSubtrees(ctx, sqlcgen.LoadFileSubtreesParams{
			RootIds: pgUUIDs(replacedRoots), UserID: userID,
		})
		if err != nil {
			return nil, fmt.Errorf("load replaced subtrees: %w", err)
		}
		for _, row := range replacedRows {
			if id, ok := dbtypes.GoogleUUID(row.ID); ok {
				invalidated = append(invalidated, id)
			}
		}
		if err := queries.MarkFileSubtreesDeletionPending(ctx, sqlcgen.MarkFileSubtreesDeletionPendingParams{
			FileIds: pgUUIDs(replacedRoots), UserID: userID,
		}); err != nil {
			return nil, fmt.Errorf("mark replaced subtrees for deletion: %w", err)
		}
		if err := queries.RevokeSharesForFileSubtrees(ctx, sqlcgen.RevokeSharesForFileSubtreesParams{
			UserID: userID, FileIds: pgUUIDs(replacedRoots),
		}); err != nil {
			return nil, fmt.Errorf("revoke replaced subtree shares: %w", err)
		}
	}

	updatedRows, err := queries.MoveFilesWithNames(ctx, sqlcgen.MoveFilesWithNamesParams{
		ParentID: dbtypes.OptionalUUID(parentID), UserID: userID,
		ExpectedGeneration: dbtypes.OptionalInt8(expectedGeneration), FileIds: pgUUIDs(ids),
		Names: names,
	})
	if err != nil {
		return nil, classifyWriteError("move files", err)
	}
	if len(updatedRows) != len(ids) {
		if expectedGeneration != nil {
			return nil, ErrPrecondition
		}
		return nil, ErrNotFound
	}
	updatedByID := make(map[uuid.UUID]*sqlcgen.File, len(updatedRows))
	for _, file := range updatedRows {
		if id, ok := fileUUID(file); ok {
			updatedByID[id] = file
		}
	}
	result := make([]*sqlcgen.File, 0, len(ids))
	for _, id := range ids {
		file := updatedByID[id]
		if file == nil {
			return nil, ErrNotFound
		}
		result = append(result, file)
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

func nextAvailableNameFromSet(original string, used map[string]struct{}) (string, error) {
	base, extension := splitCatalogName(original)
	for sequence := 1; sequence <= 10000; sequence++ {
		candidate := base + fmt.Sprintf(" (%d)", sequence) + extension
		if _, exists := used[candidate]; !exists {
			return candidate, nil
		}
	}
	return "", ErrConflict
}

func splitCatalogName(name string) (string, string) {
	index := strings.LastIndex(name, ".")
	if index <= 0 || index == len(name)-1 {
		return name, ""
	}
	return name[:index], name[index:]
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
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	return ids
}
