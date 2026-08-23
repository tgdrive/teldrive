package localtelegram_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/localtelegram"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

func TestServerPersistsGotdStorageLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()

	server, err := localtelegram.Open(root)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	runner, err := localtelegram.NewRunner(server)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	storage := telegramstore.NewGotdStorage(runner, nil)

	channel, err := storage.CreateChannel(ctx, 1001, "rclone")
	if err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}
	if channel.ID <= 0 || channel.Name != "rclone" {
		t.Fatalf("channel = %#v", channel)
	}
	account, err := telegramstore.NewGotdAccount(runner)
	if err != nil {
		t.Fatalf("NewGotdAccount() error = %v", err)
	}
	discovered, err := account.DiscoverChannels(ctx, 1001)
	if err != nil {
		t.Fatalf("DiscoverChannels() error = %v", err)
	}
	if len(discovered) != 1 || discovered[0].ID != channel.ID || discovered[0].Name != channel.Name {
		t.Fatalf("discovered channels = %#v", discovered)
	}
	if photo, found, err := account.ProfilePhoto(ctx, 1001); err != nil || found || len(photo.Content) != 0 {
		t.Fatalf("ProfilePhoto() = %#v, %t, %v", photo, found, err)
	}

	payload := bytes.Repeat([]byte("0123456789abcdef"), 1024)
	stored, err := storage.Upload(ctx, telegramstore.UploadRequest{
		UserID: 1001, ChannelID: channel.ID, Name: "payload.bin",
		Reader: bytes.NewReader(payload), Size: int64(len(payload)), Threads: 4,
	})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	assertRange(t, storage, stored, 101, 4096, payload[101:101+4096])

	copied, err := storage.CopyPart(ctx, 1001, stored.ChannelID, stored.MessageID, channel.ID)
	if err != nil {
		t.Fatalf("CopyPart() error = %v", err)
	}
	if err := storage.DeleteMessages(ctx, 1001, channel.ID, []int64{stored.MessageID}); err != nil {
		t.Fatalf("DeleteMessages(source) error = %v", err)
	}

	reopened, err := localtelegram.Open(root)
	if err != nil {
		t.Fatalf("reopen emulator: %v", err)
	}
	reopenedRunner, err := localtelegram.NewRunner(reopened)
	if err != nil {
		t.Fatal(err)
	}
	reopenedStorage := telegramstore.NewGotdStorage(reopenedRunner, nil)
	assertRange(t, reopenedStorage, copied, 0, int64(len(payload)), payload)

	if err := reopenedStorage.DeleteMessages(ctx, 1001, channel.ID, []int64{copied.MessageID}); err != nil {
		t.Fatalf("DeleteMessages(copy) error = %v", err)
	}
	documentEntries, err := os.ReadDir(filepath.Join(root, "documents"))
	if err != nil {
		t.Fatalf("read documents: %v", err)
	}
	if len(documentEntries) != 0 {
		t.Fatalf("unreferenced documents remain: %v", documentEntries)
	}
	if err := reopenedStorage.DeleteChannel(ctx, 1001, channel.ID); err != nil {
		t.Fatalf("DeleteChannel() error = %v", err)
	}
}

func TestServerSupportsBigFileUpload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, err := localtelegram.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := localtelegram.NewRunner(server)
	if err != nil {
		t.Fatal(err)
	}
	storage := telegramstore.NewGotdStorage(runner, nil)
	channel, err := storage.CreateChannel(ctx, 1001, "big")
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("b"), 11*1024*1024)
	stored, err := storage.Upload(ctx, telegramstore.UploadRequest{
		UserID: 1001, ChannelID: channel.ID, Name: "big.bin",
		Reader: bytes.NewReader(payload), Size: int64(len(payload)), Threads: 4,
	})
	if err != nil {
		t.Fatalf("Upload(big) error = %v", err)
	}
	assertRange(t, storage, stored, int64(len(payload)-2048), 2048, payload[len(payload)-2048:])
}

func assertRange(t *testing.T, storage telegramstore.Storage, part telegramstore.StoredPart, offset, length int64, want []byte) {
	t.Helper()
	reader, err := storage.OpenRange(context.Background(), telegramstore.RangeRequest{
		UserID: 1001, ChannelID: part.ChannelID, MessageID: part.MessageID,
		Offset: offset, Length: length,
	})
	if err != nil {
		t.Fatalf("OpenRange() error = %v", err)
	}
	got, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		t.Fatalf("ReadAll() error = %v", readErr)
	}
	if closeErr != nil {
		t.Fatalf("Close() error = %v", closeErr)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("range mismatch: got %d bytes, want %d", len(got), len(want))
	}
}
