//go:build integration

package transfer_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/contentcrypto"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
	"github.com/tgdrive/teldrive/v2/internal/treehash"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

func TestUploadPipelinePlaintextAndRetry(t *testing.T) {
	db := testpostgres.New(t)
	seedTransferOwner(t, db.Pool, 1001, 9001)
	catalog := uploads.NewService(db.Pool)
	body := []byte("plain Telegram upload")
	session, err := catalog.Create(context.Background(), uploads.CreateInput{
		UserID: 1001, Name: "plain.bin", ExpectedSize: int64(len(body)), PartSize: int64(len(body)),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	uploadID, ok := dbtypes.GoogleUUID(session.ID)
	if !ok {
		t.Fatal("missing upload id")
	}
	checksum := checksumFor(body)
	storage := &memoryStorage{}
	resolver := &fixedResolver{channelID: 9001}
	pipeline := transfer.NewPipeline(catalog, resolver, storage, nil, transfer.Config{})

	result, err := pipeline.UploadPart(context.Background(), transfer.UploadPartRequest{
		UserID: 1001, UploadID: uploadID, PartNo: 1, PlainSize: int64(len(body)), Checksum: &checksum, Body: bytes.NewReader(body),
	})
	if err != nil {
		t.Fatalf("UploadPart() error = %v", err)
	}
	if result.Existing || result.Part.State != sqlcgen.UploadPartStateStored {
		t.Fatalf("UploadPart() result = %#v", result)
	}
	if got := storage.payload(0); !bytes.Equal(got, body) {
		t.Fatalf("stored payload = %q", got)
	}
	if !result.Part.Checksum.Valid || result.Part.Checksum.String != checksum || len(result.Part.BlockHashes) != treehash.DigestSize {
		t.Fatalf("stored metadata = %#v", result.Part)
	}

	retry, err := pipeline.UploadPart(context.Background(), transfer.UploadPartRequest{
		UserID: 1001, UploadID: uploadID, PartNo: 1, PlainSize: int64(len(body)), Body: panicReader{},
	})
	if err != nil || !retry.Existing {
		t.Fatalf("retry = %#v, %v", retry, err)
	}
	if storage.uploadCount() != 1 || resolver.calls != 1 {
		t.Fatalf("retry performed side effects: uploads=%d resolves=%d", storage.uploadCount(), resolver.calls)
	}

	file, err := catalog.Complete(context.Background(), 1001, uploadID)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	expectedFileHash := treehash.SumToHex(treehash.ComputeTreeHash(result.Part.BlockHashes))
	if !file.HashValue.Valid || file.HashValue.String != expectedFileHash {
		t.Fatalf("file hash = %#v, want %s", file.HashValue, expectedFileHash)
	}
}

func TestUploadPipelineHashingDisabled(t *testing.T) {
	db := testpostgres.New(t)
	seedTransferOwner(t, db.Pool, 1001, 9001)
	catalog := uploads.NewService(db.Pool)
	body := []byte("upload without hashing")
	session, err := catalog.Create(context.Background(), uploads.CreateInput{
		UserID: 1001, Name: "unhashed.bin", ExpectedSize: int64(len(body)), PartSize: int64(len(body)),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	uploadID, _ := dbtypes.GoogleUUID(session.ID)
	pipeline := transfer.NewPipeline(catalog, &fixedResolver{channelID: 9001}, &memoryStorage{}, nil, transfer.Config{DisableHashing: true})
	result, err := pipeline.UploadPart(context.Background(), transfer.UploadPartRequest{
		UserID: 1001, UploadID: uploadID, PartNo: 1, PlainSize: int64(len(body)), Body: bytes.NewReader(body),
	})
	if err != nil {
		t.Fatalf("UploadPart() error = %v", err)
	}
	if result.Part.Checksum.Valid || len(result.Part.BlockHashes) != 0 {
		t.Fatalf("stored hash metadata = (%#v, %x), want empty", result.Part.Checksum, result.Part.BlockHashes)
	}
	file, err := catalog.Complete(context.Background(), 1001, uploadID)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if file.HashAlgorithm.Valid || file.HashValue.Valid {
		t.Fatalf("file hash = (%#v, %#v), want empty", file.HashAlgorithm, file.HashValue)
	}
}

func TestUploadPipelineEncryptedCompatibility(t *testing.T) {
	db := testpostgres.New(t)
	seedTransferOwner(t, db.Pool, 1001, 9001)
	catalog := uploads.NewService(db.Pool)
	body := bytes.Repeat([]byte("encrypted-block-"), 6000)
	version := int32(7)
	session, err := catalog.Create(context.Background(), uploads.CreateInput{
		UserID: 1001, Name: "encrypted.bin", ExpectedSize: int64(len(body)), PartSize: int64(len(body)),
		Encryption: true, EncryptionKeyVersion: &version,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	uploadID, _ := dbtypes.GoogleUUID(session.ID)
	storage := &memoryStorage{}
	pipeline := transfer.NewPipeline(catalog, &fixedResolver{channelID: 9001}, storage, transfer.StaticKeyProvider{7: "same-key-format-as-original"}, transfer.Config{
		Random: bytes.NewReader(bytes.Repeat([]byte{9}, 32)),
	})
	result, err := pipeline.UploadPart(context.Background(), transfer.UploadPartRequest{
		UserID: 1001, UploadID: uploadID, PartNo: 1, PlainSize: int64(len(body)), Body: bytes.NewReader(body),
	})
	if err != nil {
		t.Fatalf("UploadPart() error = %v", err)
	}
	ciphertext := storage.payload(0)
	if int64(len(ciphertext)) != contentcrypto.EncryptedSize(int64(len(body))) || bytes.Equal(ciphertext, body) {
		t.Fatalf("ciphertext size/content invalid: %d", len(ciphertext))
	}
	if !result.Part.Salt.Valid || result.Part.Salt.String == "" {
		t.Fatalf("encrypted part salt = %#v", result.Part.Salt)
	}
	cipher, err := contentcrypto.NewCipher("same-key-format-as-original", result.Part.Salt.String)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := cipher.DecryptData(io.NopCloser(bytes.NewReader(ciphertext)))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(decrypted)
	if err != nil || !bytes.Equal(plain, body) {
		t.Fatalf("decrypt = %d bytes, %v", len(plain), err)
	}
	if _, err := catalog.Complete(context.Background(), 1001, uploadID); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
}

func TestUploadPipelineChecksumMismatchCompensates(t *testing.T) {
	db := testpostgres.New(t)
	seedTransferOwner(t, db.Pool, 1001, 9001)
	catalog := uploads.NewService(db.Pool)
	body := []byte("checksum body")
	session, err := catalog.Create(context.Background(), uploads.CreateInput{
		UserID: 1001, Name: "bad.bin", ExpectedSize: int64(len(body)), PartSize: int64(len(body)),
	})
	if err != nil {
		t.Fatal(err)
	}
	uploadID, _ := dbtypes.GoogleUUID(session.ID)
	wrong := checksumFor([]byte("different body"))
	storage := &memoryStorage{}
	pipeline := transfer.NewPipeline(catalog, &fixedResolver{channelID: 9001}, storage, nil, transfer.Config{})
	_, err = pipeline.UploadPart(context.Background(), transfer.UploadPartRequest{
		UserID: 1001, UploadID: uploadID, PartNo: 1, PlainSize: int64(len(body)), Checksum: &wrong, Body: bytes.NewReader(body),
	})
	if !errors.Is(err, transfer.ErrChecksumMismatch) {
		t.Fatalf("UploadPart() error = %v", err)
	}
	if len(storage.deleted) != 1 || storage.deleted[0] != 1 {
		t.Fatalf("deleted messages = %#v", storage.deleted)
	}
	part, err := catalog.GetPart(context.Background(), 1001, uploadID, 1)
	if err != nil {
		t.Fatalf("GetPart() error = %v", err)
	}
	if part.State != sqlcgen.UploadPartStateFailed || !part.LastErrorCode.Valid || part.LastErrorCode.String != "checksum_mismatch" {
		t.Fatalf("failed part = %#v", part)
	}
}

func TestUploadPipelineRejectsLongBodyAndCompensates(t *testing.T) {
	db := testpostgres.New(t)
	seedTransferOwner(t, db.Pool, 1001, 9001)
	catalog := uploads.NewService(db.Pool)
	session, err := catalog.Create(context.Background(), uploads.CreateInput{
		UserID: 1001, Name: "long.bin", ExpectedSize: 4, PartSize: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	uploadID, _ := dbtypes.GoogleUUID(session.ID)
	storage := &memoryStorage{}
	pipeline := transfer.NewPipeline(catalog, &fixedResolver{channelID: 9001}, storage, nil, transfer.Config{})
	_, err = pipeline.UploadPart(context.Background(), transfer.UploadPartRequest{
		UserID: 1001, UploadID: uploadID, PartNo: 1, PlainSize: 4, Body: bytes.NewBufferString("abcde"),
	})
	if !errors.Is(err, transfer.ErrBodyTooLong) {
		t.Fatalf("UploadPart() error = %v", err)
	}
	if len(storage.deleted) != 1 {
		t.Fatalf("expected compensation, deleted=%#v", storage.deleted)
	}
}

func seedTransferOwner(t testing.TB, pool *pgxpool.Pool, userID, channelID int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO users (user_id) VALUES ($1)", userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO channels (channel_id, user_id, name, selected) VALUES ($1, $2, 'storage', true)", channelID, userID); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
}

func checksumFor(data []byte) string {
	hasher := treehash.NewBlockHasher()
	_, _ = hasher.Write(data)
	return treehash.SumToHex(treehash.ComputeTreeHash(hasher.Sum()))
}

type fixedResolver struct {
	channelID int64
	calls     int
}

func (r *fixedResolver) Resolve(_ context.Context, _ int64, requested int64) (int64, error) {
	r.calls++
	if requested != 0 {
		return requested, nil
	}
	return r.channelID, nil
}

type memoryStorage struct {
	mu            sync.Mutex
	uploads       [][]byte
	messages      map[int64][]byte
	deleted       []int64
	rangeRequests []telegramstore.RangeRequest
	nextID        int64
	sessionOpens  int
	sessionCloses int
}

func (s *memoryStorage) OpenDownloadSession(context.Context, int64) (telegramstore.DownloadSession, error) {
	s.mu.Lock()
	s.sessionOpens++
	s.mu.Unlock()
	return &memoryDownloadSession{storage: s}, nil
}

type memoryDownloadSession struct {
	storage *memoryStorage
	closed  bool
}

func (s *memoryDownloadSession) Metadata(_ context.Context, request telegramstore.MetadataRequest) (telegramstore.StoredPart, error) {
	s.storage.mu.Lock()
	defer s.storage.mu.Unlock()
	payload, ok := s.storage.messages[request.MessageID]
	if !ok {
		return telegramstore.StoredPart{}, telegramstore.ErrMessageNotFound
	}
	return telegramstore.StoredPart{ChannelID: request.ChannelID, MessageID: request.MessageID, Size: int64(len(payload))}, nil
}

func (s *memoryDownloadSession) OpenRange(ctx context.Context, request telegramstore.RangeRequest) (io.ReadCloser, error) {
	return s.storage.OpenRange(ctx, request)
}

func (s *memoryDownloadSession) Close() error {
	s.storage.mu.Lock()
	defer s.storage.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.storage.sessionCloses++
	}
	return nil
}

func (s *memoryStorage) Upload(_ context.Context, request telegramstore.UploadRequest) (telegramstore.StoredPart, error) {
	payload, err := io.ReadAll(request.Reader)
	if err != nil {
		return telegramstore.StoredPart{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	copyPayload := append([]byte(nil), payload...)
	s.uploads = append(s.uploads, copyPayload)
	if s.messages == nil {
		s.messages = make(map[int64][]byte)
	}
	s.messages[s.nextID] = copyPayload
	return telegramstore.StoredPart{ChannelID: request.ChannelID, MessageID: s.nextID, Size: int64(len(payload))}, nil
}

func (s *memoryStorage) OpenRange(_ context.Context, request telegramstore.RangeRequest) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, ok := s.messages[request.MessageID]
	if !ok {
		return nil, telegramstore.ErrMessageNotFound
	}
	if request.Offset < 0 || request.Offset > int64(len(payload)) || request.Length < -1 {
		return nil, telegramstore.ErrInvalidRequest
	}
	end := int64(len(payload))
	if request.Length >= 0 && request.Offset+request.Length < end {
		end = request.Offset + request.Length
	}
	s.rangeRequests = append(s.rangeRequests, request)
	return io.NopCloser(bytes.NewReader(append([]byte(nil), payload[request.Offset:end]...))), nil
}

func (s *memoryStorage) DeleteMessages(_ context.Context, _ int64, _ int64, ids []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted = append(s.deleted, ids...)
	for _, id := range ids {
		delete(s.messages, id)
	}
	return nil
}

func (s *memoryStorage) CopyPart(context.Context, int64, int64, int64, int64) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not implemented")
}
func (s *memoryStorage) CreateChannel(context.Context, int64, string) (telegramstore.Channel, error) {
	return telegramstore.Channel{}, errors.New("not implemented")
}

func (s *memoryStorage) DeleteChannel(context.Context, int64, int64) error { return nil }

func (s *memoryStorage) payload(index int) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.uploads[index]...)
}

func (s *memoryStorage) uploadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.uploads)
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("retry body was read") }
