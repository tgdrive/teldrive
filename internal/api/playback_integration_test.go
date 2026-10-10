//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	api "github.com/tgdrive/teldrive/v2/internal/api"
	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/health"
	"github.com/tgdrive/teldrive/v2/internal/shares"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
	"net/http"
	"testing"
)

func TestPublicPlaybackRangesConsumeOneReservationAndRespectRevocation(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users(user_id) VALUES(1001); INSERT INTO channels(channel_id,user_id,name,selected) VALUES(9001,1001,'storage',true)"); err != nil {
		t.Fatal(err)
	}
	cat := catalog.NewService(db.Pool, nil)
	up := uploads.NewService(db.Pool)
	storage := &apiMemoryStorage{}
	pipeline := transfer.NewPipeline(up, apiFixedResolver(9001), storage, nil, transfer.Config{})
	shareService, err := shares.NewService(db.Pool, cat)
	if err != nil {
		t.Fatal(err)
	}
	handler := api.NewHandler(cat, up, pipeline, transfer.NewDownloader(cat, storage, nil), health.NewService("test", db.Pool), 0, nil).ConfigureDomains(nil, nil, nil, nil, shareService, nil)
	server, err := api.NewServer(handler, api.NewSecurity(apiAuthenticator{}))
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": "Bearer test-token", "Content-Type": "application/json", "Idempotency-Key": uuid.NewString()}
	response := performRequest(t, server, http.MethodPost, "/v1/uploads", []byte(`{"name":"audio.wav","size":10,"modTime":"2026-07-01T00:00:00Z","preferredPartSize":1048576}`), headers)
	if response.Code != 201 {
		t.Fatalf("upload: %d %s", response.Code, response.Body.String())
	}
	var upload gen.UploadSession
	if err := json.Unmarshal(response.Body.Bytes(), &upload); err != nil {
		t.Fatal(err)
	}
	uploadPath := "/v1/uploads/" + uuid.UUID(upload.ID).String()
	response = performRequest(t, server, http.MethodPut, uploadPath+"/parts/1", []byte("abcdefghij"), map[string]string{"Authorization": "Bearer test-token", "Content-Type": "application/octet-stream"})
	if response.Code != 201 {
		t.Fatal(response.Body.String())
	}
	response = performRequest(t, server, http.MethodPost, uploadPath+"/complete", nil, headers)
	var entry gen.FileEntry
	if response.Code != 201 {
		t.Fatal(response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	fileID := uuid.UUID(entry.ID)
	password := "protected"
	maximum := int64(1)
	created, err := shareService.Create(ctx, shares.CreateInput{OwnerID: 1001, FileID: fileID, Password: &password, MaxDownloads: &maximum})
	if err != nil {
		t.Fatal(err)
	}
	path := "/v1/public/shares/" + created.Token + "/files/" + fileID.String() + "/playback"
	response = performRequest(t, server, http.MethodPost, path, nil, nil)
	if response.Code != 401 {
		t.Fatalf("missing password: %d", response.Code)
	}
	response = performRequest(t, server, http.MethodPost, path, nil, map[string]string{"X-Share-Password": password})
	if response.Code != 201 {
		t.Fatalf("playback: %d %s", response.Code, response.Body.String())
	}
	var playback gen.PlaybackSession
	if err := json.Unmarshal(response.Body.Bytes(), &playback); err != nil {
		t.Fatal(err)
	}
	playbackPath := "/v1/playback?ticket=" + playback.Ticket
	for _, part := range []struct{ rangeHeader, want string }{{"bytes=0-3", "abcd"}, {"bytes=6-9", "ghij"}, {"bytes=2-5", "cdef"}} {
		response = performRequest(t, server, http.MethodGet, playbackPath, nil, map[string]string{"Range": part.rangeHeader})
		if response.Code != 206 || response.Body.String() != part.want {
			t.Fatalf("range: %d %q", response.Code, response.Body.String())
		}
	}
	var count int64
	if err := db.Pool.QueryRow(ctx, "SELECT download_count FROM file_shares WHERE id=$1", created.Row.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reservations=%d", count)
	}
	response = performRequest(t, server, http.MethodPost, path, nil, map[string]string{"X-Share-Password": password})
	if response.Code == 201 {
		t.Fatal("second session bypassed download limit")
	}
	id, ok := dbtypes.GoogleUUID(created.Row.ID)
	if !ok {
		t.Fatal("invalid share ID")
	}
	if err := shareService.Revoke(ctx, 1001, id); err != nil {
		t.Fatal(err)
	}
	response = performRequest(t, server, http.MethodGet, playbackPath, nil, nil)
	if response.Code == 200 || response.Code == 206 {
		t.Fatal("revoked share still streams")
	}
	response = performRequest(t, server, http.MethodGet, "/v1/playback?ticket=00000000000000000000000000000000", nil, nil)
	if response.Code != 401 {
		t.Fatalf("forged ticket: %d", response.Code)
	}
}
