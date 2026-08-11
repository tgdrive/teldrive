package transfer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/contentcrypto"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	"github.com/tgdrive/teldrive/v2/internal/treehash"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

var (
	ErrInvalidUpload       = errors.New("invalid upload request")
	ErrBodyTooShort        = errors.New("upload body is shorter than Content-Length")
	ErrBodyTooLong         = errors.New("upload body is longer than Content-Length")
	ErrChecksumMismatch    = errors.New("plaintext part checksum mismatch")
	ErrEncryptionKey       = errors.New("encryption key is unavailable")
	ErrStoredSizeMismatch  = errors.New("stored Telegram part size mismatch")
	ErrUploadNotConfigured = errors.New("upload pipeline is not configured")
)

// UploadCatalog is the durable upload-session boundary required by Pipeline.
type UploadCatalog interface {
	Get(context.Context, int64, uuid.UUID) (*sqlcgen.UploadSession, error)
	GetPart(context.Context, int64, uuid.UUID, int32) (*sqlcgen.UploadPart, error)
	ClaimPart(context.Context, uploads.ClaimPartInput) (*uploads.ClaimPartResult, error)
	StorePart(context.Context, uploads.StorePartInput) (*sqlcgen.UploadPart, error)
	FailPart(context.Context, uploads.FailPartInput) (*sqlcgen.UploadPart, error)
}

// ChannelResolver chooses an owned channel and performs rollover when needed.
type ChannelResolver interface {
	Resolve(context.Context, int64, int64) (int64, error)
}

// KeyProvider resolves versioned server-managed content-encryption keys.
type KeyProvider interface {
	Key(context.Context, int64, int32) (string, error)
}

type Config struct {
	UploadThreads      int
	RandomizePartNames bool
	Random             io.Reader
}

type Pipeline struct {
	catalog  UploadCatalog
	channels ChannelResolver
	storage  telegramstore.Storage
	keys     KeyProvider
	config   Config
}

func NewPipeline(catalog UploadCatalog, channels ChannelResolver, storage telegramstore.Storage, keys KeyProvider, cfg Config) *Pipeline {
	if cfg.Random == nil {
		cfg.Random = rand.Reader
	}
	return &Pipeline{catalog: catalog, channels: channels, storage: storage, keys: keys, config: cfg}
}

type UploadPartRequest struct {
	UserID             int64
	UploadID           uuid.UUID
	PartNo             int32
	RequestedChannelID int64
	PlainSize          int64
	Checksum           *string
	Body               io.Reader
}

type UploadPartResult struct {
	Part     *sqlcgen.UploadPart
	Existing bool
}

