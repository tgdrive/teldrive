package fileops

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/channels"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

var (
	ErrInvalidInput = errors.New("invalid file operation input")
	ErrNotFound     = errors.New("file not found")
	ErrNotTrashed   = errors.New("file must be trashed before permanent deletion")
)

type Service struct {
	pool     *pgxpool.Pool
	queries  *sqlcgen.Queries
	catalog  *catalog.Service
	channels *channels.Service
	storage  telegramstore.Storage
}

type CopyInput struct {
	UserID         int64
	FileID         uuid.UUID
	ParentID       *uuid.UUID
	Name           *string
	ConflictPolicy sqlcgen.NameConflictPolicy
}

type treeNode struct {
	File  sqlcgen.File
	Depth int32
}

type copiedPart struct {
	Part   sqlcgen.FilePart
	Stored telegramstore.StoredPart
}

type copiedFileRecord struct {
	ID                   uuid.UUID  `json:"id"`
	UserID               int64      `json:"user_id"`
	ParentID             *uuid.UUID `json:"parent_id"`
	Name                 string     `json:"name"`
	NormalizedName       string     `json:"normalized_name"`
	Kind                 string     `json:"kind"`
	MIMEType             *string    `json:"mime_type"`
	Size                 *int64     `json:"size"`
	HashAlgorithm        *string    `json:"hash_algorithm"`
	HashValue            *string    `json:"hash_value"`
	Encryption           bool       `json:"encryption"`
	EncryptionKeyVersion *int32     `json:"encryption_key_version"`
	ModTime              time.Time  `json:"mod_time"`
}

type copiedFilePartRecord struct {
	FileID      uuid.UUID `json:"file_id"`
	PartNo      int32     `json:"part_no"`
	ChannelID   int64     `json:"channel_id"`
	MessageID   int64     `json:"message_id"`
	PlainSize   *int64    `json:"plain_size"`
	StoredSize  *int64    `json:"stored_size"`
	Checksum    *string   `json:"checksum"`
	Salt        *string   `json:"salt"`
	BlockHashes []byte    `json:"block_hashes"`
}

func NewService(pool *pgxpool.Pool, catalogService *catalog.Service, channelService *channels.Service, storage telegramstore.Storage) (*Service, error) {
	if pool == nil || catalogService == nil || channelService == nil || storage == nil {
		return nil, ErrInvalidInput
	}
	return &Service{pool: pool, queries: sqlcgen.New(pool), catalog: catalogService, channels: channelService, storage: storage}, nil
}

