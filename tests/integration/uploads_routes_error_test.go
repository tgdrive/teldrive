package integration_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/gotd/td/tg"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
	"github.com/tgdrive/teldrive/pkg/services"
)

func TestUploadsRoutes_TelegramErrorMatrix(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	token := loginAndGetToken(t, s, 7301, "user7301")

	if err := s.repos.Bots.Create(ctx, &jetmodel.Bots{UserID: 7301, Token: "12345:bot", BotID: 12345}); err != nil {
		t.Fatalf("seed bot token: %v", err)
	}

	cases := []struct {
		name string
		set  func()
	}{
		{
			name: "select bot token failure",
			set: func() {
				s.tgMock.selectBotTokenFn = func(context.Context, string, int64, []string) (string, int, error) {
					return "", 0, errors.New("select bot failed")
				}
			},
		},
		{
			name: "bot client failure",
			set: func() {
				s.tgMock.botClientFn = func(context.Context, string, int) (services.TelegramClient, error) {
					return nil, errors.New("bot client failed")
				}
			},
		},
		{
			name: "upload pool failure",
			set: func() {
				s.tgMock.newUploadPoolFn = func(context.Context, services.TelegramClient, int64, int) (services.UploadPool, error) {
					return nil, errors.New("pool failed")
				}
			},
		},
		{
			name: "run with auth failure",
			set: func() {
				s.tgMock.runWithAuthFn = func(context.Context, services.TelegramClient, string, func(context.Context) error) error {
					return errors.New("run with auth failed")
				}
			},
		},
		{
			name: "upload part failure",
			set: func() {
				s.tgMock.uploadPartFn = func(context.Context, *tg.Client, int64, string, io.Reader, int64, int) (int, int64, error) {
					return 0, 0, errors.New("upload part failed")
				}
			},
		},
		{
			name: "upload part zero message id",
			set: func() {
				s.tgMock.uploadPartFn = func(_ context.Context, _ *tg.Client, _ int64, _ string, _ io.Reader, fileSize int64, _ int) (int, int64, error) {
					return 0, fileSize, nil
				}
			},
		},
		{
			name: "upload part size mismatch",
			set: func() {
				s.tgMock.uploadPartFn = func(_ context.Context, _ *tg.Client, _ int64, _ string, _ io.Reader, fileSize int64, _ int) (int, int64, error) {
					return 9001, fileSize + 1, nil
				}
			},
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s.tgMock.authClientFn = nil
			s.tgMock.botClientFn = nil
			s.tgMock.runWithAuthFn = nil
			s.tgMock.newUploadPoolFn = nil
			s.tgMock.selectBotTokenFn = nil
			s.tgMock.uploadPartFn = nil
			s.cfg.TG.Uploads.EncryptionKey = "integration-test-encryption-key"
			tc.set()

			_, status, raw := uploadPartRaw(t, s, token, "up-err-"+strconv.Itoa(i+1), "err.part", "err.txt", 1, 910090, false, true, []byte("hello"))
			if status != http.StatusInternalServerError {
				t.Fatalf("expected 500, got %d body=%s", status, string(raw))
			}
		})
	}
}

func TestUploadsRoutes_HashingDisabledStoresNoBlockHashes(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	token := loginAndGetToken(t, s, 7302, "user7302")

	s.tgMock.uploadPartFn = func(_ context.Context, _ *tg.Client, _ int64, _ string, _ io.Reader, fileSize int64, _ int) (int, int64, error) {
		return 14001, fileSize, nil
	}

	_, status, raw := uploadPartRaw(t, s, token, "up-nohash-1", "p1", "f1.txt", 1, 910091, false, false, []byte("nohash"))
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", status, string(raw))
	}

	rows, err := s.repos.Uploads.GetByUploadID(ctx, "up-nohash-1")
	if err != nil {
		t.Fatalf("GetByUploadID failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if rows[0].BlockHashes != nil {
		t.Fatalf("expected block hashes to be nil when hashing=false")
	}
}

func TestUploadsRoutes_InvalidContentType(t *testing.T) {
	s := newSuite(t)
	token := loginAndGetToken(t, s, 7303, "user7303")

	q := url.Values{}
	q.Set("partName", "p1")
	q.Set("fileName", "f1.txt")
	q.Set("partNo", "1")
	q.Set("channelId", "910092")

	u := s.server.URL + "/uploads/up-bad-content-type?" + q.Encode()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, u, bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Cookie", "access_token="+token)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 2

	resp, err := s.httpCli.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}
