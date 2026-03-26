package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/gotd/td/tg"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/crypt"
	"github.com/tgdrive/teldrive/internal/hash"
)

func TestUploadFlow_HashingAndFileHashGeneration(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	token := loginAndGetToken(t, s, 7101, "user7101")
	client := s.newClientWithToken(token)

	if err := client.UsersUpdateChannel(ctx, &api.ChannelUpdate{ChannelId: api.NewOptInt64(910050), ChannelName: api.NewOptString("upload-default")}); err != nil {
		t.Fatalf("UsersUpdateChannel failed: %v", err)
	}

	plaintext := []byte("hash me please")
	s.tgMock.uploadPartFn = func(_ context.Context, _ *tg.Client, channelID int64, partName string, fileStream io.Reader, fileSize int64, _ int) (int, int64, error) {
		if channelID != 910050 {
			return 0, 0, fmt.Errorf("unexpected channel id: %d", channelID)
		}
		if partName != "hash.part1" {
			return 0, 0, fmt.Errorf("unexpected part name: %s", partName)
		}
		payload, err := io.ReadAll(fileStream)
		if err != nil {
			return 0, 0, err
		}
		if int64(len(payload)) != fileSize {
			return 0, 0, fmt.Errorf("size mismatch got=%d want=%d", len(payload), fileSize)
		}
		if !bytes.Equal(payload, plaintext) {
			return 0, 0, fmt.Errorf("payload mismatch")
		}
		return 12001, fileSize, nil
	}

	part, status, raw := uploadPartRaw(t, s, token, "up-hash-1", "hash.part1", "hash.txt", 1, 910050, false, true, plaintext)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", status, string(raw))
	}
	if part.PartId != 12001 || part.Encrypted {
		t.Fatalf("unexpected upload part: %+v", part)
	}

	uploadRows, err := s.repos.Uploads.GetByUploadID(ctx, "up-hash-1")
	if err != nil {
		t.Fatalf("GetByUploadID failed: %v", err)
	}
	if len(uploadRows) != 1 {
		t.Fatalf("expected one upload row, got %d", len(uploadRows))
	}
	if uploadRows[0].BlockHashes == nil || len(*uploadRows[0].BlockHashes) == 0 {
		t.Fatalf("expected block hashes to be present")
	}

	b := hash.NewBlockHasher()
	_, _ = b.Write(plaintext)
	expected := hash.SumToHex(hash.ComputeTreeHash(b.Sum()))

	created, err := client.FilesCreate(ctx, &api.File{
		Name:      "hashed.txt",
		Type:      api.FileTypeFile,
		Path:      api.NewOptString("/"),
		MimeType:  api.NewOptString("text/plain"),
		ChannelId: api.NewOptInt64(910050),
		Size:      api.NewOptInt64(int64(len(plaintext))),
		UploadId:  api.NewOptString("up-hash-1"),
	})
	if err != nil {
		t.Fatalf("FilesCreate from upload failed: %v", err)
	}
	if !created.Hash.IsSet() || created.Hash.Value == "" {
		created, err = client.FilesGetById(ctx, api.FilesGetByIdParams{ID: created.ID.Value})
		if err != nil {
			t.Fatalf("FilesGetById after create failed: %v", err)
		}
	}
	if !created.Hash.IsSet() || created.Hash.Value == "" {
		t.Fatalf("expected file hash to be set")
	}
	if created.Hash.Value != expected {
		t.Fatalf("hash mismatch got=%s want=%s", created.Hash.Value, expected)
	}

	uploadRows, err = s.repos.Uploads.GetByUploadID(ctx, "up-hash-1")
	if err != nil {
		t.Fatalf("GetByUploadID after create failed: %v", err)
	}
	if len(uploadRows) != 0 {
		t.Fatalf("expected uploads to be cleaned up, got %d rows", len(uploadRows))
	}

	stats, err := client.UploadsStats(ctx, api.UploadsStatsParams{Days: 2})
	if err != nil {
		t.Fatalf("UploadsStats failed: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 days of stats, got %d", len(stats))
	}
}