func (s *Service) Copy(ctx context.Context, in CopyInput) (*sqlcgen.File, error) {
	if in.UserID <= 0 || in.FileID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	if in.ConflictPolicy == "" {
		in.ConflictPolicy = sqlcgen.NameConflictPolicyFail
	}
	switch in.ConflictPolicy {
	case sqlcgen.NameConflictPolicyFail, sqlcgen.NameConflictPolicyRename, sqlcgen.NameConflictPolicyReplace:
	default:
		return nil, ErrInvalidInput
	}
	if in.ParentID != nil {
		parent, err := s.catalog.Get(ctx, in.UserID, *in.ParentID)
		if err != nil {
			return nil, err
		}
		if parent.Kind != sqlcgen.FileKindFolder || parent.Status != sqlcgen.FileStatusActive {
			return nil, catalog.ErrInvalidParent
		}
	}
	nodes, err := s.loadTree(ctx, in.UserID, in.FileID)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 || nodes[0].File.Status != sqlcgen.FileStatusActive {
		return nil, ErrNotFound
	}

	idMap := make(map[uuid.UUID]uuid.UUID, len(nodes))
	for _, node := range nodes {
		oldID, ok := dbtypes.GoogleUUID(node.File.ID)
		if !ok {
			return nil, ErrNotFound
		}
		idMap[oldID] = uuid.New()
	}
	rootOldID, _ := dbtypes.GoogleUUID(nodes[0].File.ID)
	rootNewID := idMap[rootOldID]
	rootName := nodes[0].File.Name
	if in.Name != nil {
		rootName = strings.TrimSpace(*in.Name)
	}
	rootDisplayName, rootNormalizedName, err := catalog.NormalizeName(rootName)
	if err != nil {
		return nil, err
	}

	sourceFileIDs := make([]uuid.UUID, 0)
	for _, node := range nodes {
		if node.File.Kind != sqlcgen.FileKindFile {
			continue
		}
		if node.File.Status != sqlcgen.FileStatusActive {
			return nil, catalog.ErrNotAFile
		}
		oldID, _ := dbtypes.GoogleUUID(node.File.ID)
		sourceFileIDs = append(sourceFileIDs, oldID)
	}
	parts, err := s.queries.ListFilePartsByFileIDs(ctx, pgUUIDs(sourceFileIDs))
	if err != nil {
		return nil, fmt.Errorf("list copied file parts: %w", err)
	}
	destinationChannels, err := s.channels.ResolveMany(ctx, in.UserID, len(parts))
	if err != nil {
		return nil, err
	}
	copied := make(map[uuid.UUID][]copiedPart, len(sourceFileIDs))
	cleanup := make([]telegramstore.StoredPart, 0)
	compensate := func() {
		grouped := make(map[int64][]int64)
		for _, part := range cleanup {
			if part.ChannelID != 0 && part.MessageID > 0 {
				grouped[part.ChannelID] = append(grouped[part.ChannelID], part.MessageID)
			}
		}
		for channelID, messageIDs := range grouped {
			_ = s.storage.DeleteMessages(context.Background(), in.UserID, channelID, messageIDs)
		}
	}

	for index, part := range parts {
		oldID, ok := dbtypes.GoogleUUID(part.FileID)
		if !ok {
			compensate()
			return nil, ErrNotFound
		}
		stored, err := s.storage.CopyPart(ctx, in.UserID, part.ChannelID, part.MessageID, destinationChannels[index])
		if err != nil {
			compensate()
			return nil, fmt.Errorf("copy Telegram part %d: %w", part.PartNo, err)
		}
		if !part.StoredSize.Valid || stored.Size != part.StoredSize.Int64 {
			cleanup = append(cleanup, stored)
			compensate()
			return nil, telegramstore.ErrSizeMismatch
		}
		cleanup = append(cleanup, stored)
		copied[oldID] = append(copied[oldID], copiedPart{Part: *part, Stored: stored})
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		compensate()
		return nil, err
	}
	defer tx.Rollback(ctx)
	queries := s.queries.WithTx(tx)
	if err := queries.AcquireAdvisoryTransactionLock(ctx, copyDestinationLockID(in.UserID, in.ParentID)); err != nil {
		compensate()
		return nil, fmt.Errorf("lock copy destination: %w", err)
	}
	conflict, conflictErr := queries.LockUploadDestinationConflict(ctx, sqlcgen.LockUploadDestinationConflictParams{
		UserID: in.UserID, ParentID: dbtypes.OptionalUUID(in.ParentID), NormalizedName: rootNormalizedName,
	})
	if conflictErr != nil && !errors.Is(conflictErr, pgx.ErrNoRows) {
		compensate()
		return nil, fmt.Errorf("check copy destination conflict: %w", conflictErr)
	}
	var replacedIDs []uuid.UUID
	if conflictErr == nil {
		conflictID, ok := dbtypes.GoogleUUID(conflict.ID)
		if !ok {
			compensate()
			return nil, catalog.ErrConflict
		}
		switch in.ConflictPolicy {
		case sqlcgen.NameConflictPolicyFail:
			compensate()
			return nil, catalog.ErrConflict
		case sqlcgen.NameConflictPolicyRename:
			rootDisplayName, rootNormalizedName, err = nextAvailableCopyName(ctx, queries, in.UserID, in.ParentID, rootDisplayName)
			if err != nil {
				compensate()
				return nil, err
			}
		case sqlcgen.NameConflictPolicyReplace:
			subtreeIDs, err := queries.ListFileSubtreeIDs(ctx, sqlcgen.ListFileSubtreeIDsParams{
				FileID: dbtypes.UUID(conflictID), UserID: in.UserID,
			})
			if err != nil {
				compensate()
				return nil, fmt.Errorf("list replaced copy destination subtree: %w", err)
			}
			for _, subtreeID := range subtreeIDs {
				if value, ok := dbtypes.GoogleUUID(subtreeID); ok {
					replacedIDs = append(replacedIDs, value)
				}
			}
			if err := queries.MarkFileSubtreeDeletionPending(ctx, sqlcgen.MarkFileSubtreeDeletionPendingParams{
				UserID: in.UserID, FileID: dbtypes.UUID(conflictID),
			}); err != nil {
				compensate()
				return nil, fmt.Errorf("mark replaced copy destination for deletion: %w", err)
			}
			if err := queries.RevokeSharesForFileSubtree(ctx, sqlcgen.RevokeSharesForFileSubtreeParams{
				UserID: in.UserID, FileID: dbtypes.UUID(conflictID),
			}); err != nil {
				compensate()
				return nil, fmt.Errorf("revoke replaced copy destination shares: %w", err)
			}
		}
	}
	fileRecords := make([]copiedFileRecord, 0, len(nodes))
	partRecords := make([]copiedFilePartRecord, 0, len(parts))
	for _, node := range nodes {
		oldID, _ := dbtypes.GoogleUUID(node.File.ID)
		newID := idMap[oldID]
		var parentID *uuid.UUID
		if oldID == rootOldID {
			parentID = in.ParentID
		} else if oldParent, ok := dbtypes.GoogleUUID(node.File.ParentID); ok {
			mapped := idMap[oldParent]
			parentID = &mapped
		}
		displayName, normalizedName := node.File.Name, node.File.NormalizedName
		if oldID == rootOldID {
			displayName, normalizedName = rootDisplayName, rootNormalizedName
		}
		fileRecords = append(fileRecords, copiedFileRecord{
			ID: newID, UserID: in.UserID, ParentID: parentID, Name: displayName,
			NormalizedName: normalizedName, Kind: string(node.File.Kind),
			MIMEType: optionalText(node.File.MimeType), Size: optionalInt64(node.File.Size),
			HashAlgorithm: optionalText(node.File.HashAlgorithm), HashValue: optionalText(node.File.HashValue),
			Encryption: node.File.Encryption, EncryptionKeyVersion: optionalInt32(node.File.EncryptionKeyVersion),
			ModTime: node.File.ModTime.Time,
		})
		for _, part := range copied[oldID] {
			partRecords = append(partRecords, copiedFilePartRecord{
				FileID: newID, PartNo: part.Part.PartNo, ChannelID: part.Stored.ChannelID,
				MessageID: part.Stored.MessageID, PlainSize: optionalInt64(part.Part.PlainSize),
				StoredSize: optionalInt64(part.Part.StoredSize), Checksum: optionalText(part.Part.Checksum),
				Salt: optionalText(part.Part.Salt), BlockHashes: part.Part.BlockHashes,
			})
		}
	}
	encodedFiles, err := json.Marshal(fileRecords)
	if err != nil {
		compensate()
		return nil, fmt.Errorf("encode copied files: %w", err)
	}
	insertedFiles, err := queries.InsertCopiedFiles(ctx, encodedFiles)
	if err != nil {
		compensate()
		return nil, fmt.Errorf("insert copied catalog rows: %w", err)
	}
	if len(insertedFiles) != len(fileRecords) {
		compensate()
		return nil, ErrNotFound
	}
	if len(partRecords) > 0 {
		encodedParts, err := json.Marshal(partRecords)
		if err != nil {
			compensate()
			return nil, fmt.Errorf("encode copied file parts: %w", err)
		}
		insertedParts, err := queries.InsertCopiedFileParts(ctx, encodedParts)
		if err != nil {
			compensate()
			return nil, fmt.Errorf("insert copied file parts: %w", err)
		}
		if insertedParts != int64(len(partRecords)) {
			compensate()
			return nil, ErrNotFound
		}
	}
	if err := tx.Commit(ctx); err != nil {
		compensate()
		return nil, fmt.Errorf("commit file copy: %w", err)
	}
	s.catalog.InvalidateFiles(ctx, in.UserID, replacedIDs...)
	for _, file := range insertedFiles {
		if id, ok := dbtypes.GoogleUUID(file.ID); ok && id == rootNewID {
			return file, nil
		}
	}
	return nil, ErrNotFound
}