func (p *Pipeline) UploadPart(ctx context.Context, request UploadPartRequest) (*UploadPartResult, error) {
	if p.catalog == nil || p.channels == nil || p.storage == nil {
		return nil, ErrUploadNotConfigured
	}
	if request.UserID <= 0 || request.UploadID == uuid.Nil || request.PartNo <= 0 || request.PlainSize <= 0 || request.Body == nil {
		return nil, ErrInvalidUpload
	}
	checksum, err := normalizeOptionalChecksum(request.Checksum)
	if err != nil {
		return nil, err
	}
	request.Checksum = checksum

	session, err := p.catalog.Get(ctx, request.UserID, request.UploadID)
	if err != nil {
		return nil, err
	}
	if existing, err := p.catalog.GetPart(ctx, request.UserID, request.UploadID, request.PartNo); err == nil {
		if existing.State == sqlcgen.UploadPartStateStored {
			if existing.PlainSize != request.PlainSize || !checksumMatches(existing.Checksum.String, existing.Checksum.Valid, request.Checksum) {
				return nil, uploads.ErrPartConflict
			}
			return &UploadPartResult{Part: existing, Existing: true}, nil
		}
	} else if !errors.Is(err, uploads.ErrNotFound) {
		return nil, err
	}

	channelID, err := p.channels.Resolve(ctx, request.UserID, request.RequestedChannelID)
	if err != nil {
		return nil, err
	}
	claim, err := p.catalog.ClaimPart(ctx, uploads.ClaimPartInput{
		UserID:    request.UserID,
		UploadID:  request.UploadID,
		PartNo:    request.PartNo,
		ChannelID: channelID,
		PlainSize: request.PlainSize,
		Checksum:  request.Checksum,
	})
	if err != nil {
		return nil, err
	}
	if claim.Existing {
		return &UploadPartResult{Part: claim.Part, Existing: true}, nil
	}

	exact := newExactReader(ctx, request.Body, request.PlainSize)
	hasher := treehash.NewBlockHasher()
	plainReader := io.TeeReader(exact, hasher)
	storedReader := io.Reader(plainReader)
	storedSize := request.PlainSize
	var salt *string

	if session.Encryption {
		if p.keys == nil || !session.EncryptionKeyVersion.Valid {
			return nil, p.failPart(ctx, request, claim.LeaseToken, "encryption_key_missing", ErrEncryptionKey)
		}
		key, keyErr := p.keys.Key(ctx, request.UserID, session.EncryptionKeyVersion.Int32)
		if keyErr != nil || key == "" {
			return nil, p.failPart(ctx, request, claim.LeaseToken, "encryption_key_missing", errors.Join(ErrEncryptionKey, keyErr))
		}
		generatedSalt, saltErr := generateSalt(p.config.Random)
		if saltErr != nil {
			return nil, p.failPart(ctx, request, claim.LeaseToken, "salt_generation_failed", saltErr)
		}
		cipher, cipherErr := contentcrypto.NewCipher(key, generatedSalt)
		if cipherErr != nil {
			return nil, p.failPart(ctx, request, claim.LeaseToken, "cipher_initialization_failed", cipherErr)
		}
		encrypted, cipherErr := cipher.EncryptData(plainReader)
		if cipherErr != nil {
			return nil, p.failPart(ctx, request, claim.LeaseToken, "cipher_initialization_failed", cipherErr)
		}
		storedReader = encrypted
		storedSize = contentcrypto.EncryptedSize(request.PlainSize)
		salt = &generatedSalt
	}

	stored, err := p.storage.Upload(ctx, telegramstore.UploadRequest{
		UserID:    request.UserID,
		ChannelID: channelID,
		Name:      p.partName(request.UploadID, request.PartNo),
		Reader:    storedReader,
		Size:      storedSize,
		Threads:   p.config.UploadThreads,
	})
	if err != nil {
		return nil, p.failPart(ctx, request, claim.LeaseToken, "telegram_upload_failed", err)
	}
	if stored.ChannelID != channelID || stored.MessageID <= 0 || stored.Size != storedSize {
		cleanupErr := p.deleteUploaded(ctx, request.UserID, stored)
		return nil, p.failPart(ctx, request, claim.LeaseToken, "telegram_size_mismatch", errors.Join(ErrStoredSizeMismatch, cleanupErr))
	}
	if err := exact.Verify(); err != nil {
		cleanupErr := p.deleteUploaded(ctx, request.UserID, stored)
		return nil, p.failPart(ctx, request, claim.LeaseToken, "body_size_mismatch", errors.Join(err, cleanupErr))
	}

	blockHashes := hasher.Sum()
	if len(blockHashes) == 0 || len(blockHashes)%treehash.DigestSize != 0 {
		cleanupErr := p.deleteUploaded(ctx, request.UserID, stored)
		return nil, p.failPart(ctx, request, claim.LeaseToken, "hash_generation_failed", errors.Join(ErrInvalidUpload, cleanupErr))
	}
	actualChecksum := treehash.SumToHex(treehash.ComputeTreeHash(blockHashes))
	if request.Checksum != nil && !strings.EqualFold(*request.Checksum, actualChecksum) {
		cleanupErr := p.deleteUploaded(ctx, request.UserID, stored)
		return nil, p.failPart(ctx, request, claim.LeaseToken, "checksum_mismatch", errors.Join(ErrChecksumMismatch, cleanupErr))
	}

	part, err := p.catalog.StorePart(ctx, uploads.StorePartInput{
		UploadID:    request.UploadID,
		PartNo:      request.PartNo,
		LeaseToken:  claim.LeaseToken,
		MessageID:   stored.MessageID,
		StoredSize:  stored.Size,
		Checksum:    actualChecksum,
		Salt:        salt,
		BlockHashes: append([]byte(nil), blockHashes...),
	})
	if err != nil {
		cleanupErr := p.deleteUploaded(ctx, request.UserID, stored)
		return nil, errors.Join(err, cleanupErr)
	}
	return &UploadPartResult{Part: part}, nil
}

