package transfer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	varccache "github.com/tgdrive/varc/cache"
	"github.com/tgdrive/varc/source"

	"github.com/tgdrive/teldrive/v2/internal/contentcrypto"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
)

type cachedFileCatalog struct {
	mu    sync.Mutex
	file  sqlcgen.File
	parts []*sqlcgen.FilePart
	err   error
}

func (c *cachedFileCatalog) Get(_ context.Context, userID int64, fileID uuid.UUID) (*sqlcgen.File, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	if userID != c.file.UserID || fileID != uuid.UUID(c.file.ID.Bytes) {
		return nil, ErrInvalidDownload
	}
	file := c.file
	return &file, nil
}

func (c *cachedFileCatalog) Parts(context.Context, int64, uuid.UUID) ([]*sqlcgen.FilePart, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	parts := make([]*sqlcgen.FilePart, len(c.parts))
	for i, part := range c.parts {
		copy := *part
		parts[i] = &copy
	}
	return parts, nil
}

func newTestDownloadCache(t *testing.T, directory string) *varccache.Cache {
	t.Helper()
	options := varccache.DefaultOptions()
	options.ChunkSize, options.ChunkSizeLimit = 4, 4
	options.BufferSize, options.ReadAhead, options.HandleCaching = 0, 0, 0
	options.LowLevelRetries = 1
	cache, err := varccache.New(context.Background(), directory, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Error(err)
		}
	})
	return cache
}

func cachedDownloadFixture() (*cachedFileCatalog, *downloadStorage, DownloadRequest) {
	fileID := uuid.New()
	file := sqlcgen.File{
		ID: pgtype.UUID{Bytes: fileID, Valid: true}, UserID: 7, Name: "file.bin",
		Kind: sqlcgen.FileKindFile, Status: sqlcgen.FileStatusActive,
		Size: pgtype.Int8{Int64: 10, Valid: true}, Generation: 1,
	}
	parts := []*sqlcgen.FilePart{
		{PartNo: 1, ChannelID: 11, MessageID: 101, PlainSize: pgtype.Int8{Int64: 4, Valid: true}, StoredSize: pgtype.Int8{Int64: 4, Valid: true}},
		{PartNo: 2, ChannelID: 11, MessageID: 102, PlainSize: pgtype.Int8{Int64: 6, Valid: true}, StoredSize: pgtype.Int8{Int64: 6, Valid: true}},
	}
	storage := &downloadStorage{data: map[int64][]byte{101: []byte("abcd"), 102: []byte("efghij"), 103: []byte("ABCDEF")}}
	return &cachedFileCatalog{file: file, parts: parts}, storage, DownloadRequest{UserID: 7, FileID: fileID, Length: -1}
}