func optionalText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func optionalInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func optionalInt32(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	return &value.Int32
}

func nextAvailableCopyName(ctx context.Context, queries *sqlcgen.Queries, userID int64, parentID *uuid.UUID, original string) (string, string, error) {
	names, err := queries.ListActiveNormalizedNames(ctx, sqlcgen.ListActiveNormalizedNamesParams{
		UserID: userID, ParentID: dbtypes.OptionalUUID(parentID),
	})
	if err != nil {
		return "", "", fmt.Errorf("list copy destination names: %w", err)
	}
	used := make(map[string]struct{}, len(names))
	for _, name := range names {
		used[name] = struct{}{}
	}
	base, extension := splitCopyName(original)
	for sequence := 1; sequence <= 10000; sequence++ {
		candidate := boundedCopyName(base, extension, fmt.Sprintf(" (%d)", sequence))
		display, normalized, err := catalog.NormalizeName(candidate)
		if err != nil {
			return "", "", err
		}
		if _, exists := used[normalized]; !exists {
			return display, normalized, nil
		}
	}
	return "", "", catalog.ErrConflict
}

func splitCopyName(name string) (string, string) {
	index := strings.LastIndex(name, ".")
	if index <= 0 || index == len(name)-1 {
		return name, ""
	}
	return name[:index], name[index:]
}

