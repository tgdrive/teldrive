package transfer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	varccache "github.com/tgdrive/varc/cache"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

func TestPlanSegments(t *testing.T) {
	t.Parallel()
	parts := []*sqlcgen.FilePart{
		{PartNo: 1, ChannelID: 1, MessageID: 11, PlainSize: pgtype.Int8{Int64: 4, Valid: true}, StoredSize: pgtype.Int8{Int64: 4, Valid: true}},
		{PartNo: 2, ChannelID: 1, MessageID: 12, PlainSize: pgtype.Int8{Int64: 4, Valid: true}, StoredSize: pgtype.Int8{Int64: 4, Valid: true}},
		{PartNo: 3, ChannelID: 1, MessageID: 13, PlainSize: pgtype.Int8{Int64: 2, Valid: true}, StoredSize: pgtype.Int8{Int64: 2, Valid: true}},
	}

	segments, length, err := planSegments(parts, 10, 3, 5)
	if err != nil {
		t.Fatalf("planSegments() error = %v", err)
	}
	if length != 5 || len(segments) != 2 {
		t.Fatalf("segments = %#v, length=%d", segments, length)
	}
	if segments[0].part.PartNo != 1 || segments[0].offset != 3 || segments[0].length != 1 {
		t.Fatalf("first segment = %#v", segments[0])
	}
	if segments[1].part.PartNo != 2 || segments[1].offset != 0 || segments[1].length != 4 {
		t.Fatalf("second segment = %#v", segments[1])
	}

	segments, length, err = planSegments(parts, 10, 8, -1)
	if err != nil || length != 2 || len(segments) != 1 || segments[0].part.PartNo != 3 {
		t.Fatalf("tail range = %#v, %d, %v", segments, length, err)
	}

	segments, length, err = planSegments(parts, 10, 10, -1)
	if err != nil || length != 0 || len(segments) != 0 {
		t.Fatalf("empty end range = %#v, %d, %v", segments, length, err)
	}
}

func TestPlanSegmentsRejectsInvalidLayouts(t *testing.T) {
	t.Parallel()
	valid := []*sqlcgen.FilePart{{PartNo: 1, ChannelID: 1, MessageID: 1, PlainSize: pgtype.Int8{Int64: 1, Valid: true}, StoredSize: pgtype.Int8{Int64: 1, Valid: true}}}
	if _, _, err := planSegments(valid, 1, 2, -1); !errors.Is(err, ErrRangeNotSatisfiable) {
		t.Fatalf("offset error = %v", err)
	}
	if _, _, err := planSegments(nil, 1, 0, -1); !errors.Is(err, ErrCorruptPartLayout) {
		t.Fatalf("missing parts error = %v", err)
	}
	badOrder := []*sqlcgen.FilePart{{PartNo: 2, ChannelID: 1, MessageID: 1, PlainSize: pgtype.Int8{Int64: 1, Valid: true}, StoredSize: pgtype.Int8{Int64: 1, Valid: true}}}
	if _, _, err := planSegments(badOrder, 1, 0, -1); !errors.Is(err, ErrCorruptPartLayout) {
		t.Fatalf("order error = %v", err)
	}
	badStored := []*sqlcgen.FilePart{{PartNo: 1, ChannelID: 1, MessageID: 1, PlainSize: pgtype.Int8{Int64: 1, Valid: true}, StoredSize: pgtype.Int8{Int64: 0, Valid: true}, Salt: pgtype.Text{}}}
	if _, _, err := planSegments(badStored, 1, 0, -1); !errors.Is(err, ErrCorruptPartLayout) {
		t.Fatalf("stored size error = %v", err)
	}
}

func TestPlanSegmentsEmptyFile(t *testing.T) {
	t.Parallel()
	segments, length, err := planSegments(nil, 0, 0, -1)
	if err != nil || length != 0 || len(segments) != 0 {
		t.Fatalf("empty file = %#v, %d, %v", segments, length, err)
	}
}