func (p *Pipeline) failPart(ctx context.Context, request UploadPartRequest, leaseToken uuid.UUID, code string, cause error) error {
	_, failErr := p.catalog.FailPart(ctx, uploads.FailPartInput{
		UploadID:   request.UploadID,
		PartNo:     request.PartNo,
		LeaseToken: leaseToken,
		ErrorCode:  code,
	})
	if failErr != nil && !errors.Is(failErr, uploads.ErrLeaseLost) {
		return errors.Join(cause, failErr)
	}
	return cause
}

func (p *Pipeline) deleteUploaded(ctx context.Context, userID int64, part telegramstore.StoredPart) error {
	if part.ChannelID == 0 || part.MessageID <= 0 {
		return nil
	}
	if err := p.storage.DeleteMessages(ctx, userID, part.ChannelID, []int64{part.MessageID}); err != nil {
		return fmt.Errorf("compensate Telegram upload: %w", err)
	}
	return nil
}

func (p *Pipeline) partName(uploadID uuid.UUID, partNo int32) string {
	var material string
	if p.config.RandomizePartNames {
		material = uuid.NewString()
	} else {
		material = fmt.Sprintf("%s:%d", uploadID, partNo)
	}
	digest := sha256.Sum256([]byte(material))
	return hex.EncodeToString(digest[:])
}

func generateSalt(random io.Reader) (string, error) {
	if random == nil {
		return "", errors.New("random source is required")
	}
	seed := make([]byte, 32)
	if _, err := io.ReadFull(random, seed); err != nil {
		return "", fmt.Errorf("read encryption salt entropy: %w", err)
	}
	digest := sha256.Sum256(seed)
	return base64.URLEncoding.EncodeToString(digest[:]), nil
}

func normalizeOptionalChecksum(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*value))
	digest, err := hex.DecodeString(normalized)
	if err != nil || len(digest) != treehash.DigestSize {
		return nil, ErrInvalidUpload
	}
	return &normalized, nil
}

func checksumMatches(stored string, valid bool, expected *string) bool {
	if expected == nil {
		return true
	}
	return valid && strings.EqualFold(stored, *expected)
}

type exactReader struct {
	ctx       context.Context
	source    io.Reader
	remaining int64
}

func newExactReader(ctx context.Context, source io.Reader, size int64) *exactReader {
	return &exactReader{ctx: ctx, source: source, remaining: size}
}

func (r *exactReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.source.Read(p)
	r.remaining -= int64(n)
	if errors.Is(err, io.EOF) && r.remaining > 0 {
		return n, ErrBodyTooShort
	}
	if n == 0 && err == nil {
		return 0, io.ErrNoProgress
	}
	return n, err
}

func (r *exactReader) Verify() error {
	if r.remaining != 0 {
		return ErrBodyTooShort
	}
	var probe [1]byte
	n, err := r.source.Read(probe[:])
	if n > 0 {
		return ErrBodyTooLong
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// StaticKeyProvider is suitable for configuration-backed key rotation. New
// uploads reference a version while old files remain decryptable.
type StaticKeyProvider map[int32]string

func (p StaticKeyProvider) Key(_ context.Context, _ int64, version int32) (string, error) {
	key, ok := p[version]
	if !ok || key == "" {
		return "", ErrEncryptionKey
	}
	return key, nil
}