func boundedCopyName(base, extension, suffix string) string {
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

func copyDestinationLockID(userID int64, parentID *uuid.UUID) int64 {
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

func (s *Service) CleanTrash(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, ErrInvalidInput
	}
	count, err := s.queries.MarkAllTrashedDeletionPending(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("mark trash deletion pending: %w", err)
	}
	return count, nil
}

func (s *Service) QueuePurge(ctx context.Context, userID int64, fileID uuid.UUID) error {
	if userID <= 0 || fileID == uuid.Nil {
		return ErrInvalidInput
	}
	files, err := s.queries.QueueFileSubtreePurge(ctx, sqlcgen.QueueFileSubtreePurgeParams{
		UserID: userID, FileID: dbtypes.UUID(fileID),
	})
	if err != nil {
		return fmt.Errorf("queue file subtree purge: %w", err)
	}
	if len(files) == 0 {
		if _, err := s.queries.GetFileForUser(ctx, sqlcgen.GetFileForUserParams{
			FileID: dbtypes.UUID(fileID), UserID: userID,
		}); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return fmt.Errorf("load purge root: %w", err)
		}
		return ErrNotTrashed
	}
	ids := make([]uuid.UUID, 0, len(files))
	for _, file := range files {
		id, ok := dbtypes.GoogleUUID(file.ID)
		if !ok {
			return ErrNotFound
		}
		ids = append(ids, id)
	}
	s.catalog.InvalidateFiles(ctx, userID, ids...)
	return nil
}

func (s *Service) Purge(ctx context.Context, userID int64, fileID uuid.UUID) error {
	return s.PurgeMany(ctx, userID, []uuid.UUID{fileID})
}

