//go:build integration

package transfer_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

func TestDownloaderReadsAcrossTelegramParts(t *testing.T) {
	db := testpostgres.New(t)
	seedTransferOwner(t, db.Pool, 1001, 9001)
	uploadCatalog := uploads.NewService(db.Pool)
	fileCatalog := catalog.NewService(db.Pool)
	storage := &memoryStorage{}
	pipeline := transfer.NewPipeline(uploadCatalog, &fixedResolver{channelID: 9001}, storage, nil, transfer.Config{})

	session, err := uploadCatalog.Create(context.Background(), uploads.CreateInput{
		UserID: 1001, Name: "parts.bin", ExpectedSize: 10, PartSize: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	uploadID, _ := dbtypes.GoogleUUID(session.ID)
	for partNo, body := range [][]byte{[]byte("abcd"), []byte("efgh"), []byte("ij")} {
		if _, err := pipeline.UploadPart(context.Background(), transfer.UploadPartRequest{
			UserID: 1001, UploadID: uploadID, PartNo: int32(partNo + 1), PlainSize: int64(len(body)), Body: bytes.NewReader(body),
		}); err != nil {
			t.Fatalf("upload part %d: %v", partNo+1, err)
		}
	}
	file, err := uploadCatalog.Complete(context.Background(), 1001, uploadID)
	if err != nil {
		t.Fatal(err)
	}
	fileID, _ := dbtypes.GoogleUUID(file.ID)

	downloader := transfer.NewDownloader(fileCatalog, storage, nil)
	download, err := downloader.Open(context.Background(), transfer.DownloadRequest{
		UserID: 1001, FileID: fileID, Offset: 3, Length: 5,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer download.Reader.Close()
	got, err := io.ReadAll(download.Reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "defgh" {
		t.Fatalf("range = %q, want defgh", got)
	}
	if download.Offset != 3 || download.Length != 5 || download.TotalSize != 10 {
		t.Fatalf("download metadata = %#v", download)
	}

	storage.mu.Lock()
	requests := append([]telegramstore.RangeRequest(nil), storage.rangeRequests...)
	storage.mu.Unlock()
	if len(requests) != 2 || requests[0].Offset != 3 || requests[0].Length != 1 || requests[1].Offset != 0 || requests[1].Length != 4 {
		t.Fatalf("Telegram range requests = %#v", requests)
	}
}

func TestDownloaderDecryptsRangeAcrossCipherBlocks(t *testing.T) {
	db := testpostgres.New(t)
	seedTransferOwner(t, db.Pool, 1001, 9001)
	uploadCatalog := uploads.NewService(db.Pool)
	fileCatalog := catalog.NewService(db.Pool)
	storage := &memoryStorage{}
	keyVersion := int32(3)
	keys := transfer.StaticKeyProvider{3: "download-compatible-key"}
	pipeline := transfer.NewPipeline(uploadCatalog, &fixedResolver{channelID: 9001}, storage, keys, transfer.Config{
		Random: bytes.NewReader(bytes.Repeat([]byte{4}, 32)),
	})
	body := bytes.Repeat([]byte("0123456789abcdef"), 9000)
	session, err := uploadCatalog.Create(context.Background(), uploads.CreateInput{
		UserID: 1001, Name: "encrypted-range.bin", ExpectedSize: int64(len(body)), PartSize: int64(len(body)),
		Encryption: true, EncryptionKeyVersion: &keyVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	uploadID, _ := dbtypes.GoogleUUID(session.ID)
	if _, err := pipeline.UploadPart(context.Background(), transfer.UploadPartRequest{
		UserID: 1001, UploadID: uploadID, PartNo: 1, PlainSize: int64(len(body)), Body: bytes.NewReader(body),
	}); err != nil {
		t.Fatalf("UploadPart() error = %v", err)
	}
	file, err := uploadCatalog.Complete(context.Background(), 1001, uploadID)
	if err != nil {
		t.Fatal(err)
	}
	fileID, _ := dbtypes.GoogleUUID(file.ID)

	offset := int64(65536 - 17)
	length := int64(100)
	downloader := transfer.NewDownloader(fileCatalog, storage, keys)
	download, err := downloader.Open(context.Background(), transfer.DownloadRequest{
		UserID: 1001, FileID: fileID, Offset: offset, Length: length,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer download.Reader.Close()
	got, err := io.ReadAll(download.Reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !bytes.Equal(got, body[offset:offset+length]) {
		t.Fatalf("decrypted range mismatch: got %d bytes", len(got))
	}
	if err := download.Reader.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	storage.mu.Lock()
	requests := append([]telegramstore.RangeRequest(nil), storage.rangeRequests...)
	ciphertextSize := len(storage.uploads[0])
	sessionOpens, sessionCloses := storage.sessionOpens, storage.sessionCloses
	storage.mu.Unlock()
	if sessionOpens != 1 || sessionCloses != 1 {
		t.Fatalf("encrypted download sessions = opened %d, closed %d, want 1/1", sessionOpens, sessionCloses)
	}
	if len(requests) == 0 {
		t.Fatal("expected Telegram range request")
	}
	for _, request := range requests {
		if request.Length >= int64(ciphertextSize) {
			t.Fatalf("encrypted range fetched whole object: %#v", request)
		}
	}
}
