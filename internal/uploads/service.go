package uploads

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/treehash"
)

const (
	defaultPartSize   int64 = 512 * 1024 * 1024
	defaultSessionTTL       = 7 * 24 * time.Hour
	defaultLeaseTTL         = time.Minute
)

var (
	ErrInvalidInput              = errors.New("invalid upload input")
	ErrNotFound                  = errors.New("upload not found")
	ErrExpired                   = errors.New("upload expired")
	ErrInvalidState              = errors.New("invalid upload state")
	ErrPartConflict              = errors.New("upload part conflicts with stored metadata")
	ErrPartBusy                  = errors.New("upload part is currently leased")
	ErrLeaseLost                 = errors.New("upload part lease was lost")
	ErrIncomplete                = errors.New("upload parts are incomplete")
	ErrNameConflict              = errors.New("destination name already exists")
	ErrInvalidParent             = errors.New("invalid parent folder")
	ErrInvalidChannel            = errors.New("invalid storage channel")
	ErrHashMismatch              = errors.New("upload hash does not match expected hash")
	ErrUnsupportedConflictPolicy = errors.New("upload conflict policy is not implemented")
)

type CatalogCacheInvalidator interface {
	InvalidateFiles(context.Context, int64, ...uuid.UUID)
}

type Service struct {
	pool               *pgxpool.Pool
	queries            *sqlcgen.Queries
	now                func() time.Time
	sessionTTL         time.Duration
	leaseTTL           time.Duration
	catalogInvalidator CatalogCacheInvalidator
}

func NewService(pool *pgxpool.Pool, sessionTTLs ...time.Duration) *Service {
	sessionTTL := defaultSessionTTL
	if len(sessionTTLs) > 0 && sessionTTLs[0] > 0 {
		sessionTTL = sessionTTLs[0]
	}
	return &Service{
		pool:       pool,
		queries:    sqlcgen.New(pool),
		now:        time.Now,
		sessionTTL: sessionTTL,
		leaseTTL:   defaultLeaseTTL,
	}
}

func (s *Service) SetCacheInvalidator(catalogInvalidator CatalogCacheInvalidator) {
	s.catalogInvalidator = catalogInvalidator
}

type CreateInput struct {
	ID                    uuid.UUID
	UserID                int64
	ParentID              *uuid.UUID
	Name                  string
	ExpectedSize          int64
	ExpectedHashAlgorithm *string
	ExpectedHashValue     *string
	MIMEType              *string
	ModTime               time.Time
	Encryption            bool
	EncryptionKeyVersion  *int32
	ConflictPolicy        sqlcgen.NameConflictPolicy
	PartSize              int64
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*sqlcgen.UploadSession, error) {
	if in.UserID <= 0 || in.ExpectedSize < -1 {
		return nil, ErrInvalidInput
	}
	name, normalized, err := catalog.NormalizeName(in.Name)
	if err != nil {
		return nil, err
	}
	if (in.ExpectedHashAlgorithm == nil) != (in.ExpectedHashValue == nil) {
		return nil, ErrInvalidInput
	}
	if in.ExpectedHashAlgorithm != nil {
		algorithm, value, err := normalizeExpectedHash(*in.ExpectedHashAlgorithm, *in.ExpectedHashValue)
		if err != nil {
			return nil, err
		}
		in.ExpectedHashAlgorithm = &algorithm
		in.ExpectedHashValue = &value
	}
	if in.Encryption != (in.EncryptionKeyVersion != nil) {
		return nil, ErrInvalidInput
	}
	if in.ConflictPolicy == "" {
		in.ConflictPolicy = sqlcgen.NameConflictPolicyFail
	}
	switch in.ConflictPolicy {
	case sqlcgen.NameConflictPolicyFail, sqlcgen.NameConflictPolicyReplace, sqlcgen.NameConflictPolicyRename:
		// Applied atomically during publication.
	default:
		return nil, ErrInvalidInput
	}
	if in.ParentID != nil {
		if _, err := s.queries.GetActiveFolderForUser(ctx, sqlcgen.GetActiveFolderForUserParams{
			FolderID: dbtypes.UUID(*in.ParentID),
			UserID:   in.UserID,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrInvalidParent
			}
			return nil, fmt.Errorf("get upload parent: %w", err)
		}
	}
	if in.PartSize <= 0 {
		in.PartSize = defaultPartSize
	}
	modTime := in.ModTime
	if modTime.IsZero() {
		modTime = s.now().UTC()
	}
	id := in.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	return s.queries.CreateUploadSession(ctx, sqlcgen.CreateUploadSessionParams{
		ID:                    dbtypes.UUID(id),
		UserID:                in.UserID,
		ParentID:              dbtypes.OptionalUUID(in.ParentID),
		Name:                  name,
		NormalizedName:        normalized,
		ExpectedSize:          in.ExpectedSize,
		ExpectedHashAlgorithm: dbtypes.OptionalText(in.ExpectedHashAlgorithm),
		ExpectedHashValue:     dbtypes.OptionalText(in.ExpectedHashValue),
		MimeType:              dbtypes.OptionalText(in.MIMEType),
		ModTime:               dbtypes.Time(modTime.UTC()),
		Encryption:            in.Encryption,
		EncryptionKeyVersion:  dbtypes.OptionalInt4(in.EncryptionKeyVersion),
		ConflictPolicy:        in.ConflictPolicy,
		PartSize:              in.PartSize,
		ExpiresAt:             dbtypes.Time(s.now().UTC().Add(s.sessionTTL)),
	})
}