func (s *Service) PurgeMany(ctx context.Context, userID int64, rootIDs []uuid.UUID) error {
	if userID <= 0 || len(rootIDs) == 0 {
		return ErrInvalidInput
	}
	uniqueRoots := make([]uuid.UUID, 0, len(rootIDs))
	seenRoots := make(map[uuid.UUID]struct{}, len(rootIDs))
	for _, rootID := range rootIDs {
		if rootID == uuid.Nil {
			return ErrInvalidInput
		}
		if _, ok := seenRoots[rootID]; ok {
			continue
		}
		seenRoots[rootID] = struct{}{}
		uniqueRoots = append(uniqueRoots, rootID)
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire purge lock connection: %w", err)
	}
	defer conn.Release()
	lockQueries := sqlcgen.New(conn)
	lockedRoots := make([]uuid.UUID, 0, len(uniqueRoots))
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, "SELECT pg_advisory_unlock_all()")
	}()
	rootByLockID := make(map[int64]uuid.UUID, len(uniqueRoots))
	lockIDs := make([]int64, 0, len(uniqueRoots))
	for _, rootID := range uniqueRoots {
		lockID := purgeAdvisoryLockID(userID, rootID)
		rootByLockID[lockID] = rootID
		lockIDs = append(lockIDs, lockID)
	}
	locks, err := lockQueries.TryAdvisoryLocks(ctx, lockIDs)
	if err != nil {
		return fmt.Errorf("acquire purge advisory locks: %w", err)
	}
	for _, lock := range locks {
		if lock.Locked {
			lockedRoots = append(lockedRoots, rootByLockID[lock.LockID])
		}
	}
	if len(lockedRoots) == 0 {
		return nil
	}

	nodesByID := make(map[uuid.UUID]treeNode)
	rows, err := s.queries.LoadFileSubtrees(ctx, sqlcgen.LoadFileSubtreesParams{
		RootIds: pgUUIDs(lockedRoots), UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("load file subtrees: %w", err)
	}
	foundRoots := make(map[uuid.UUID]sqlcgen.FileStatus, len(lockedRoots))
	rootSet := make(map[uuid.UUID]struct{}, len(lockedRoots))
	for _, rootID := range lockedRoots {
		rootSet[rootID] = struct{}{}
	}
	for _, row := range rows {
		id, ok := dbtypes.GoogleUUID(row.ID)
		if !ok {
			return ErrNotFound
		}
		node := treeNode{File: sqlcgen.File{
			ID: row.ID, UserID: row.UserID, ParentID: row.ParentID, Name: row.Name,
			NormalizedName: row.NormalizedName, Kind: row.Kind, MimeType: row.MimeType,
			Size: row.Size, HashAlgorithm: row.HashAlgorithm, HashValue: row.HashValue,
			Encryption: row.Encryption, EncryptionKeyVersion: row.EncryptionKeyVersion,
			Status: row.Status, ModTime: row.ModTime, Generation: row.Generation,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DeletedAt: row.DeletedAt,
		}, Depth: row.Depth}
		if _, isRoot := rootSet[id]; isRoot {
			foundRoots[id] = row.Status
		}
		if existing, exists := nodesByID[id]; !exists || node.Depth > existing.Depth {
			nodesByID[id] = node
		}
	}
	for _, rootID := range lockedRoots {
		status, ok := foundRoots[rootID]
		if !ok {
			return ErrNotFound
		}
		if status != sqlcgen.FileStatusTrashed && status != sqlcgen.FileStatusDeletionPending {
			return ErrNotTrashed
		}
	}
	nodes := make([]treeNode, 0, len(nodesByID))
	ids := make([]uuid.UUID, 0, len(nodesByID))
	for id, node := range nodesByID {
		nodes = append(nodes, node)
		ids = append(ids, id)
	}
	fileIDs := pgUUIDs(ids)
	if err := s.queries.MarkFileIDsDeletionPending(ctx, sqlcgen.MarkFileIDsDeletionPendingParams{
		UserID: userID, FileIds: fileIDs,
	}); err != nil {
		return fmt.Errorf("mark subtree deletion pending: %w", err)
	}
	s.catalog.InvalidateFiles(ctx, userID, ids...)

	refs, err := s.queries.ListFilePartMessageRefs(ctx, fileIDs)
	if err != nil {
		return fmt.Errorf("list purge parts: %w", err)
	}
	grouped := make(map[int64][]int64)
	for _, ref := range refs {
		grouped[ref.ChannelID] = append(grouped[ref.ChannelID], ref.MessageID)
	}
	channelIDs := make([]int64, 0, len(grouped))
	for channelID := range grouped {
		channelIDs = append(channelIDs, channelID)
	}
	sort.Slice(channelIDs, func(i, j int) bool { return channelIDs[i] < channelIDs[j] })
	for _, channelID := range channelIDs {
		if err := s.storage.DeleteMessages(ctx, userID, channelID, grouped[channelID]); err != nil {
			return fmt.Errorf("delete Telegram purge messages for channel %d: %w", channelID, err)
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	queries := s.queries.WithTx(tx)
	if err := queries.DeleteFilePartsByFileIDs(ctx, fileIDs); err != nil {
		return fmt.Errorf("delete purge parts: %w", err)
	}
	if err := queries.ClearUploadSessionParentsByFileIDs(ctx, sqlcgen.ClearUploadSessionParentsByFileIDsParams{
		UserID: userID, FileIds: fileIDs,
	}); err != nil {
		return fmt.Errorf("clear purge upload session parents: %w", err)
	}
	byDepth := make(map[int32][]uuid.UUID)
	depths := make([]int32, 0)
	for _, node := range nodes {
		id, _ := dbtypes.GoogleUUID(node.File.ID)
		if _, ok := byDepth[node.Depth]; !ok {
			depths = append(depths, node.Depth)
		}
		byDepth[node.Depth] = append(byDepth[node.Depth], id)
	}
	sort.Slice(depths, func(i, j int) bool { return depths[i] > depths[j] })
	for _, depth := range depths {
		depthIDs := byDepth[depth]
		count, err := queries.DeleteFileCatalogRowsByIDs(ctx, sqlcgen.DeleteFileCatalogRowsByIDsParams{
			FileIds: pgUUIDs(depthIDs), UserID: userID,
		})
		if err != nil {
			return fmt.Errorf("delete purge catalog rows at depth %d: %w", depth, err)
		}
		if count != int64(len(depthIDs)) {
			return ErrNotFound
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit file purge: %w", err)
	}
	s.catalog.InvalidateFiles(ctx, userID, ids...)
	return nil
}

func (s *Service) loadTree(ctx context.Context, userID int64, rootID uuid.UUID) ([]treeNode, error) {
	rows, err := s.queries.LoadFileSubtree(ctx, sqlcgen.LoadFileSubtreeParams{
		RootID: dbtypes.UUID(rootID), UserID: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("load file subtree: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	out := make([]treeNode, 0, len(rows))
	for _, row := range rows {
		out = append(out, treeNode{File: sqlcgen.File{
			ID: row.ID, UserID: row.UserID, ParentID: row.ParentID, Name: row.Name,
			NormalizedName: row.NormalizedName, Kind: row.Kind, MimeType: row.MimeType,
			Size: row.Size, HashAlgorithm: row.HashAlgorithm, HashValue: row.HashValue,
			Encryption: row.Encryption, EncryptionKeyVersion: row.EncryptionKeyVersion,
			Status: row.Status, ModTime: row.ModTime, Generation: row.Generation,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DeletedAt: row.DeletedAt,
		}, Depth: row.Depth})
	}
	return out, nil
}

func pgUUIDs(ids []uuid.UUID) []pgtype.UUID {
	result := make([]pgtype.UUID, len(ids))
	for index, id := range ids {
		result[index] = dbtypes.UUID(id)
	}
	return result
}

func purgeAdvisoryLockID(userID int64, fileID uuid.UUID) int64 {
	var user [8]byte
	binary.BigEndian.PutUint64(user[:], uint64(userID))
	input := append([]byte("teldrive/purge/"), user[:]...)
	input = append(input, fileID[:]...)
	digest := sha256.Sum256(input)
	return int64(binary.BigEndian.Uint64(digest[:8]))
}
