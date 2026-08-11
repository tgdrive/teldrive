package transfer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	"github.com/tgdrive/teldrive/v2/internal/treehash"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

func TestExactReader(t *testing.T) {
	t.Parallel()

	t.Run("exact", func(t *testing.T) {
		r := newExactReader(context.Background(), bytes.NewBufferString("abcd"), 4)
		got, err := io.ReadAll(r)
		if err != nil || string(got) != "abcd" {
			t.Fatalf("ReadAll() = %q, %v", got, err)
		}
		if err := r.Verify(); err != nil {
			t.Fatalf("Verify() error = %v", err)
		}
	})

	t.Run("short", func(t *testing.T) {
		r := newExactReader(context.Background(), bytes.NewBufferString("abc"), 4)
		_, err := io.ReadAll(r)
		if !errors.Is(err, ErrBodyTooShort) {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if err := r.Verify(); !errors.Is(err, ErrBodyTooShort) {
			t.Fatalf("Verify() error = %v", err)
		}
	})

	t.Run("long", func(t *testing.T) {
		r := newExactReader(context.Background(), bytes.NewBufferString("abcde"), 4)
		got, err := io.ReadAll(r)
		if err != nil || string(got) != "abcd" {
			t.Fatalf("ReadAll() = %q, %v", got, err)
		}
		if err := r.Verify(); !errors.Is(err, ErrBodyTooLong) {
			t.Fatalf("Verify() error = %v", err)
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r := newExactReader(ctx, bytes.NewBufferString("abcd"), 4)
		var b [1]byte
		if _, err := r.Read(b[:]); !errors.Is(err, context.Canceled) {
			t.Fatalf("Read() error = %v", err)
		}
	})
}

func TestGenerateSaltDeterministic(t *testing.T) {
	t.Parallel()
	seed := bytes.Repeat([]byte{1}, 32)
	first, err := generateSalt(bytes.NewReader(seed))
	if err != nil {
		t.Fatal(err)
	}
	second, err := generateSalt(bytes.NewReader(seed))
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first == "" {
		t.Fatalf("salt values = %q and %q", first, second)
	}
	if _, err := generateSalt(nil); err == nil {
		t.Fatal("expected nil random source error")
	}
}

func TestPartName(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	pipeline := &Pipeline{}
	first := pipeline.partName(id, 1)
	second := pipeline.partName(id, 1)
	if first != second || len(first) != 64 {
		t.Fatalf("deterministic names = %q, %q", first, second)
	}
}

func TestStaticKeyProvider(t *testing.T) {
	t.Parallel()
	provider := StaticKeyProvider{1: "secret"}
	if key, err := provider.Key(context.Background(), 1, 1); err != nil || key != "secret" {
		t.Fatalf("Key() = %q, %v", key, err)
	}
	if _, err := provider.Key(context.Background(), 1, 2); !errors.Is(err, ErrEncryptionKey) {
		t.Fatalf("missing key error = %v", err)
	}
}

func TestChecksumMatches(t *testing.T) {
	t.Parallel()
	value := "abc"
	if !checksumMatches("", false, nil) {
		t.Fatal("nil assertion should match")
	}
	if !checksumMatches(value, true, &value) {
		t.Fatal("equal checksum should match")
	}
	if checksumMatches(value, false, &value) {
		t.Fatal("invalid stored checksum should not match")
	}
	other := "def"
	if checksumMatches(value, true, &other) {
		t.Fatal("different checksum should not match")
	}
}

func TestDeleteUploadedCompensation(t *testing.T) {
	t.Parallel()
	storage := &deleteStorage{}
	pipeline := &Pipeline{storage: storage}
	if err := pipeline.deleteUploaded(context.Background(), 1, telegramstore.StoredPart{}); err != nil {
		t.Fatalf("empty part deletion error = %v", err)
	}
	part := telegramstore.StoredPart{ChannelID: 9, MessageID: 7}
	if err := pipeline.deleteUploaded(context.Background(), 1, part); err != nil {
		t.Fatalf("deleteUploaded() error = %v", err)
	}
	if len(storage.deleted) != 1 || storage.deleted[0] != 7 {
		t.Fatalf("deleted messages = %#v", storage.deleted)
	}
	storage.err = errors.New("delete failed")
	if err := pipeline.deleteUploaded(context.Background(), 1, part); err == nil {
		t.Fatal("expected compensation error")
	}
}

func TestFailPartDetachesCleanupFromCanceledRequest(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	catalog := &cleanupCatalog{}
	pipeline := &Pipeline{catalog: catalog}
	cause := context.Canceled

	err := pipeline.failPart(ctx, UploadPartRequest{UploadID: uuid.New(), PartNo: 1}, uuid.New(), "request_canceled", cause)

	if !errors.Is(err, cause) {
		t.Fatalf("failPart() error = %v, want %v", err, cause)
	}
	if catalog.cleanupErr != nil {
		t.Fatalf("FailPart() context error = %v", catalog.cleanupErr)
	}
	if !catalog.hadDeadline {
		t.Fatal("FailPart() cleanup context has no deadline")
	}
}

func TestUploadPartRenewsLeaseDuringTransfer(t *testing.T) {
	t.Parallel()
	catalog := newLeaseCatalog()
	storage := &leaseStorage{waitForRenewal: catalog.renewed}
	pipeline := NewPipeline(catalog, fixedChannelResolver(9), storage, nil, Config{LeaseRenewInterval: time.Millisecond})

	result, err := pipeline.UploadPart(context.Background(), UploadPartRequest{
		UserID: 1, UploadID: catalog.uploadID, PartNo: 1, PlainSize: 4, Body: bytes.NewBufferString("data"),
	})
	if err != nil {
		t.Fatalf("UploadPart() error = %v", err)
	}
	if result.Part.State != sqlcgen.UploadPartStateStored || catalog.renewCount() == 0 {
		t.Fatalf("result = %#v, renewals = %d", result, catalog.renewCount())
	}
}

func TestUploadPartFinalizesPublishedMessageAfterRequestCancellation(t *testing.T) {
	t.Parallel()
	catalog := newLeaseCatalog()
	ctx, cancel := context.WithCancel(context.Background())
	storage := &leaseStorage{cancelAfterUpload: cancel}
	pipeline := NewPipeline(catalog, fixedChannelResolver(9), storage, nil, Config{})

	result, err := pipeline.UploadPart(ctx, UploadPartRequest{
		UserID: 1, UploadID: catalog.uploadID, PartNo: 1, PlainSize: 4, Body: bytes.NewBufferString("data"),
	})
	if err != nil {
		t.Fatalf("UploadPart() error = %v", err)
	}
	if result.Part.State != sqlcgen.UploadPartStateStored || catalog.storeContextErr != nil {
		t.Fatalf("result = %#v, store context error = %v", result, catalog.storeContextErr)
	}
}

type fixedChannelResolver int64

func (r fixedChannelResolver) Resolve(context.Context, int64, int64) (int64, error) {
	return int64(r), nil
}

type leaseCatalog struct {
	mu              sync.Mutex
	uploadID        uuid.UUID
	leaseToken      uuid.UUID
	renewed         chan struct{}
	renewOnce       sync.Once
	renewals        int
	storeContextErr error
}

func newLeaseCatalog() *leaseCatalog {
	return &leaseCatalog{uploadID: uuid.New(), leaseToken: uuid.New(), renewed: make(chan struct{})}
}

func (c *leaseCatalog) Get(context.Context, int64, uuid.UUID) (*sqlcgen.UploadSession, error) {
	return &sqlcgen.UploadSession{}, nil
}
func (c *leaseCatalog) GetPart(context.Context, int64, uuid.UUID, int32) (*sqlcgen.UploadPart, error) {
	return nil, uploads.ErrNotFound
}
func (c *leaseCatalog) ClaimPart(context.Context, uploads.ClaimPartInput) (*uploads.ClaimPartResult, error) {
	return &uploads.ClaimPartResult{Part: &sqlcgen.UploadPart{}, LeaseToken: c.leaseToken}, nil
}
func (c *leaseCatalog) RenewPart(context.Context, uploads.RenewPartInput) error {
	c.mu.Lock()
	c.renewals++
	c.mu.Unlock()
	c.renewOnce.Do(func() { close(c.renewed) })
	return nil
}
func (c *leaseCatalog) StorePart(ctx context.Context, _ uploads.StorePartInput) (*sqlcgen.UploadPart, error) {
	c.storeContextErr = ctx.Err()
	return &sqlcgen.UploadPart{State: sqlcgen.UploadPartStateStored}, nil
}
func (*leaseCatalog) FailPart(context.Context, uploads.FailPartInput) (*sqlcgen.UploadPart, error) {
	return &sqlcgen.UploadPart{}, nil
}
func (c *leaseCatalog) renewCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.renewals
}

type leaseStorage struct {
	waitForRenewal    <-chan struct{}
	cancelAfterUpload context.CancelFunc
}

func (s *leaseStorage) Upload(ctx context.Context, request telegramstore.UploadRequest) (telegramstore.StoredPart, error) {
	if s.waitForRenewal != nil {
		select {
		case <-s.waitForRenewal:
		case <-ctx.Done():
			return telegramstore.StoredPart{}, context.Cause(ctx)
		}
	}
	data, err := io.ReadAll(request.Reader)
	if err != nil {
		return telegramstore.StoredPart{}, err
	}
	if s.cancelAfterUpload != nil {
		s.cancelAfterUpload()
	}
	return telegramstore.StoredPart{ChannelID: request.ChannelID, MessageID: 7, Size: int64(len(data))}, nil
}
func (*leaseStorage) OpenRange(context.Context, telegramstore.RangeRequest) (io.ReadCloser, error) {
	return nil, errors.New("not used")
}
func (*leaseStorage) DeleteMessages(context.Context, int64, int64, []int64) error { return nil }
func (*leaseStorage) CopyPart(context.Context, int64, int64, int64, int64) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not used")
}
func (*leaseStorage) CreateChannel(context.Context, int64, string) (telegramstore.Channel, error) {
	return telegramstore.Channel{}, errors.New("not used")
}
func (*leaseStorage) DeleteChannel(context.Context, int64, int64) error { return nil }

