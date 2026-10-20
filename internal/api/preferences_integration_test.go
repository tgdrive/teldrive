//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	api "github.com/tgdrive/teldrive/v2/internal/api"
	"github.com/tgdrive/teldrive/v2/internal/authn"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/health"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

type preferencesResponse struct {
	Uploads struct {
		Encryption        bool   `json:"encryption"`
		ConflictPolicy    string `json:"conflictPolicy"`
		Concurrency       int    `json:"concurrency"`
		PreferredPartSize int64  `json:"preferredPartSize"`
	} `json:"uploads"`
	Files struct {
		Sort  string `json:"sort"`
		Order string `json:"order"`
	} `json:"files"`
	Appearance struct {
		Theme  string  `json:"theme"`
		Locale *string `json:"locale"`
	} `json:"appearance"`
	UpdatedAt *time.Time `json:"updatedAt"`
}

func TestUserPreferencesPersistPerUserWithPartialUpdates(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001), (2002)"); err != nil {
		t.Fatal(err)
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
	handler := api.NewHandler(catalog.NewService(db.Pool, nil), uploads.NewService(db.Pool), nil, nil, health.NewService("test", db.Pool), 0, nil).
		ConfigureDomains(authService, nil, nil, nil, nil, nil)
	server, err := api.NewServer(handler, api.NewSecurity(tokenUserAuthenticator{"owner-token": 1001, "other-token": 2002}))
	if err != nil {
		t.Fatal(err)
	}
	request := func(token, method string, body string) (int, preferencesResponse) {
		t.Helper()
		headers := map[string]string{"Authorization": "Bearer " + token}
		var payload []byte
		if body != "" {
			payload = []byte(body)
			headers["Content-Type"] = "application/json"
		}
		response := performRequest(t, server, method, "/v1/me/preferences", payload, headers)
		var decoded preferencesResponse
		if response.Code == http.StatusOK {
			if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
				t.Fatalf("decode %s: %v", response.Body.String(), err)
			}
		}
		return response.Code, decoded
	}

	status, defaults := request("owner-token", http.MethodGet, "")
	if status != http.StatusOK {
		t.Fatalf("GET defaults status = %d", status)
	}
	if defaults.Uploads.Encryption || defaults.Uploads.ConflictPolicy != "rename" || defaults.Uploads.Concurrency != 3 ||
		defaults.Uploads.PreferredPartSize != 512<<20 || defaults.Files.Sort != "name" || defaults.Files.Order != "asc" ||
		defaults.Appearance.Theme != "system" || defaults.Appearance.Locale != nil || defaults.UpdatedAt != nil {
		t.Fatalf("defaults = %+v", defaults)
	}

	status, updated := request("owner-token", http.MethodPatch, `{"uploads":{"encryption":true,"preferredPartSize":1073741824},"appearance":{"locale":"pt-BR"}}`)
	if status != http.StatusOK || !updated.Uploads.Encryption || updated.Uploads.PreferredPartSize != 1<<30 ||
		updated.Appearance.Locale == nil || *updated.Appearance.Locale != "pt-BR" || updated.UpdatedAt == nil {
		t.Fatalf("PATCH = %d %+v", status, updated)
	}
	// A later partial update in another group must keep earlier values.
	status, merged := request("owner-token", http.MethodPatch, `{"files":{"sort":"size","order":"desc"},"appearance":{"theme":"dark"}}`)
	if status != http.StatusOK || !merged.Uploads.Encryption || merged.Uploads.ConflictPolicy != "rename" ||
		merged.Uploads.PreferredPartSize != 1<<30 || merged.Files.Sort != "size" || merged.Files.Order != "desc" ||
		merged.Appearance.Theme != "dark" || merged.Appearance.Locale == nil || *merged.Appearance.Locale != "pt-BR" {
		t.Fatalf("merged PATCH = %d %+v", status, merged)
	}
	if status, reread := request("owner-token", http.MethodGet, ""); status != http.StatusOK || reread.Uploads != merged.Uploads || reread.Files != merged.Files {
		t.Fatalf("GET after PATCH = %d %+v", status, reread)
	}

	if status, other := request("other-token", http.MethodGet, ""); status != http.StatusOK || other.Uploads.Encryption || other.UpdatedAt != nil {
		t.Fatalf("other user preferences = %d %+v", status, other)
	}

	for name, body := range map[string]string{
		"part size not a 16 MiB multiple": `{"uploads":{"preferredPartSize":20971520}}`,
		"part size above maximum":         `{"uploads":{"preferredPartSize":2164260864}}`,
		"concurrency above maximum":       `{"uploads":{"concurrency":13}}`,
		"unsupported sort":                `{"files":{"sort":"id"}}`,
		"unsupported theme":               `{"appearance":{"theme":"sepia"}}`,
		"unsupported locale":              `{"appearance":{"locale":"xx"}}`,
	} {
		if status, _ := request("owner-token", http.MethodPatch, body); status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
			t.Fatalf("%s: status = %d, want 400 or 422", name, status)
		}
	}
	if status, final := request("owner-token", http.MethodGet, ""); status != http.StatusOK || final.Uploads != merged.Uploads || final.Appearance.Theme != "dark" {
		t.Fatalf("rejected updates changed preferences: %d %+v", status, final)
	}

	if status, _ := request("missing-token", http.MethodGet, ""); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", status)
	}

	// Server enforcement is independent of the saved UI preference and its API.
	handler.UploadEncryptionRequired = true
	if status, other := request("other-token", http.MethodGet, ""); status != http.StatusOK || other.Uploads.Encryption {
		t.Fatalf("server policy changed UI defaults: %d %+v", status, other)
	}
	if status, updated := request("owner-token", http.MethodPatch, `{"uploads":{"encryption":false}}`); status != http.StatusOK || updated.Uploads.Encryption {
		t.Fatalf("server policy changed saved UI preference: %d %+v", status, updated)
	}
}

type tokenUserAuthenticator map[string]int64

func (a tokenUserAuthenticator) AuthenticateBearer(_ context.Context, token string) (api.Identity, error) {
	userID, ok := a[token]
	if !ok {
		return api.Identity{}, errors.New("invalid token")
	}
	return api.Identity{UserID: userID, Roles: []string{"user"}}, nil
}

func (tokenUserAuthenticator) AuthenticateAPIKey(context.Context, string) (api.Identity, error) {
	return api.Identity{}, errors.New("invalid key")
}

type unusedTelegramLogin struct{}

func (unusedTelegramLogin) Start(context.Context, string) (authn.LoginStep, error) {
	return authn.LoginStep{}, errors.New("unused")
}

func (unusedTelegramLogin) StartQR(context.Context) (authn.LoginStep, error) {
	return authn.LoginStep{}, errors.New("unused")
}

func (unusedTelegramLogin) PollQR(context.Context, []byte) (authn.LoginStep, error) {
	return authn.LoginStep{}, errors.New("unused")
}

func (unusedTelegramLogin) VerifyCode(context.Context, string, []byte, string) (authn.LoginStep, error) {
	return authn.LoginStep{}, errors.New("unused")
}

func (unusedTelegramLogin) VerifyPassword(context.Context, []byte, string) (authn.LoginStep, error) {
	return authn.LoginStep{}, errors.New("unused")
}