func TestUploadFlow_EncryptedUploadAndSalt(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	s.cfg.TG.Uploads.EncryptionKey = "integration-test-encryption-key"
	token := loginAndGetToken(t, s, 7102, "user7102")
	client := s.newClientWithToken(token)

	if err := client.UsersUpdateChannel(ctx, &api.ChannelUpdate{ChannelId: api.NewOptInt64(910060), ChannelName: api.NewOptString("enc-default")}); err != nil {
		t.Fatalf("UsersUpdateChannel failed: %v", err)
	}

	plaintext := []byte("secret-content")
	expectedEncryptedSize := crypt.EncryptedSize(int64(len(plaintext)))

	s.tgMock.uploadPartFn = func(_ context.Context, _ *tg.Client, _ int64, _ string, fileStream io.Reader, fileSize int64, _ int) (int, int64, error) {
		payload, err := io.ReadAll(fileStream)
		if err != nil {
			return 0, 0, err
		}
		if int64(len(payload)) != fileSize {
			return 0, 0, fmt.Errorf("encrypted payload size mismatch got=%d want=%d", len(payload), fileSize)
		}
		return 13001, fileSize, nil
	}

	part, status, raw := uploadPartRaw(t, s, token, "up-enc-1", "enc.part1", "secret.txt", 1, 910060, true, false, plaintext)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", status, string(raw))
	}
	if !part.Encrypted {
		t.Fatalf("expected encrypted part")
	}
	if !part.Salt.IsSet() || part.Salt.Value == "" {
		t.Fatalf("expected salt in response")
	}
	if part.Size != expectedEncryptedSize {
		t.Fatalf("expected encrypted size %d, got %d", expectedEncryptedSize, part.Size)
	}

	uploadRows, err := s.repos.Uploads.GetByUploadID(ctx, "up-enc-1")
	if err != nil {
		t.Fatalf("GetByUploadID failed: %v", err)
	}
	if len(uploadRows) != 1 {
		t.Fatalf("expected one upload row, got %d", len(uploadRows))
	}
	if !uploadRows[0].Encrypted {
		t.Fatalf("expected encrypted flag in DB")
	}
	if uploadRows[0].Salt == nil || *uploadRows[0].Salt == "" {
		t.Fatalf("expected salt in DB")
	}
	if uploadRows[0].Size != expectedEncryptedSize {
		t.Fatalf("expected DB size %d, got %d", expectedEncryptedSize, uploadRows[0].Size)
	}

	parts, err := client.UploadsPartsById(ctx, api.UploadsPartsByIdParams{ID: "up-enc-1"})
	if err != nil {
		t.Fatalf("UploadsPartsById failed: %v", err)
	}
	if len(parts) != 0 {
		t.Fatalf("expected no parts because retention filter excludes new parts, got %d", len(parts))
	}

	if err := client.UploadsDelete(ctx, api.UploadsDeleteParams{ID: "up-enc-1"}); err != nil {
		t.Fatalf("UploadsDelete failed: %v", err)
	}
	uploadRows, err = s.repos.Uploads.GetByUploadID(ctx, "up-enc-1")
	if err != nil {
		t.Fatalf("GetByUploadID after delete failed: %v", err)
	}
	if len(uploadRows) != 0 {
		t.Fatalf("expected uploads deleted, got %d", len(uploadRows))
	}
}

func TestUploadFlow_EncryptedWithoutKey(t *testing.T) {
	s := newSuite(t)

	token := loginAndGetToken(t, s, 7103, "user7103")
	part, status, raw := uploadPartRaw(t, s, token, "up-enc-no-key", "p1", "f1.txt", 1, 910070, true, true, []byte("abc"))
	_ = part
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", status, string(raw))
	}
}

func uploadPartRaw(t *testing.T, s *suite, token string, uploadID, partName, fileName string, partNo int, channelID int64, encrypted, hashing bool, body []byte) (api.UploadPart, int, []byte) {
	t.Helper()

	q := url.Values{}
	q.Set("partName", partName)
	q.Set("fileName", fileName)
	q.Set("partNo", strconv.Itoa(partNo))
	q.Set("channelId", strconv.FormatInt(channelID, 10))
	q.Set("encrypted", strconv.FormatBool(encrypted))
	q.Set("hashing", strconv.FormatBool(hashing))

	u := fmt.Sprintf("%s/uploads/%s?%s", s.server.URL, url.PathEscape(uploadID), q.Encode())
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create upload request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Cookie", "access_token="+token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(body))

	resp, err := s.httpCli.Do(req)
	if err != nil {
		t.Fatalf("execute upload request: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read upload response: %v", err)
	}

	var out api.UploadPart
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode upload response: %v body=%s", err, string(raw))
		}
	}

	return out, resp.StatusCode, raw
}