func readCachedDownload(t *testing.T, downloader *Downloader, request DownloadRequest, want string) {
	t.Helper()
	download, err := downloader.Open(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	got, readErr := io.ReadAll(download.Reader)
	closeErr := download.Reader.Close()
	if readErr != nil || closeErr != nil || string(got) != want {
		t.Fatalf("download = %q, read error = %v, close error = %v; want %q", got, readErr, closeErr, want)
	}
}

func TestDownloadCacheReusesRangesAndPersists(t *testing.T) {
	catalog, storage, request := cachedDownloadFixture()
	directory := t.TempDir()
	cache := newTestDownloadCache(t, directory)
	downloader := NewDownloader(catalog, storage, nil).WithCache(cache)
	readCachedDownload(t, downloader, request, "abcdefghij")
	calls := storage.rangeCalls.Load()
	if calls == 0 {
		t.Fatal("cold read did not fetch origin bytes")
	}
	if storage.sessionOpens.Load() != storage.sessionCloses.Load() {
		t.Fatal("cache fetch leaked a Telegram session")
	}
	request.Offset, request.Length = 3, 5
	readCachedDownload(t, downloader, request, "defgh")
	if storage.rangeCalls.Load() != calls {
		t.Fatal("warm overlapping range fetched origin bytes")
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	downloader.WithCache(newTestDownloadCache(t, directory))
	readCachedDownload(t, downloader, request, "defgh")
	if storage.rangeCalls.Load() != calls {
		t.Fatal("persisted cache fetched origin bytes after reopening")
	}
}

func TestDownloadCacheFetchesSparseRanges(t *testing.T) {
	catalog, storage, request := cachedDownloadFixture()
	cache := newTestDownloadCache(t, t.TempDir())
	downloader := NewDownloader(catalog, storage, nil).WithCache(cache)
	request.Offset, request.Length = 4, 2
	readCachedDownload(t, downloader, request, "ef")
	if cached := cache.Stats().CachedBytes; cached < 2 || cached >= 10 {
		t.Fatalf("cached bytes = %d, want only a sparse portion of the file", cached)
	}
}

func TestDownloadCacheChecksAccessAndGeneration(t *testing.T) {
	catalog, storage, request := cachedDownloadFixture()
	cache := newTestDownloadCache(t, t.TempDir())
	downloader := NewDownloader(catalog, storage, nil).WithCache(cache)
	readCachedDownload(t, downloader, request, "abcdefghij")
	denied := errors.New("access revoked")
	catalog.mu.Lock()
	catalog.err = denied
	catalog.mu.Unlock()
	if _, err := downloader.Open(context.Background(), request); !errors.Is(err, denied) {
		t.Fatalf("warm cache bypassed authorization: %v", err)
	}
	catalog.mu.Lock()
	catalog.err = nil
	catalog.file.Status = sqlcgen.FileStatusTrashed
	catalog.mu.Unlock()
	if _, err := downloader.Open(context.Background(), request); !errors.Is(err, ErrInvalidDownload) {
		t.Fatalf("warm cache served trashed file: %v", err)
	}
	catalog.mu.Lock()
	catalog.file.Status = sqlcgen.FileStatusActive
	catalog.file.Generation++
	catalog.parts[1].MessageID = 103
	catalog.mu.Unlock()
	readCachedDownload(t, downloader, request, "abcdABCDEF")
	if items := cache.Stats().Items; items != 1 {
		t.Fatalf("generation change created %d cache entries, want one per file ID", items)
	}
	request.UserID++
	if _, err := downloader.Open(context.Background(), request); !errors.Is(err, ErrInvalidDownload) {
		t.Fatalf("warm cache served another owner's file: %v", err)
	}
}

func TestDownloadCacheReaderRangesAndConcurrentReadAt(t *testing.T) {
	catalog, storage, request := cachedDownloadFixture()
	downloader := NewDownloader(catalog, storage, nil).WithCache(newTestDownloadCache(t, t.TempDir()))
	request.Offset, request.Length = 2, 6
	download, err := downloader.Open(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer download.Reader.Close()
	if pos, err := download.Reader.Seek(2, io.SeekStart); err != nil || pos != 2 {
		t.Fatalf("Seek() = %d, %v", pos, err)
	}
	got, err := io.ReadAll(download.Reader)
	if err != nil || string(got) != "efgh" {
		t.Fatalf("ReadAll() = %q, %v", got, err)
	}
	var wg sync.WaitGroup
	for _, offset := range []int64{0, 1, 3} {
		wg.Go(func() {
			buf := make([]byte, 3)
			n, err := download.Reader.ReadAt(buf, offset)
			if n != 3 || err != nil || string(buf) != "cdefgh"[offset:offset+3] {
				t.Errorf("ReadAt(%d) = %q, %d, %v", offset, buf, n, err)
			}
		})
	}
	wg.Wait()
	for _, tc := range []struct {
		offset, length int64
		want           string
	}{
		{10, -1, ""}, {8, 100, "ij"}, {0, 0, ""},
	} {
		request.Offset, request.Length = tc.offset, tc.length
		readCachedDownload(t, downloader, request, tc.want)
	}
	request.Offset = 11
	if _, err := downloader.Open(context.Background(), request); !errors.Is(err, ErrRangeNotSatisfiable) {
		t.Fatalf("invalid range error = %v", err)
	}
}

func TestDownloadCacheRejectsChangedOrigin(t *testing.T) {
	catalog, storage, request := cachedDownloadFixture()
	downloader := NewDownloader(catalog, storage, nil).WithCache(newTestDownloadCache(t, t.TempDir()))
	download, err := downloader.Open(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer download.Reader.Close()
	catalog.mu.Lock()
	catalog.file.Generation++
	catalog.mu.Unlock()
	if _, err := io.ReadAll(download.Reader); !errors.Is(err, source.ErrObjectChanged) {
		t.Fatalf("changed origin error = %v", err)
	}
}

func TestDownloadCacheDecryptsAndChecksKeysOnHits(t *testing.T) {
	catalog, storage, request := cachedDownloadFixture()
	keys := StaticKeyProvider{1: "cache-test-key"}
	cipher, err := contentcrypto.NewCipher(keys[1], "test-salt")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := cipher.EncryptData(bytes.NewBufferString("abcdefghij"))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := io.ReadAll(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if err := encrypted.Close(); err != nil {
		t.Fatal(err)
	}
	storage.data[101] = payload
	catalog.file.Encryption = true
	catalog.file.EncryptionKeyVersion = pgtype.Int4{Int32: 1, Valid: true}
	catalog.parts = []*sqlcgen.FilePart{{
		PartNo: 1, ChannelID: 11, MessageID: 101,
		PlainSize: pgtype.Int8{Int64: 10, Valid: true}, StoredSize: pgtype.Int8{Int64: int64(len(payload)), Valid: true},
		Salt: pgtype.Text{String: "test-salt", Valid: true},
	}}
	downloader := NewDownloader(catalog, storage, keys).WithCache(newTestDownloadCache(t, t.TempDir()))
	readCachedDownload(t, downloader, request, "abcdefghij")
	calls := storage.rangeCalls.Load()
	readCachedDownload(t, downloader, request, "abcdefghij")
	if storage.rangeCalls.Load() != calls {
		t.Fatal("warm encrypted file fetched origin bytes")
	}
	downloader.keys = StaticKeyProvider{1: "different-key"}
	download, err := downloader.Open(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.ReadAll(download.Reader)
	if err := download.Reader.Close(); err != nil {
		t.Fatal(err)
	}
	if readErr == nil {
		t.Fatal("changed encryption key reused previously decrypted cache bytes")
	}
	downloader.keys = StaticKeyProvider{}
	if _, err := downloader.Open(context.Background(), request); !errors.Is(err, ErrEncryptionKey) {
		t.Fatalf("warm cache bypassed missing encryption key: %v", err)
	}
}
