//go:build integration

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/google/uuid"

	api "github.com/tgdrive/teldrive/v2/internal/api"
	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/health"
	"github.com/tgdrive/teldrive/v2/internal/shares"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

func TestGeneratedServerUploadCompleteAndRangeDownload(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "INSERT INTO channels (channel_id, user_id, name, selected) VALUES (9001, 1001, 'storage', true)"); err != nil {
		t.Fatal(err)
	}

	catalogService := catalog.NewService(db.Pool, nil)
	uploadService := uploads.NewService(db.Pool)
	storage := &apiMemoryStorage{}
	pipeline := transfer.NewPipeline(uploadService, apiFixedResolver(9001), storage, nil, transfer.Config{})
	downloader := transfer.NewDownloader(catalogService, storage, nil)
	shareService, err := shares.NewService(db.Pool, catalogService)
	if err != nil {
		t.Fatal(err)
	}
	handler := api.NewHandler(catalogService, uploadService, pipeline, downloader, health.NewService("test", db.Pool), 0, nil).
		ConfigureDomains(nil, nil, nil, nil, shareService, nil)
	authenticator := apiAuthenticator{}
	server, err := api.NewServer(handler, api.NewSecurity(authenticator))
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	healthResponse := performRequest(t, server, http.MethodGet, "/health/live", nil, nil)
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("health status = %d, body=%s", healthResponse.Code, healthResponse.Body.String())
	}

	createBody := []byte(`{"name":"api.bin","size":10,"modTime":"2026-07-01T00:00:00Z","preferredPartSize":1048576}`)
	createHeaders := map[string]string{
		"Authorization":   "Bearer test-token",
		"Content-Type":    "application/json",
		"Idempotency-Key": uuid.NewString(),
	}
	created := performRequest(t, server, http.MethodPost, "/v1/uploads", createBody, createHeaders)
	if created.Code != http.StatusCreated {
		t.Fatalf("create upload status = %d, body=%s", created.Code, created.Body.String())
	}
	var session gen.UploadSession
	if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode upload session: %v", err)
	}

	payload := []byte("abcdefghij")
	partPath := "/v1/uploads/" + uuid.UUID(session.ID).String() + "/parts/1"
	partResponse := performRequest(t, server, http.MethodPut, partPath, payload, map[string]string{
		"Authorization": "Bearer test-token",
		"Content-Type":  "application/octet-stream",
	})
	if partResponse.Code != http.StatusCreated {
		t.Fatalf("put part status = %d, body=%s", partResponse.Code, partResponse.Body.String())
	}

	completePath := "/v1/uploads/" + uuid.UUID(session.ID).String() + "/complete"
	completed := performRequest(t, server, http.MethodPost, completePath, nil, map[string]string{
		"Authorization":   "Bearer test-token",
		"Idempotency-Key": uuid.NewString(),
	})
	if completed.Code != http.StatusCreated {
		t.Fatalf("complete status = %d, body=%s", completed.Code, completed.Body.String())
	}
	var file gen.FileEntry
	if err := json.Unmarshal(completed.Body.Bytes(), &file); err != nil {
		t.Fatalf("decode file: %v", err)
	}
	if file.Name != "api.bin" || !file.Hash.IsSet() {
		t.Fatalf("completed file = %#v", file)
	}

	downloadPath := "/v1/files/" + uuid.UUID(file.ID).String() + "/content/api.bin"
	download := performRequest(t, server, http.MethodGet, downloadPath, nil, map[string]string{
		"Authorization": "Bearer test-token",
		"Range":         "bytes=3-6",
	})
	if download.Code != http.StatusPartialContent || download.Body.String() != "defg" {
		t.Fatalf("download = status %d, body %q", download.Code, download.Body.String())
	}
	if got := download.Header().Get("Content-Range"); got != "bytes 3-6/10" {
		t.Fatalf("Content-Range = %q", got)
	}
	fullDownload := performRequest(t, server, http.MethodGet, downloadPath+"?download=1", nil, map[string]string{
		"Authorization": "Bearer test-token",
	})
	if fullDownload.Code != http.StatusOK || fullDownload.Body.String() != "abcdefghij" {
		t.Fatalf("full download = status %d, body %q", fullDownload.Code, fullDownload.Body.String())
	}
	if got := fullDownload.Header().Get("Content-Length"); got != "10" {
		t.Fatalf("full Content-Length = %q, want 10", got)
	}
	if got := fullDownload.Header().Get("Content-Disposition"); got != `attachment; filename=api.bin` {
		t.Fatalf("full Content-Disposition = %q", got)
	}

	// Public file content is opt-in and affects both legacy and filename URLs,
	// including HEAD, byte ranges, and attachment downloads.
	publicServer, err := api.NewServer(handler, api.NewSecurity(authenticator).WithPublicFileContent(true))
	if err != nil {
		t.Fatal(err)
	}
	fileIDPath := "/v1/files/" + uuid.UUID(file.ID).String() + "/content"
	for _, path := range []string{fileIDPath, downloadPath} {
		unauthorized := performRequest(t, server, http.MethodGet, path, nil, nil)
		if unauthorized.Code != http.StatusUnauthorized {
			t.Fatalf("default auth %s: status %d", path, unauthorized.Code)
		}
		stream := performRequest(t, publicServer, http.MethodGet, path, nil, map[string]string{"Range": "bytes=3-6"})
		if stream.Code != http.StatusPartialContent || stream.Body.String() != "defg" {
			t.Fatalf("anonymous range %s: status %d, body %q", path, stream.Code, stream.Body.String())
		}
		head := performRequest(t, publicServer, http.MethodHead, path, nil, nil)
		if head.Code != http.StatusOK || head.Header().Get("Content-Length") != "10" {
			t.Fatalf("anonymous HEAD %s: status %d, length %q", path, head.Code, head.Header().Get("Content-Length"))
		}
		attachment := performRequest(t, publicServer, http.MethodGet, path+"?download=1", nil, nil)
		if attachment.Code != http.StatusOK || attachment.Body.String() != "abcdefghij" {
			t.Fatalf("anonymous download %s: status %d, body %q", path, attachment.Code, attachment.Body.String())
		}
	}
	otherRoute := performRequest(t, publicServer, http.MethodGet, "/v1/files", nil, nil)
	if otherRoute.Code != http.StatusUnauthorized {
		t.Fatalf("unrelated list should require auth: %d", otherRoute.Code)
	}
	missingFile := performRequest(t, publicServer, http.MethodGet, "/v1/files/"+uuid.NewString()+"/content", nil, nil)
	if missingFile.Code != http.StatusNotFound {
		t.Fatalf("anonymous missing file: status %d", missingFile.Code)
	}

	fileID := uuid.UUID(file.ID)
	password := "share-password"
	maxDownloads := int64(1)
	createdShare, err := shareService.Create(ctx, shares.CreateInput{
		OwnerID: 1001, FileID: fileID, Password: &password, MaxDownloads: &maxDownloads,
	})
	if err != nil {
		t.Fatalf("create share: %v", err)
	}
	publicPath := "/v1/public/shares/" + createdShare.Token + "/content/api.bin"
	sharePath := "/v1/public/shares/" + createdShare.Token
	for _, headers := range []map[string]string{nil, {"X-Share-Password": "wrong"}} {
		denied := performRequest(t, server, http.MethodPost, sharePath+"/download-session", nil, headers)
		if denied.Code != http.StatusUnauthorized || denied.Header().Get("Set-Cookie") != "" {
			t.Fatalf("invalid download authorization = %d, %s", denied.Code, denied.Body.String())
		}
	}
	authorized := performRequest(t, server, http.MethodPost, sharePath+"/download-session", nil, map[string]string{"X-Share-Password": password})
	if authorized.Code != http.StatusNoContent || authorized.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("download authorization = %d, %s", authorized.Code, authorized.Body.String())
	}
	cookies := authorized.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "teldrive_share_download" || !cookies[0].HttpOnly || cookies[0].Path != "/api"+sharePath+"/" {
		t.Fatalf("download cookies = %#v", cookies)
	}
	cookieHeader := cookies[0].Name + "=" + cookies[0].Value
	// Cookies authorize only content, not metadata, session renewal, or edits.
	for _, request := range []struct {
		method, path string
		body         []byte
	}{
		{http.MethodGet, sharePath, nil},
		{http.MethodPost, sharePath + "/download-session", nil},
		{http.MethodPatch, sharePath + "/files/" + fileID.String(), []byte(`{"name":"renamed.bin"}`)},
	} {
		denied := performRequest(t, server, request.method, request.path, request.body, map[string]string{"Cookie": cookieHeader, "Content-Type": "application/json", "If-Match": `"1"`})
		if denied.Code != http.StatusUnauthorized {
			t.Fatalf("cookie authorized %s %s: %d, %s", request.method, request.path, denied.Code, denied.Body.String())
		}
	}
	storage.mu.Lock()
	beforeHead := storage.openCount
	storage.mu.Unlock()
	head := performRequest(t, server, http.MethodHead, publicPath, nil, map[string]string{"X-Share-Password": password})
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("public HEAD = %d, %q", head.Code, head.Body.String())
	}
	storage.mu.Lock()
	if storage.openCount != beforeHead {
		t.Fatalf("HEAD opened Telegram: before=%d after=%d", beforeHead, storage.openCount)
	}
	storage.mu.Unlock()

	notModified := performRequest(t, server, http.MethodGet, publicPath, nil, map[string]string{
		"X-Share-Password": password, "If-None-Match": head.Header().Get("ETag"),
	})
	if notModified.Code != http.StatusNotModified {
		t.Fatalf("public 304 = %d, body=%s", notModified.Code, notModified.Body.String())
	}
	for _, path := range []string{
		sharePath + "/content", publicPath,
		sharePath + "/files/" + fileID.String() + "/content",
		sharePath + "/files/" + fileID.String() + "/content/api.bin",
	} {
		denied := performRequest(t, server, http.MethodGet, path, nil, nil)
		if denied.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous content %s: %d", path, denied.Code)
		}
		cookieHead := performRequest(t, server, http.MethodHead, path, nil, map[string]string{"Cookie": cookieHeader})
		if cookieHead.Code != http.StatusOK || cookieHead.Body.Len() != 0 || cookieHead.Header().Get("Content-Length") != "10" {
			t.Fatalf("cookie HEAD %s: %d, %s", path, cookieHead.Code, cookieHead.Body.String())
		}
		cached := performRequest(t, server, http.MethodGet, path, nil, map[string]string{"Cookie": cookieHeader, "If-None-Match": cookieHead.Header().Get("ETag")})
		if cached.Code != http.StatusNotModified {
			t.Fatalf("cookie 304 %s: %d, %s", path, cached.Code, cached.Body.String())
		}
	}
	var downloadCount int64
	if err := db.Pool.QueryRow(ctx, "SELECT download_count FROM file_shares WHERE id=$1", createdShare.Row.ID).Scan(&downloadCount); err != nil {
		t.Fatal(err)
	}
	if downloadCount != 0 {
		t.Fatalf("HEAD/304 consumed downloads: %d", downloadCount)
	}

	publicRange := performRequest(t, server, http.MethodGet, publicPath, nil, map[string]string{
		"Cookie": cookieHeader, "Range": "bytes=1-4",
	})
	if publicRange.Code != http.StatusPartialContent || publicRange.Body.String() != "bcde" {
		t.Fatalf("public range = %d, %q", publicRange.Code, publicRange.Body.String())
	}
	if publicRange.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("cookie-authorized content must not be cached")
	}
	exhausted := performRequest(t, server, http.MethodGet, publicPath, nil, map[string]string{"Cookie": cookieHeader})
	if exhausted.Code != http.StatusGone {
		t.Fatalf("exhausted share = %d, body=%s", exhausted.Code, exhausted.Body.String())
	}

}