type cleanupCatalog struct {
	cleanupErr  error
	hadDeadline bool
}

func (*cleanupCatalog) Get(context.Context, int64, uuid.UUID) (*sqlcgen.UploadSession, error) {
	return nil, errors.New("not used")
}
func (*cleanupCatalog) GetPart(context.Context, int64, uuid.UUID, int32) (*sqlcgen.UploadPart, error) {
	return nil, errors.New("not used")
}
func (*cleanupCatalog) ClaimPart(context.Context, uploads.ClaimPartInput) (*uploads.ClaimPartResult, error) {
	return nil, errors.New("not used")
}
func (*cleanupCatalog) RenewPart(context.Context, uploads.RenewPartInput) error {
	return errors.New("not used")
}
func (*cleanupCatalog) StorePart(context.Context, uploads.StorePartInput) (*sqlcgen.UploadPart, error) {
	return nil, errors.New("not used")
}
func (c *cleanupCatalog) FailPart(ctx context.Context, _ uploads.FailPartInput) (*sqlcgen.UploadPart, error) {
	c.cleanupErr = ctx.Err()
	_, c.hadDeadline = ctx.Deadline()
	return &sqlcgen.UploadPart{}, nil
}

type deleteStorage struct {
	deleted []int64
	err     error
}

func (*deleteStorage) Upload(context.Context, telegramstore.UploadRequest) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not used")
}
func (*deleteStorage) OpenRange(context.Context, telegramstore.RangeRequest) (io.ReadCloser, error) {
	return nil, errors.New("not used")
}
func (s *deleteStorage) DeleteMessages(_ context.Context, _ int64, _ int64, ids []int64) error {
	if s.err != nil {
		return s.err
	}
	s.deleted = append(s.deleted, ids...)
	return nil
}
func (*deleteStorage) CopyPart(context.Context, int64, int64, int64, int64) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not implemented")
}
func (*deleteStorage) CreateChannel(context.Context, int64, string) (telegramstore.Channel, error) {
	return telegramstore.Channel{}, errors.New("not used")
}
func (*deleteStorage) DeleteChannel(context.Context, int64, int64) error { return nil }

func TestNormalizeOptionalChecksum(t *testing.T) {
	t.Parallel()
	if value, err := normalizeOptionalChecksum(nil); err != nil || value != nil {
		t.Fatalf("nil checksum = %#v, %v", value, err)
	}
	valid := strings.Repeat("AB", treehash.DigestSize)
	normalized, err := normalizeOptionalChecksum(&valid)
	if err != nil || normalized == nil || *normalized != strings.ToLower(valid) {
		t.Fatalf("valid checksum = %#v, %v", normalized, err)
	}
	invalid := "xyz"
	if _, err := normalizeOptionalChecksum(&invalid); !errors.Is(err, ErrInvalidUpload) {
		t.Fatalf("invalid checksum error = %v", err)
	}
}

func TestRandomizedPartName(t *testing.T) {
	t.Parallel()
	pipeline := &Pipeline{config: Config{RandomizePartNames: true}}
	first := pipeline.partName(uuid.Nil, 1)
	second := pipeline.partName(uuid.Nil, 1)
	if first == second || len(first) != 64 || len(second) != 64 {
		t.Fatalf("randomized names = %q, %q", first, second)
	}
}
