package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/tgdrive/teldrive/internal/api"
)

func TestUsersApiKeys_ExpiryAndNoExpiry(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	_, client, _ := loginWithClient(t, s, 7210, "user7210")

	t.Run("create key without expiry and authenticate", func(t *testing.T) {
		created, err := client.UsersCreateApiKey(ctx, &api.UserApiKeyCreate{Name: "permanent-key"})
		if err != nil {
			t.Fatalf("UsersCreateApiKey failed: %v", err)
		}
		if created.Key == "" {
			t.Fatalf("expected plaintext key in create response")
		}
		if created.ExpiresAt.Set {
			t.Fatalf("expected no expiry for permanent key")
		}

		apiKeyClient := s.newClientWithToken(created.Key)
		if _, err := apiKeyClient.UsersStats(ctx); err != nil {
			t.Fatalf("UsersStats with API key failed: %v", err)
		}

		keys, err := client.UsersListApiKeys(ctx)
		if err != nil {
			t.Fatalf("UsersListApiKeys failed: %v", err)
		}
		found := false
		for _, key := range keys {
			if key.ID == created.ID {
				found = true
				if key.ExpiresAt.Set {
					t.Fatalf("expected listed key to have no expiry")
				}
				break
			}
		}
		if !found {
			t.Fatalf("created key not found in list")
		}
	})

	t.Run("expired key is rejected", func(t *testing.T) {
		expiresAt := time.Now().UTC().Add(30 * time.Minute)
		created, err := client.UsersCreateApiKey(ctx, &api.UserApiKeyCreate{
			Name:      "expiring-key",
			ExpiresAt: api.NewOptDateTime(expiresAt),
		})
		if err != nil {
			t.Fatalf("UsersCreateApiKey with expiry failed: %v", err)
		}
		if !created.ExpiresAt.Set {
			t.Fatalf("expected expiry in create response")
		}

		_, err = s.pool.Exec(ctx, "UPDATE teldrive.api_keys SET expires_at = timezone('utc'::text, now()) - interval '1 minute' WHERE id = $1", created.ID)
		if err != nil {
			t.Fatalf("force key expiry failed: %v", err)
		}

		apiKeyClient := s.newClientWithToken(created.Key)
		_, err = apiKeyClient.UsersStats(ctx)
		if statusCode(err) != 401 {
			t.Fatalf("expected 401 for expired api key, got %d err=%v", statusCode(err), err)
		}
	})

	t.Run("create key with past expiry is rejected", func(t *testing.T) {
		_, err := client.UsersCreateApiKey(ctx, &api.UserApiKeyCreate{
			Name:      "invalid-expiry",
			ExpiresAt: api.NewOptDateTime(time.Now().UTC().Add(-time.Minute)),
		})
		if statusCode(err) != 400 {
			t.Fatalf("expected 400 for past expiry, got %d err=%v", statusCode(err), err)
		}
	})

	t.Run("revoked key is rejected", func(t *testing.T) {
		created, err := client.UsersCreateApiKey(ctx, &api.UserApiKeyCreate{Name: "revoke-key"})
		if err != nil {
			t.Fatalf("UsersCreateApiKey failed: %v", err)
		}

		apiKeyClient := s.newClientWithToken(created.Key)
		if _, err := apiKeyClient.UsersStats(ctx); err != nil {
			t.Fatalf("UsersStats with API key before revoke failed: %v", err)
		}

		if err := client.UsersRemoveApiKey(ctx, api.UsersRemoveApiKeyParams{ID: created.ID}); err != nil {
			t.Fatalf("UsersRemoveApiKey failed: %v", err)
		}

		_, err = apiKeyClient.UsersStats(ctx)
		if statusCode(err) != 401 {
			t.Fatalf("expected 401 for revoked api key, got %d err=%v", statusCode(err), err)
		}
	})

	t.Run("api key requires at least one valid session", func(t *testing.T) {
		created, err := client.UsersCreateApiKey(ctx, &api.UserApiKeyCreate{Name: "session-required-key"})
		if err != nil {
			t.Fatalf("UsersCreateApiKey failed: %v", err)
		}

		apiKeyClient := s.newClientWithToken(created.Key)
		if _, err := apiKeyClient.UsersStats(ctx); err != nil {
			t.Fatalf("UsersStats with API key before session revoke failed: %v", err)
		}

		sessions, err := client.UsersListSessions(ctx)
		if err != nil {
			t.Fatalf("UsersListSessions failed: %v", err)
		}
		if len(sessions) == 0 {
			t.Fatalf("expected at least one session")
		}

		if err := client.UsersRemoveSession(ctx, api.UsersRemoveSessionParams{ID: sessions[0].SessionId}); err != nil {
			t.Fatalf("UsersRemoveSession failed: %v", err)
		}

		_, err = apiKeyClient.UsersStats(ctx)
		if statusCode(err) != 401 {
			t.Fatalf("expected 401 for api key without any valid session, got %d err=%v", statusCode(err), err)
		}
	})

	t.Run("api key cache is invalidated on auth logout", func(t *testing.T) {
		_, logoutClient, _ := loginWithClient(t, s, 7211, "user7211")

		created, err := logoutClient.UsersCreateApiKey(ctx, &api.UserApiKeyCreate{Name: "logout-invalidation-key"})
		if err != nil {
			t.Fatalf("UsersCreateApiKey failed: %v", err)
		}

		apiKeyClient := s.newClientWithToken(created.Key)
		if _, err := apiKeyClient.UsersStats(ctx); err != nil {
			t.Fatalf("UsersStats with API key before logout failed: %v", err)
		}

		if _, err := logoutClient.AuthLogout(ctx); err != nil {
			t.Fatalf("AuthLogout failed: %v", err)
		}

		_, err = apiKeyClient.UsersStats(ctx)
		if statusCode(err) != 401 {
			t.Fatalf("expected 401 for api key after logout invalidation, got %d err=%v", statusCode(err), err)
		}
	})
}
