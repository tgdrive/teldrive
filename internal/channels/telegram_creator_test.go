package channels

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

func TestTelegramCreator(t *testing.T) {
	t.Parallel()
	storage := &creatorStorage{created: telegramstore.Channel{ID: 99, Name: "storage"}}
	creator := TelegramCreator{Storage: storage}
	created, err := creator.Create(context.Background(), 1, "storage")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID != 99 || created.Name != "storage" {
		t.Fatalf("Create() = %#v", created)
	}
	if err := creator.Delete(context.Background(), 1, 99); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if storage.deleted != 99 {
		t.Fatalf("deleted channel = %d", storage.deleted)
	}
}

func TestTelegramCreatorRejectsMissingStorage(t *testing.T) {
	t.Parallel()
	creator := TelegramCreator{}
	if _, err := creator.Create(context.Background(), 1, "storage"); err == nil {
		t.Fatal("expected Create error")
	}
	if err := creator.Delete(context.Background(), 1, 99); err == nil {
		t.Fatal("expected Delete error")
	}
}

type creatorStorage struct {
	created telegramstore.Channel
	deleted int64
}

func (*creatorStorage) Upload(context.Context, telegramstore.UploadRequest) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not used")
}
func (*creatorStorage) OpenRange(context.Context, telegramstore.RangeRequest) (io.ReadCloser, error) {
	return nil, errors.New("not used")
}
func (*creatorStorage) DeleteMessages(context.Context, int64, int64, []int64) error { return nil }
func (s *creatorStorage) CopyPart(context.Context, int64, int64, int64, int64) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not implemented")
}
func (s *creatorStorage) CreateChannel(context.Context, int64, string) (telegramstore.Channel, error) {
	return s.created, nil
}
func (s *creatorStorage) DeleteChannel(_ context.Context, _ int64, channelID int64) error {
	s.deleted = channelID
	return nil
}