func TestGeneratedServerRejectsMissingAuthentication(t *testing.T) {
	db := testpostgres.New(t)
	handler := api.NewHandler(catalog.NewService(db.Pool, nil), uploads.NewService(db.Pool), nil, nil, health.NewService("test", db.Pool), 0, nil)
	authenticator := apiAuthenticator{}
	server, err := api.NewServer(handler, api.NewSecurity(authenticator))
	if err != nil {
		t.Fatal(err)
	}
	response := performRequest(t, server, http.MethodGet, "/v1/files/"+uuid.NewString(), nil, nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
}

func performRequest(t testing.TB, handler http.Handler, method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		request.Header.Set("Content-Length", strconv.Itoa(len(body)))
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

type apiAuthenticator struct{}

func (apiAuthenticator) AuthenticateBearer(_ context.Context, token string) (api.Identity, error) {
	if token != "test-token" {
		return api.Identity{}, errors.New("invalid token")
	}
	return api.Identity{UserID: 1001, Roles: []string{"user"}}, nil
}
func (apiAuthenticator) AuthenticateAPIKey(_ context.Context, key string) (api.Identity, error) {
	if key != "test-key" {
		return api.Identity{}, errors.New("invalid key")
	}
	return api.Identity{UserID: 1001, Roles: []string{"user"}}, nil
}

type apiFixedResolver int64

func (r apiFixedResolver) Resolve(context.Context, int64, int64) (int64, error) { return int64(r), nil }

type apiMemoryStorage struct {
	mu        sync.Mutex
	messages  map[int64][]byte
	nextID    int64
	openCount int
}

func (s *apiMemoryStorage) Upload(_ context.Context, request telegramstore.UploadRequest) (telegramstore.StoredPart, error) {
	payload, err := io.ReadAll(request.Reader)
	if err != nil {
		return telegramstore.StoredPart{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.messages == nil {
		s.messages = make(map[int64][]byte)
	}
	s.nextID++
	s.messages[s.nextID] = append([]byte(nil), payload...)
	return telegramstore.StoredPart{ChannelID: request.ChannelID, MessageID: s.nextID, Size: int64(len(payload))}, nil
}
func (s *apiMemoryStorage) OpenRange(_ context.Context, request telegramstore.RangeRequest) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openCount++
	payload, ok := s.messages[request.MessageID]
	if !ok {
		return nil, telegramstore.ErrMessageNotFound
	}
	end := int64(len(payload))
	if request.Length >= 0 && request.Offset+request.Length < end {
		end = request.Offset + request.Length
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), payload[request.Offset:end]...))), nil
}
func (s *apiMemoryStorage) DeleteMessages(_ context.Context, _ int64, _ int64, ids []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		delete(s.messages, id)
	}
	return nil
}
func (s *apiMemoryStorage) CopyPart(_ context.Context, _ int64, _ int64, sourceMessageID, destinationChannelID int64) (telegramstore.StoredPart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, ok := s.messages[sourceMessageID]
	if !ok {
		return telegramstore.StoredPart{}, telegramstore.ErrMessageNotFound
	}
	s.nextID++
	s.messages[s.nextID] = append([]byte(nil), payload...)
	return telegramstore.StoredPart{ChannelID: destinationChannelID, MessageID: s.nextID, Size: int64(len(payload))}, nil
}
func (*apiMemoryStorage) CreateChannel(context.Context, int64, string) (telegramstore.Channel, error) {
	return telegramstore.Channel{}, errors.New("not implemented")
}
func (*apiMemoryStorage) DeleteChannel(context.Context, int64, int64) error { return nil }
