//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	api "github.com/tgdrive/teldrive/v2/internal/api"
	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/channels"
	"github.com/tgdrive/teldrive/v2/internal/health"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

func TestPutUploadPartExplicitChannel(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO users (user_id) VALUES (1001), (1002);
INSERT INTO channels (channel_id, user_id, name, selected) VALUES
    (9001, 1001, 'selected', true),
    (9002, 1001, 'explicit', false),
    (9003, 1002, 'foreign', true)`); err != nil {
		t.Fatal(err)
	}
	uploadService := uploads.NewService(db.Pool)
	channelService := channels.NewService(db.Pool, nil, channels.Config{PartLimit: 1})
	pipeline := transfer.NewPipeline(uploadService, channelService, &apiMemoryStorage{}, nil, transfer.Config{})
	handler := api.NewHandler(catalog.NewService(db.Pool, nil), uploadService, pipeline, nil, health.NewService("test", db.Pool), 0, nil)
	server, err := api.NewServer(handler, api.NewSecurity(apiAuthenticator{}))
	if err != nil {
		t.Fatal(err)
	}

	createUpload := func(name string) string {
		t.Helper()
		body := []byte(`{"name":"` + name + `","size":1,"modTime":"2026-07-01T00:00:00Z","preferredPartSize":1048576}`)
		response := performRequest(t, server, http.MethodPost, "/v1/uploads", body, map[string]string{
			"Authorization": "Bearer test-token", "Content-Type": "application/json", "Idempotency-Key": uuid.NewString(),
		})
		if response.Code != http.StatusCreated {
			t.Fatalf("create upload status = %d, body=%s", response.Code, response.Body.String())
		}
		var session gen.UploadSession
		if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil {
			t.Fatal(err)
		}
		return uuid.UUID(session.ID).String()
	}
	putPart := func(uploadID, query string) int {
		t.Helper()
		response := performRequest(t, server, http.MethodPut, "/v1/uploads/"+uploadID+"/parts/1"+query, []byte("x"), map[string]string{
			"Authorization": "Bearer test-token", "Content-Type": "application/octet-stream",
		})
		return response.Code
	}
	partChannel := func(uploadID string) int64 {
		t.Helper()
		var channelID int64
		if err := db.Pool.QueryRow(ctx, "SELECT channel_id FROM upload_parts WHERE upload_id=$1 AND part_no=1", uploadID).Scan(&channelID); err != nil {
			t.Fatal(err)
		}
		return channelID
	}

	first := createUpload("first.bin")
	if code := putPart(first, "?channelId=9003"); code != http.StatusNotFound {
		t.Fatalf("foreign channel status = %d, want 404", code)
	}
	if code := putPart(first, "?channelId=9002"); code != http.StatusCreated {
		t.Fatalf("explicit channel status = %d, want 201", code)
	}
	if got := partChannel(first); got != 9002 {
		t.Fatalf("explicit part channel = %d, want 9002", got)
	}

	second := createUpload("second.bin")
	if code := putPart(second, "?channelId=9002"); code != http.StatusConflict {
		t.Fatalf("full explicit channel status = %d, want 409", code)
	}
	if code := putPart(second, ""); code != http.StatusCreated {
		t.Fatalf("selected channel status = %d, want 201", code)
	}
	if got := partChannel(second); got != 9001 {
		t.Fatalf("default part channel = %d, want selected 9001", got)
	}
}
