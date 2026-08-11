package transfer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	"github.com/tgdrive/teldrive/v2/internal/treehash"
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