func TestDownloadReaderSeekAndConcurrentReadAt(t *testing.T) {
	fileID := uuid.New()
	catalog := &downloadCatalog{
		file: &sqlcgen.File{
			ID:         pgtype.UUID{Bytes: fileID, Valid: true},
			UserID:     7,
			Name:       "file.bin",
			Kind:       sqlcgen.FileKindFile,
			Size:       pgtype.Int8{Int64: 10, Valid: true},
			Encryption: false,
			Status:     sqlcgen.FileStatusActive,
		},
		parts: []*sqlcgen.FilePart{
			{PartNo: 1, ChannelID: 11, MessageID: 101, PlainSize: pgtype.Int8{Int64: 4, Valid: true}, StoredSize: pgtype.Int8{Int64: 4, Valid: true}},
			{PartNo: 2, ChannelID: 11, MessageID: 102, PlainSize: pgtype.Int8{Int64: 4, Valid: true}, StoredSize: pgtype.Int8{Int64: 4, Valid: true}},
			{PartNo: 3, ChannelID: 11, MessageID: 103, PlainSize: pgtype.Int8{Int64: 2, Valid: true}, StoredSize: pgtype.Int8{Int64: 2, Valid: true}},
		},
	}
	storage := &downloadStorage{data: map[int64][]byte{
		101: []byte("abcd"),
		102: []byte("efgh"),
		103: []byte("ij"),
	}}
	download, err := NewDownloader(catalog, storage, nil).Open(context.Background(), DownloadRequest{
		UserID: 7,
		FileID: fileID,
		Offset: 2,
		Length: 6,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer download.Reader.Close()

	if pos, err := download.Reader.Seek(1, io.SeekStart); err != nil || pos != 1 {
		t.Fatalf("Seek() = %d, %v", pos, err)
	}
	buf := make([]byte, 3)
	if n, err := download.Reader.Read(buf); err != nil || n != 3 || string(buf) != "def" {
		t.Fatalf("Read() = %d, %v, %q", n, err, buf)
	}

	var wg sync.WaitGroup
	for _, tc := range []struct {
		off  int64
		want string
	}{
		{off: 0, want: "cde"},
		{off: 2, want: "efg"},
		{off: 4, want: "gh"},
	} {
		tc := tc
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := make([]byte, len(tc.want))
			n, err := download.Reader.ReadAt(got, tc.off)
			if err != nil || n != len(tc.want) || string(got) != tc.want {
				t.Errorf("ReadAt(%d) = %d, %v, %q, want %q", tc.off, n, err, got, tc.want)
			}
		}()
	}
	wg.Wait()
}

func TestDownloaderReusesCachedStreamRanges(t *testing.T) {
	fileID := uuid.New()
	catalog := &downloadCatalog{
		file: &sqlcgen.File{
			ID: pgtype.UUID{Bytes: fileID, Valid: true}, UserID: 7, Name: "file.bin",
			Kind: sqlcgen.FileKindFile, Size: pgtype.Int8{Int64: 10, Valid: true}, Status: sqlcgen.FileStatusActive, Generation: 1,
		},
		parts: []*sqlcgen.FilePart{
			{PartNo: 1, ChannelID: 11, MessageID: 101, PlainSize: pgtype.Int8{Int64: 10, Valid: true}, StoredSize: pgtype.Int8{Int64: 10, Valid: true}},
		},
	}
	storage := &downloadStorage{data: map[int64][]byte{101: []byte("abcdefghij")}}
	options := varccache.DefaultOptions()
	options.CachePollInterval = 0
	options.ChunkSize = 4
	options.ChunkSizeLimit = 4
	options.BufferSize = 4
	streamCache, err := varccache.New(context.Background(), t.TempDir(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = streamCache.Close() })
	downloader := NewDownloader(catalog, storage, nil, streamCache)

	read := func() string {
		download, err := downloader.Open(context.Background(), DownloadRequest{UserID: 7, FileID: fileID, Offset: 2, Length: 6})
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer download.Reader.Close()
		body, err := io.ReadAll(download.Reader)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		return string(body)
	}
	if got := read(); got != "cdefgh" {
		t.Fatalf("first cached read = %q", got)
	}
	firstCalls := storage.rangeCalls.Load()
	if firstCalls == 0 {
		t.Fatal("first cached read did not fetch the origin")
	}
	if opens, closes := storage.sessionOpens.Load(), storage.sessionCloses.Load(); opens != 1 || closes != 1 {
		t.Fatalf("cached origin sessions = opens:%d closes:%d, want one shared session", opens, closes)
	}
	if got := read(); got != "cdefgh" {
		t.Fatalf("second cached read = %q", got)
	}
	if calls := storage.rangeCalls.Load(); calls != firstCalls {
		t.Fatalf("origin range calls after cache hit = %d, want %d", calls, firstCalls)
	}
}

func TestDownloaderResolvesAndBackfillsMissingPartSizes(t *testing.T) {
	fileID := uuid.New()
	catalog := &downloadCatalog{
		file: &sqlcgen.File{
			ID: pgtype.UUID{Bytes: fileID, Valid: true}, UserID: 7, Name: "legacy.bin",
			Kind: sqlcgen.FileKindFile, Size: pgtype.Int8{Int64: 6, Valid: true},
			Encryption: false, Status: sqlcgen.FileStatusActive,
		},
		parts: []*sqlcgen.FilePart{
			{PartNo: 1, ChannelID: 11, MessageID: 101},
			{PartNo: 2, ChannelID: 11, MessageID: 102},
		},
	}
	storage := &downloadStorage{data: map[int64][]byte{101: []byte("abcd"), 102: []byte("ef")}}

	download, err := NewDownloader(catalog, storage, nil).Open(context.Background(), DownloadRequest{
		UserID: 7, FileID: fileID, Length: -1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer download.Reader.Close()
	got, err := io.ReadAll(download.Reader)
	if err != nil || string(got) != "abcdef" {
		t.Fatalf("ReadAll() = %q, %v", got, err)
	}
	if storage.metadataCalls != 2 {
		t.Fatalf("metadata calls = %d, want 2", storage.metadataCalls)
	}
	if len(catalog.backfills) != 2 {
		t.Fatalf("backfills = %d, want 2", len(catalog.backfills))
	}

	if _, err := NewDownloader(catalog, storage, nil).Open(context.Background(), DownloadRequest{
		UserID: 7, FileID: fileID, Length: 0,
	}); err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	if storage.metadataCalls != 2 {
		t.Fatalf("metadata calls after backfill = %d, want 2", storage.metadataCalls)
	}
}

func TestDownloaderReusesOneSessionAcrossReadsAndParts(t *testing.T) {
	fileID := uuid.New()
	first := bytes.Repeat([]byte("a"), 96*1024)
	second := bytes.Repeat([]byte("b"), 96*1024)
	catalog := &downloadCatalog{
		file: &sqlcgen.File{
			ID: pgtype.UUID{Bytes: fileID, Valid: true}, UserID: 7, Name: "reader.epub",
			Kind: sqlcgen.FileKindFile, Size: pgtype.Int8{Int64: int64(len(first) + len(second)), Valid: true},
			Encryption: false, Status: sqlcgen.FileStatusActive,
		},
		parts: []*sqlcgen.FilePart{
			{PartNo: 1, ChannelID: 11, MessageID: 101, PlainSize: pgtype.Int8{Int64: int64(len(first)), Valid: true}, StoredSize: pgtype.Int8{Int64: int64(len(first)), Valid: true}},
			{PartNo: 2, ChannelID: 11, MessageID: 102, PlainSize: pgtype.Int8{Int64: int64(len(second)), Valid: true}, StoredSize: pgtype.Int8{Int64: int64(len(second)), Valid: true}},
		},
	}
	storage := &downloadStorage{data: map[int64][]byte{101: first, 102: second}}
	download, err := NewDownloader(catalog, storage, nil).Open(context.Background(), DownloadRequest{
		UserID: 7, FileID: fileID, Length: -1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	got, err := io.ReadAll(download.Reader)
	if err != nil || len(got) != len(first)+len(second) {
		t.Fatalf("ReadAll() = %d bytes, %v", len(got), err)
	}
	if got := storage.sessionOpens.Load(); got != 1 {
		t.Fatalf("download sessions = %d, want 1", got)
	}
	if got := storage.rangeCalls.Load(); got < 2 {
		t.Fatalf("range calls = %d, want multiple reads", got)
	}
	if err := download.Reader.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := storage.sessionCloses.Load(); got != 1 {
		t.Fatalf("closed sessions = %d, want 1", got)
	}
}

func TestDownloadReaderReusesRangeForSmallSequentialReads(t *testing.T) {
	fileID := uuid.New()
	payload := bytes.Repeat([]byte("epub"), 128*1024)
	catalog := &downloadCatalog{
		file: &sqlcgen.File{
			ID: pgtype.UUID{Bytes: fileID, Valid: true}, UserID: 7, Name: "reader.epub",
			Kind: sqlcgen.FileKindFile, Size: pgtype.Int8{Int64: int64(len(payload)), Valid: true},
			Encryption: false, Status: sqlcgen.FileStatusActive,
		},
		parts: []*sqlcgen.FilePart{{
			PartNo: 1, ChannelID: 11, MessageID: 101,
			PlainSize:  pgtype.Int8{Int64: int64(len(payload)), Valid: true},
			StoredSize: pgtype.Int8{Int64: int64(len(payload)), Valid: true},
		}},
	}
	storage := &downloadStorage{data: map[int64][]byte{101: payload}}
	download, err := NewDownloader(catalog, storage, nil).Open(context.Background(), DownloadRequest{
		UserID: 7, FileID: fileID, Length: -1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer download.Reader.Close()

	got := make([]byte, len(payload))
	for off := 0; off < len(got); off += 32 * 1024 {
		end := min(off+32*1024, len(got))
		n, readErr := io.ReadFull(download.Reader, got[off:end])
		if readErr != nil || n != end-off {
			t.Fatalf("Read(%d) = %d, %v", off, n, readErr)
		}
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("downloaded payload differs")
	}
	if calls := storage.rangeCalls.Load(); calls != 1 {
		t.Fatalf("range calls = %d, want 1", calls)
	}
}

type downloadCatalog struct {
	file      *sqlcgen.File
	parts     []*sqlcgen.FilePart
	backfills []partSizeBackfill
}

type partSizeBackfill struct {
	partNo                int32
	plainSize, storedSize int64
}

func (c *downloadCatalog) Get(context.Context, int64, uuid.UUID) (*sqlcgen.File, error) {
	return c.file, nil
}

func (c *downloadCatalog) Parts(context.Context, int64, uuid.UUID) ([]*sqlcgen.FilePart, error) {
	return c.parts, nil
}

func (c *downloadCatalog) UpdatePartSizes(_ context.Context, _ uuid.UUID, partNo int32, plainSize, storedSize int64) error {
	c.backfills = append(c.backfills, partSizeBackfill{partNo: partNo, plainSize: plainSize, storedSize: storedSize})
	return nil
}

type downloadStorage struct {
	data          map[int64][]byte
	metadataCalls int
	sessionOpens  atomic.Int32
	sessionCloses atomic.Int32
	rangeCalls    atomic.Int32
}

func (s *downloadStorage) OpenDownloadSession(context.Context, int64) (telegramstore.DownloadSession, error) {
	s.sessionOpens.Add(1)
	return &downloadStorageSession{storage: s}, nil
}

type downloadStorageSession struct {
	storage *downloadStorage
	closed  bool
}

func (s *downloadStorageSession) Metadata(ctx context.Context, request telegramstore.MetadataRequest) (telegramstore.StoredPart, error) {
	return s.storage.Metadata(ctx, request)
}

func (s *downloadStorageSession) OpenRange(ctx context.Context, request telegramstore.RangeRequest) (io.ReadCloser, error) {
	s.storage.rangeCalls.Add(1)
	return s.storage.OpenRange(ctx, request)
}

func (s *downloadStorageSession) Close() error {
	if !s.closed {
		s.closed = true
		s.storage.sessionCloses.Add(1)
	}
	return nil
}

func (s *downloadStorage) Upload(context.Context, telegramstore.UploadRequest) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, nil
}

func (s *downloadStorage) Metadata(_ context.Context, request telegramstore.MetadataRequest) (telegramstore.StoredPart, error) {
	s.metadataCalls++
	payload, ok := s.data[request.MessageID]
	if !ok {
		return telegramstore.StoredPart{}, telegramstore.ErrMessageNotFound
	}
	return telegramstore.StoredPart{ChannelID: request.ChannelID, MessageID: request.MessageID, Size: int64(len(payload))}, nil
}

func (s *downloadStorage) OpenRange(_ context.Context, request telegramstore.RangeRequest) (io.ReadCloser, error) {
	payload := s.data[request.MessageID]
	end := int64(len(payload))
	if request.Length >= 0 && request.Offset+request.Length < end {
		end = request.Offset + request.Length
	}
	return io.NopCloser(bytes.NewReader(payload[request.Offset:end])), nil
}

func (s *downloadStorage) DeleteMessages(context.Context, int64, int64, []int64) error { return nil }

func (s *downloadStorage) CopyPart(context.Context, int64, int64, int64, int64) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, nil
}

func (s *downloadStorage) CreateChannel(context.Context, int64, string) (telegramstore.Channel, error) {
	return telegramstore.Channel{}, nil
}

func (s *downloadStorage) DeleteChannel(context.Context, int64, int64) error { return nil }