func (s *Service) Get(ctx context.Context, userID int64, uploadID uuid.UUID) (*sqlcgen.UploadSession, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	session, err := s.queries.GetUploadSessionForUser(ctx, sqlcgen.GetUploadSessionForUserParams{
		UploadID: dbtypes.UUID(uploadID),
		UserID:   userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get upload: %w", err)
	}
	return session, nil
}

func (s *Service) GetAnyOwner(ctx context.Context, uploadID uuid.UUID) (*sqlcgen.UploadSession, error) {
	if uploadID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	session, err := s.queries.GetUploadSessionAnyOwner(ctx, dbtypes.UUID(uploadID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get upload by id: %w", err)
	}
	return session, nil
}

// GetPart returns one upload part after verifying ownership of its session.
func (s *Service) GetPart(ctx context.Context, userID int64, uploadID uuid.UUID, partNo int32) (*sqlcgen.UploadPart, error) {
	if userID <= 0 || uploadID == uuid.Nil || partNo <= 0 {
		return nil, ErrInvalidInput
	}
	if _, err := s.Get(ctx, userID, uploadID); err != nil {
		return nil, err
	}
	part, err := s.queries.GetUploadPart(ctx, sqlcgen.GetUploadPartParams{
		UploadID: dbtypes.UUID(uploadID),
		PartNo:   partNo,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get upload part: %w", err)
	}
	return part, nil
}

type ClaimPartInput struct {
	UserID    int64
	UploadID  uuid.UUID
	PartNo    int32
	ChannelID int64
	PlainSize int64
	Checksum  *string
}

type ClaimPartResult struct {
	Part       *sqlcgen.UploadPart
	LeaseToken uuid.UUID
	Existing   bool
}

func (s *Service) ClaimPart(ctx context.Context, in ClaimPartInput) (*ClaimPartResult, error) {
	if in.UserID <= 0 || in.PartNo <= 0 || in.ChannelID == 0 || in.PlainSize < 0 {
		return nil, ErrInvalidInput
	}
	if in.Checksum != nil {
		checksum, err := normalizeDigest(*in.Checksum)
		if err != nil {
			return nil, err
		}
		in.Checksum = &checksum
	}
	session, err := s.Get(ctx, in.UserID, in.UploadID)
	if err != nil {
		return nil, err
	}
	if session.State != sqlcgen.UploadStateOpen {
		return nil, ErrInvalidState
	}
	if !session.ExpiresAt.Time.After(s.now()) {
		return nil, ErrExpired
	}
	if err := validatePartShape(session, in.PartNo, in.PlainSize); err != nil {
		return nil, err
	}
	if _, err := s.queries.GetChannelForUser(ctx, sqlcgen.GetChannelForUserParams{
		UserID:    in.UserID,
		ChannelID: in.ChannelID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidChannel
		}
		return nil, fmt.Errorf("get upload channel: %w", err)
	}

	existing, err := s.queries.GetUploadPart(ctx, sqlcgen.GetUploadPartParams{
		UploadID: dbtypes.UUID(in.UploadID),
		PartNo:   in.PartNo,
	})
	if err == nil && existing.State == sqlcgen.UploadPartStateStored {
		if existing.PlainSize == in.PlainSize && (in.Checksum == nil || optionalTextEqual(existing.Checksum, in.Checksum)) {
			return &ClaimPartResult{Part: existing, Existing: true}, nil
		}
		return nil, ErrPartConflict
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get upload part: %w", err)
	}

	leaseToken := uuid.New()
	part, err := s.queries.ClaimUploadPart(ctx, sqlcgen.ClaimUploadPartParams{
		UploadID:       dbtypes.UUID(in.UploadID),
		PartNo:         in.PartNo,
		ChannelID:      in.ChannelID,
		PlainSize:      in.PlainSize,
		Checksum:       dbtypes.OptionalText(in.Checksum),
		LeaseToken:     dbtypes.UUID(leaseToken),
		LeaseExpiresAt: dbtypes.Time(s.now().UTC().Add(s.leaseTTL)),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPartBusy
	}
	if err != nil {
		return nil, fmt.Errorf("claim upload part: %w", err)
	}
	return &ClaimPartResult{Part: part, LeaseToken: leaseToken}, nil
}

type StorePartInput struct {
	UploadID    uuid.UUID
	PartNo      int32
	LeaseToken  uuid.UUID
	MessageID   int64
	StoredSize  int64
	Checksum    string
	Salt        *string
	BlockHashes []byte
}

type RenewPartInput struct {
	UploadID   uuid.UUID
	PartNo     int32
	LeaseToken uuid.UUID
}

func (s *Service) RenewPart(ctx context.Context, in RenewPartInput) error {
	if in.UploadID == uuid.Nil || in.PartNo <= 0 || in.LeaseToken == uuid.Nil {
		return ErrInvalidInput
	}
	updated, err := s.queries.RenewUploadPartLease(ctx, sqlcgen.RenewUploadPartLeaseParams{
		LeaseExpiresAt: dbtypes.Time(s.now().UTC().Add(s.leaseTTL)),
		UploadID:       dbtypes.UUID(in.UploadID),
		PartNo:         in.PartNo,
		LeaseToken:     dbtypes.UUID(in.LeaseToken),
	})
	if err != nil {
		return fmt.Errorf("renew upload part lease: %w", err)
	}
	if updated == 0 {
		return ErrLeaseLost
	}
	return nil
}

func (s *Service) StorePart(ctx context.Context, in StorePartInput) (*sqlcgen.UploadPart, error) {
	if in.PartNo <= 0 || in.LeaseToken == uuid.Nil || in.MessageID <= 0 || in.StoredSize < 0 || (len(in.BlockHashes) > 0 && len(in.BlockHashes)%treehash.DigestSize != 0) {
		return nil, ErrInvalidInput
	}
	if (strings.TrimSpace(in.Checksum) == "") != (len(in.BlockHashes) == 0) {
		return nil, ErrInvalidInput
	}
	var checksum *string
	if strings.TrimSpace(in.Checksum) != "" {
		normalized, err := normalizeDigest(in.Checksum)
		if err != nil {
			return nil, err
		}
		checksum = &normalized
	}
	part, err := s.queries.MarkUploadPartStored(ctx, sqlcgen.MarkUploadPartStoredParams{
		MessageID:   dbtypes.Int8(in.MessageID),
		StoredSize:  dbtypes.Int8(in.StoredSize),
		Checksum:    dbtypes.OptionalText(checksum),
		Salt:        dbtypes.OptionalText(in.Salt),
		BlockHashes: append([]byte(nil), in.BlockHashes...),
		UploadID:    dbtypes.UUID(in.UploadID),
		PartNo:      in.PartNo,
		LeaseToken:  dbtypes.UUID(in.LeaseToken),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLeaseLost
	}
	if err != nil {
		return nil, fmt.Errorf("store upload part: %w", err)
	}
	return part, nil
}

type FailPartInput struct {
	UploadID   uuid.UUID
	PartNo     int32
	LeaseToken uuid.UUID
	ErrorCode  string
}

// FailPart releases a part lease after a transport or validation failure. The
// lease token prevents a stale uploader from overwriting a newer attempt.
func (s *Service) FailPart(ctx context.Context, in FailPartInput) (*sqlcgen.UploadPart, error) {
	if in.UploadID == uuid.Nil || in.PartNo <= 0 || in.LeaseToken == uuid.Nil || strings.TrimSpace(in.ErrorCode) == "" {
		return nil, ErrInvalidInput
	}
	part, err := s.queries.MarkUploadPartFailed(ctx, sqlcgen.MarkUploadPartFailedParams{
		ErrorCode:  dbtypes.Text(strings.TrimSpace(in.ErrorCode)),
		UploadID:   dbtypes.UUID(in.UploadID),
		PartNo:     in.PartNo,
		LeaseToken: dbtypes.UUID(in.LeaseToken),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLeaseLost
	}
	if err != nil {
		return nil, fmt.Errorf("fail upload part: %w", err)
	}
	return part, nil
}

func (s *Service) Complete(ctx context.Context, userID int64, uploadID uuid.UUID) (*sqlcgen.File, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin upload completion: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)

	session, err := q.LockUploadSessionForCompletion(ctx, sqlcgen.LockUploadSessionForCompletionParams{
		UploadID: dbtypes.UUID(uploadID),
		UserID:   userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock upload: %w", err)
	}
	if session.State == sqlcgen.UploadStateCompleted {
		fileID, ok := dbtypes.GoogleUUID(session.FileID)
		if !ok {
			return nil, fmt.Errorf("completed upload has no file id")
		}
		file, err := q.GetFileForUser(ctx, sqlcgen.GetFileForUserParams{FileID: dbtypes.UUID(fileID), UserID: userID})
		if err != nil {
			return nil, fmt.Errorf("get completed file: %w", err)
		}
		return file, nil
	}
	if session.State != sqlcgen.UploadStateOpen {
		return nil, ErrInvalidState
	}
	if !session.ExpiresAt.Time.After(s.now()) {
		return nil, ErrExpired
	}

	completedSize, err := validateStoredParts(ctx, tx, session)
	if err != nil {
		return nil, err
	}
	if session.ExpectedSize < 0 {
		session, err = q.FinalizeUploadExpectedSize(ctx, sqlcgen.FinalizeUploadExpectedSizeParams{
			ExpectedSize: completedSize,
			UploadID:     dbtypes.UUID(uploadID),
			UserID:       userID,
		})
		if err != nil {
			return nil, fmt.Errorf("finalize upload size: %w", err)
		}
	}
	blockHashSets, err := q.ListStoredUploadPartHashes(ctx, dbtypes.UUID(uploadID))
	if err != nil {
		return nil, fmt.Errorf("list stored upload hashes: %w", err)
	}
	var hashAlgorithm, hashValue *string
	concatenated := make([]byte, 0)
	sawUnhashed := false
	for _, blockHashes := range blockHashSets {
		if len(blockHashes) == 0 {
			sawUnhashed = true
			if len(concatenated) > 0 || session.ExpectedHashAlgorithm.Valid {
				return nil, ErrIncomplete
			}
			continue
		}
		if sawUnhashed || len(blockHashes)%treehash.DigestSize != 0 {
			return nil, ErrIncomplete
		}
		concatenated = append(concatenated, blockHashes...)
	}
	if len(concatenated) > 0 {
		algorithm := string(treehash.TypeBlake3)
		value := treehash.SumToHex(treehash.ComputeTreeHash(concatenated))
		hashAlgorithm, hashValue = &algorithm, &value
	}
	if session.ExpectedHashAlgorithm.Valid {
		if hashAlgorithm == nil || !strings.EqualFold(session.ExpectedHashAlgorithm.String, *hashAlgorithm) || !strings.EqualFold(session.ExpectedHashValue.String, *hashValue) {
			return nil, ErrHashMismatch
		}
	}
	replacedID, err := prepareConflictPolicy(ctx, tx, session)
	if err != nil {
		return nil, err
	}
	if _, err := q.MarkUploadCompleting(ctx, sqlcgen.MarkUploadCompletingParams{
		UploadID: dbtypes.UUID(uploadID),
		UserID:   userID,
	}); err != nil {
		return nil, fmt.Errorf("mark upload completing: %w", err)
	}

	fileID := uuid.New()
	file, err := q.InsertFileFromUpload(ctx, sqlcgen.InsertFileFromUploadParams{
		FileID:        dbtypes.UUID(fileID),
		HashAlgorithm: dbtypes.OptionalText(hashAlgorithm),
		HashValue:     dbtypes.OptionalText(hashValue),
		UploadID:      dbtypes.UUID(uploadID),
		UserID:        userID,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrNameConflict
		}
		return nil, fmt.Errorf("create file from upload: %w", err)
	}
	inserted, err := q.InsertFilePartsFromUpload(ctx, sqlcgen.InsertFilePartsFromUploadParams{
		FileID:   dbtypes.UUID(fileID),
		UploadID: dbtypes.UUID(uploadID),
	})
	if err != nil {
		return nil, fmt.Errorf("copy upload parts: %w", err)
	}
	expectedParts := expectedPartCount(session.ExpectedSize, session.PartSize)
	if inserted != expectedParts {
		return nil, fmt.Errorf("copied %d upload parts, expected %d: %w", inserted, expectedParts, ErrIncomplete)
	}
	if _, err := q.CompleteUploadSession(ctx, sqlcgen.CompleteUploadSessionParams{
		FileID:   dbtypes.UUID(fileID),
		UploadID: dbtypes.UUID(uploadID),
		UserID:   userID,
	}); err != nil {
		return nil, fmt.Errorf("complete upload session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit upload completion: %w", err)
	}
	if replacedID != nil {
		if s.catalogInvalidator != nil {
			s.catalogInvalidator.InvalidateFiles(ctx, userID, *replacedID)
		}
	}
	return file, nil
}

func prepareConflictPolicy(ctx context.Context, tx pgx.Tx, session *sqlcgen.UploadSession) (*uuid.UUID, error) {
	if session == nil {
		return nil, ErrInvalidInput
	}
	queries := sqlcgen.New(tx)
	if err := queries.AcquireAdvisoryTransactionLock(ctx, uploadDestinationLockID(session)); err != nil {
		return nil, fmt.Errorf("lock upload destination: %w", err)
	}

	existing, err := queries.LockUploadDestinationConflict(ctx, sqlcgen.LockUploadDestinationConflictParams{
		UserID: session.UserID, ParentID: session.ParentID, NormalizedName: session.NormalizedName,
	})
	hasConflict := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check upload destination conflict: %w", err)
	}

	switch session.ConflictPolicy {
	case sqlcgen.NameConflictPolicyFail:
		if hasConflict {
			return nil, ErrNameConflict
		}
		return nil, nil
	case sqlcgen.NameConflictPolicyReplace:
		if !hasConflict {
			return nil, nil
		}
		if existing.Kind != sqlcgen.FileKindFile {
			return nil, ErrNameConflict
		}
		existingID, ok := dbtypes.GoogleUUID(existing.ID)
		if !ok {
			return nil, ErrNameConflict
		}
		count, err := queries.MarkActiveFileDeletionPendingForReplace(ctx, sqlcgen.MarkActiveFileDeletionPendingForReplaceParams{
			FileID: dbtypes.UUID(existingID), UserID: session.UserID,
		})
		if err != nil {
			return nil, fmt.Errorf("mark replaced file for cleanup: %w", err)
		}
		if count != 1 {
			return nil, ErrNameConflict
		}
		if err := queries.RevokeActiveSharesForFile(ctx, sqlcgen.RevokeActiveSharesForFileParams{
			FileID: dbtypes.UUID(existingID), UserID: session.UserID,
		}); err != nil {
			return nil, fmt.Errorf("revoke replaced file shares: %w", err)
		}
		return &existingID, nil
	case sqlcgen.NameConflictPolicyRename:
		if !hasConflict {
			return nil, nil
		}
		name, normalized, err := nextAvailableUploadName(ctx, queries, session)
		if err != nil {
			return nil, err
		}
		count, err := queries.RenameUploadSession(ctx, sqlcgen.RenameUploadSessionParams{
			Name: name, NormalizedName: normalized, UploadID: session.ID, UserID: session.UserID,
		})
		if err != nil {
			return nil, fmt.Errorf("rename upload destination: %w", err)
		}
		if count != 1 {
			return nil, ErrInvalidState
		}
		session.Name = name
		session.NormalizedName = normalized
		return nil, nil
	default:
		return nil, ErrInvalidInput
	}
}

func nextAvailableUploadName(ctx context.Context, queries *sqlcgen.Queries, session *sqlcgen.UploadSession) (string, string, error) {
	names, err := queries.ListActiveNormalizedNames(ctx, sqlcgen.ListActiveNormalizedNamesParams{
		UserID: session.UserID, ParentID: session.ParentID,
	})
	if err != nil {
		return "", "", fmt.Errorf("list upload destination names: %w", err)
	}
	used := make(map[string]struct{}, len(names))
	for _, normalized := range names {
		used[normalized] = struct{}{}
	}

	base, extension := splitUploadName(session.Name)
	for sequence := 1; sequence <= 10000; sequence++ {
		suffix := fmt.Sprintf(" (%d)", sequence)
		candidate := boundedUploadName(base, extension, suffix)
		display, normalized, err := catalog.NormalizeName(candidate)
		if err != nil {
			return "", "", err
		}
		if _, exists := used[normalized]; !exists {
			return display, normalized, nil
		}
	}
	return "", "", ErrNameConflict
}

func splitUploadName(name string) (string, string) {
	index := strings.LastIndex(name, ".")
	if index <= 0 || index == len(name)-1 {
		return name, ""
	}
	return name[:index], name[index:]
}

func boundedUploadName(base, extension, suffix string) string {
	const maxRunes = 255
	extensionRunes := []rune(extension)
	suffixRunes := []rune(suffix)
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

func uploadDestinationLockID(session *sqlcgen.UploadSession) int64 {
	input := make([]byte, 0, 8+16+len("teldrive/upload-destination/"))
	input = append(input, []byte("teldrive/upload-destination/")...)
	var user [8]byte
	binary.BigEndian.PutUint64(user[:], uint64(session.UserID))
	input = append(input, user[:]...)
	if session.ParentID.Valid {
		input = append(input, session.ParentID.Bytes[:]...)
	} else {
		input = append(input, make([]byte, 16)...)
	}
	digest := sha256.Sum256(input)
	return int64(binary.BigEndian.Uint64(digest[:8]))
}

func (s *Service) Abort(ctx context.Context, userID int64, uploadID uuid.UUID) (*sqlcgen.UploadSession, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	session, err := s.Get(ctx, userID, uploadID)
	if err != nil {
		return nil, err
	}
	switch session.State {
	case sqlcgen.UploadStateAborted, sqlcgen.UploadStateExpired:
		return session, nil
	case sqlcgen.UploadStateCompleted:
		return nil, ErrInvalidState
	}
	aborted, err := s.queries.AbortUploadSession(ctx, sqlcgen.AbortUploadSessionParams{
		UploadID: dbtypes.UUID(uploadID),
		UserID:   userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidState
	}
	if err != nil {
		return nil, fmt.Errorf("abort upload: %w", err)
	}
	return aborted, nil
}

func validatePartShape(session *sqlcgen.UploadSession, partNo int32, plainSize int64) error {
	if session.PartSize <= 0 {
		return ErrInvalidInput
	}
	if session.ExpectedSize < 0 {
		if plainSize <= 0 || plainSize > session.PartSize {
			return ErrInvalidInput
		}
		return nil
	}
	if session.ExpectedSize == 0 {
		return ErrInvalidInput
	}
	offset := int64(partNo-1) * session.PartSize
	if offset < 0 || offset >= session.ExpectedSize {
		return ErrInvalidInput
	}
	expected := session.PartSize
	if remaining := session.ExpectedSize - offset; remaining < expected {
		expected = remaining
	}
	if plainSize != expected {
		return ErrInvalidInput
	}
	return nil
}

func validateStoredParts(ctx context.Context, tx pgx.Tx, session *sqlcgen.UploadSession) (int64, error) {
	queries := sqlcgen.New(tx)
	summary, err := queries.GetAllUploadPartSummary(ctx, session.ID)
	if err != nil {
		return 0, fmt.Errorf("summarize upload parts: %w", err)
	}
	if session.ExpectedSize >= 0 {
		expected := expectedPartCount(session.ExpectedSize, session.PartSize)
		if summary.TotalParts != expected || summary.StoredParts != expected || summary.StoredPlainSize != session.ExpectedSize {
			return 0, ErrIncomplete
		}
		if expected > 0 && (summary.MinPartNo != 1 || int64(summary.MaxPartNo) != expected) {
			return 0, ErrIncomplete
		}
		return session.ExpectedSize, nil
	}
	if summary.TotalParts != summary.StoredParts {
		return 0, ErrIncomplete
	}
	if summary.StoredParts > 0 && (summary.MinPartNo != 1 || int64(summary.MaxPartNo) != summary.StoredParts) {
		return 0, ErrIncomplete
	}
	invalidParts, err := queries.CountInvalidOpenEndedUploadParts(ctx, sqlcgen.CountInvalidOpenEndedUploadPartsParams{
		UploadID: session.ID,
		PartSize: session.PartSize,
	})
	if err != nil {
		return 0, fmt.Errorf("validate open-ended upload parts: %w", err)
	}
	if invalidParts != 0 {
		return 0, ErrIncomplete
	}
	return summary.StoredPlainSize, nil
}

func expectedPartCount(size, partSize int64) int64 {
	if size == 0 {
		return 0
	}
	return (size + partSize - 1) / partSize
}

func normalizeExpectedHash(algorithm, value string) (string, string, error) {
	algorithm = strings.ToLower(strings.TrimSpace(algorithm))
	if algorithm != string(treehash.TypeBlake3) {
		return "", "", ErrInvalidInput
	}
	normalized, err := normalizeDigest(value)
	if err != nil {
		return "", "", err
	}
	return algorithm, normalized, nil
}

func normalizeDigest(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	digest, err := hex.DecodeString(value)
	if err != nil || len(digest) != treehash.DigestSize {
		return "", ErrInvalidInput
	}
	return value, nil
}

func optionalTextEqual(stored pgtype.Text, expected *string) bool {
	if expected == nil {
		return !stored.Valid
	}
	return stored.Valid && stored.String == *expected
}
