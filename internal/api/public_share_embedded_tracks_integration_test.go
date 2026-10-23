//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	api "github.com/tgdrive/teldrive/v2/internal/api"
	"github.com/tgdrive/teldrive/v2/internal/authn"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/health"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	"github.com/tgdrive/teldrive/v2/internal/shares"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

// Visitors of a public share have no settings, so the share page learns from the server whether the
// owner lets the player read the audio tracks and subtitles inside the file.
func TestPublicShareReportsTheOwnersEmbeddedTracksChoice(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001), (2002)"); err != nil {
		t.Fatal(err)
	}
	fileIDs := map[int64]uuid.UUID{1001: uuid.New(), 2002: uuid.New()}
	for owner, id := range fileIDs {
		if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,name,kind,mime_type,size,encryption,status,mod_time)
VALUES ($1,$2,'episode.mkv','file','video/x-matroska',10,false,'active',now())`, id, owner); err != nil {
			t.Fatal(err)
		}
	}
	cipher, err := secureblob.New("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	authService, err := authn.NewService(db.Pool, cipher, unusedTelegramLogin{}, authn.Config{
		SigningKey: "0123456789abcdef0123456789abcdef", Issuer: "test",
		AccessTokenTTL: time.Hour, RefreshTokenTTL: time.Hour, LoginFlowTTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	catalogService := catalog.NewService(db.Pool, nil)
	shareService, err := shares.NewService(db.Pool, catalogService)
	if err != nil {
		t.Fatal(err)
	}
	handler := api.NewHandler(catalogService, uploads.NewService(db.Pool), nil, nil, health.NewService("test", db.Pool), 0, nil).
		ConfigureDomains(authService, nil, nil, nil, shareService, nil)
	server, err := api.NewServer(handler, api.NewSecurity(tokenUserAuthenticator{"owner-token": 1001}))
	if err != nil {
		t.Fatal(err)
	}
	tokens := map[int64]string{}
	for owner, id := range fileIDs {
		created, err := shareService.Create(ctx, shares.CreateInput{OwnerID: owner, FileID: id})
		if err != nil {
			t.Fatalf("create share for %d: %v", owner, err)
		}
		tokens[owner] = created.Token
	}
	// Anonymous: no Authorization header.
	embeddedTracks := func(owner int64) bool {
		t.Helper()
		response := performRequest(t, server, http.MethodGet, "/v1/public/shares/"+tokens[owner], nil, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("public share of %d: status %d, body %s", owner, response.Code, response.Body.String())
		}
		var share struct {
			EmbeddedTracks bool `json:"embeddedTracks"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &share); err != nil {
			t.Fatalf("decode %s: %v", response.Body.String(), err)
		}
		return share.EmbeddedTracks
	}

	if embeddedTracks(1001) || embeddedTracks(2002) {
		t.Fatal("shares report embedded tracks as allowed before anyone chose it")
	}

	yes, no := true, false
	// Shares do not get it from the account setting alone: the owner has to allow it on shares.
	if _, err := authService.UpdatePreferences(ctx, 1001, authn.PreferencesUpdate{EmbeddedTracks: &yes}); err != nil {
		t.Fatal(err)
	}
	if embeddedTracks(1001) {
		t.Fatal("the account setting alone allowed it on the share")
	}

	if _, err := authService.UpdatePreferences(ctx, 1001, authn.PreferencesUpdate{EmbeddedTracksOnShares: &yes}); err != nil {
		t.Fatal(err)
	}
	if !embeddedTracks(1001) {
		t.Fatal("the owner's choice is not reported on their share")
	}
	if embeddedTracks(2002) {
		t.Fatal("one owner's choice leaked to another owner's share")
	}

	// Turning the account setting off takes the share with it.
	if _, err := authService.UpdatePreferences(ctx, 1001, authn.PreferencesUpdate{EmbeddedTracks: &no}); err != nil {
		t.Fatal(err)
	}
	if embeddedTracks(1001) {
		t.Fatal("turning the account setting off is not reported on the share")
	}
}
