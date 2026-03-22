package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/tgdrive/teldrive/internal/api"
	authpkg "github.com/tgdrive/teldrive/internal/auth"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
	"github.com/tgdrive/teldrive/pkg/services"
	"github.com/tgdrive/teldrive/pkg/types"
)

var errPasswordRequired = fmt.Errorf("PASSWORD_AUTH_NEEDED")

func TestAuthRoutes_LoginSessionLogout(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	public := s.newClientWithToken("")
	loginRes, err := public.AuthLogin(ctx, &api.SessionCreate{
		Session:   "1BvXNhK1zA5P-FAKE-SESSION-7201",
		UserId:    7201,
		UserName:  "user7201",
		Name:      "user7201",
		IsPremium: false,
	})
	if err != nil {
		t.Fatalf("AuthLogin failed: %v", err)
	}

	token, err := tokenFromSetCookie(loginRes.SetCookie)
	if err != nil {
		t.Fatalf("tokenFromSetCookie failed: %v", err)
	}

	client := s.newClientWithToken(token)
	if _, err := client.AuthSession(ctx); err != nil {
		t.Fatalf("AuthSession failed: %v", err)
	}

	if _, err := client.AuthLogout(ctx); err != nil {
		t.Fatalf("AuthLogout failed: %v", err)
	}
}

func TestAuthRoutes_RefreshFlowCookieRotation(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	s.ensureUserExists(7301)

	sessionID := uuid.NewString()
	knownRefreshToken := "refresh-token-7301"
	hashBytes := sha256.Sum256([]byte(knownRefreshToken))
	refreshHash := hex.EncodeToString(hashBytes[:])
	now := time.Now().UTC()

	if err := s.repos.Sessions.Create(ctx, &jetmodel.Sessions{
		ID:               uuid.MustParse(sessionID),
		UserID:           7301,
		TgSession:        "1BvXNhK1zA5P-FAKE-SESSION-7301",
		RefreshTokenHash: &refreshHash,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("seed session with refresh hash: %v", err)
	}

	refreshReq, err := http.NewRequest(http.MethodPost, s.server.URL+"/auth/refresh", nil)
	if err != nil {
		t.Fatalf("new refresh request: %v", err)
	}
	refreshReq.AddCookie(&http.Cookie{Name: "refresh_token", Value: knownRefreshToken})

	refreshResp, err := s.httpCli.Do(refreshReq)
	if err != nil {
		t.Fatalf("refresh request failed: %v", err)
	}
	defer refreshResp.Body.Close()

	if refreshResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected refresh 204, got %d", refreshResp.StatusCode)
	}
	newAccessToken, err := tokenFromSetCookie(refreshResp.Header.Get("Set-Cookie"))
	if err != nil {
		t.Fatalf("expected refreshed access token set-cookie: %v", err)
	}
	authed := s.newClientWithToken(newAccessToken)
	if _, err := authed.AuthSession(ctx); err != nil {
		t.Fatalf("AuthSession with refreshed token failed: %v", err)
	}

	if _, err := s.repos.Sessions.GetByRefreshTokenHash(ctx, refreshHash); err == nil {
		t.Fatalf("old refresh token hash should be invalidated after refresh")
	}
	updatedSession, err := s.repos.Sessions.GetByID(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session after refresh: %v", err)
	}
	if updatedSession.RefreshTokenHash == nil || *updatedSession.RefreshTokenHash == "" || *updatedSession.RefreshTokenHash == refreshHash {
		t.Fatalf("expected rotated refresh token hash in session")
	}

	reuseReq, err := http.NewRequest(http.MethodPost, s.server.URL+"/auth/refresh", nil)
	if err != nil {
		t.Fatalf("new refresh reuse request: %v", err)
	}
	reuseReq.AddCookie(&http.Cookie{Name: "refresh_token", Value: knownRefreshToken})

	reuseResp, err := s.httpCli.Do(reuseReq)
	if err != nil {
		t.Fatalf("refresh reuse request failed: %v", err)
	}
	defer reuseResp.Body.Close()

	if reuseResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected old refresh token to be unauthorized, got %d (%s)", reuseResp.StatusCode, fmt.Sprint(reuseResp.Status))
	}
}

func TestAuthRoutes_LoginCookiesSecureWhenForwardedHTTPS(t *testing.T) {
	s := newSuite(t)

	body, err := json.Marshal(&api.SessionCreate{
		Session:   "1BvXNhK1zA5P-FAKE-SESSION-7401",
		UserId:    7401,
		UserName:  "user7401",
		Name:      "user7401",
		IsPremium: false,
	})
	if err != nil {
		t.Fatalf("marshal login payload: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, s.server.URL+"/auth/login", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")

	resp, err := s.httpCli.Do(req)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected login 204, got %d", resp.StatusCode)
	}

	access, ok := cookieByName(resp.Cookies(), "access_token")
	if !ok {
		t.Fatalf("missing access_token cookie")
	}
	if !access.Secure {
		t.Fatalf("expected access_token cookie to be Secure with X-Forwarded-Proto=https")
	}

}

func TestAuthRoutes_RefreshAfterExpiredAccessToken(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	const (
		userID       int64 = 7501
		sessionID          = "c58b4fd5-8e14-4ceb-af15-f1e5ac31a62a"
		refreshToken       = "refresh-token-7501"
	)
	seedSession(t, s, ctx, userID, sessionID, "1BvXNhK1zA5P-FAKE-SESSION-7501", refreshToken)

	expiredToken := mustToken(t, s, userID, sessionID, -1*time.Minute)
	client := s.newClientWithToken(expiredToken)

	_, err := client.AuthSession(ctx)
	if statusCode(err) != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired access token, got %d err=%v", statusCode(err), err)
	}

	refreshReq, err := http.NewRequest(http.MethodPost, s.server.URL+"/auth/refresh", nil)
	if err != nil {
		t.Fatalf("new refresh request: %v", err)
	}
	refreshReq.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshToken})

	refreshResp, err := s.httpCli.Do(refreshReq)
	if err != nil {
		t.Fatalf("refresh request failed: %v", err)
	}
	defer refreshResp.Body.Close()

	if refreshResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected refresh 204, got %d", refreshResp.StatusCode)
	}

	newAccess, err := tokenFromSetCookie(refreshResp.Header.Get("Set-Cookie"))
	if err != nil {
		t.Fatalf("expected refreshed access token set-cookie: %v", err)
	}
	newClient := s.newClientWithToken(newAccess)
	if _, err := newClient.AuthSession(ctx); err != nil {
		t.Fatalf("AuthSession with refreshed token failed: %v", err)
	}
}

func TestAuthRoutes_SessionWithAPIKeyDoesNotRotateCookie(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	_, client, _ := loginWithClient(t, s, 7701, "user7701")

	created, err := client.UsersCreateApiKey(ctx, &api.UserApiKeyCreate{Name: "session-api-key"})
	if err != nil {
		t.Fatalf("UsersCreateApiKey failed: %v", err)
	}

	apiKeyClient := s.newClientWithToken(created.Key)
	res, err := apiKeyClient.AuthSession(ctx)
	if err != nil {
		t.Fatalf("AuthSession with API key failed: %v", err)
	}
	sessionRes, ok := res.(*api.SessionHeaders)
	if !ok {
		t.Fatalf("expected SessionHeaders, got %T", res)
	}
	if sessionRes.Response.UserId != 7701 {
		t.Fatalf("expected userId=7701, got %d", sessionRes.Response.UserId)
	}
	if sessionRes.SetCookie.IsSet() {
		t.Fatalf("expected no Set-Cookie for API key auth")
	}
}

func TestAuthRoutes_LogoutRevokesRefreshToken(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	const (
		userID       int64 = 7601
		sessionID          = "5a212cc3-8e9e-41e2-9f53-ec65230815ac"
		refreshToken       = "refresh-token-7601"
	)
	seedSession(t, s, ctx, userID, sessionID, "1BvXNhK1zA5P-FAKE-SESSION-7601", refreshToken)

	accessToken := mustToken(t, s, userID, sessionID, 30*time.Minute)
	client := s.newClientWithToken(accessToken)

	if _, err := client.AuthLogout(ctx); err != nil {
		t.Fatalf("AuthLogout failed: %v", err)
	}

	if _, err := s.repos.Sessions.GetByID(ctx, sessionID); err == nil {
		t.Fatalf("expected session to be revoked")
	}

	refreshReq, err := http.NewRequest(http.MethodPost, s.server.URL+"/auth/refresh", nil)
	if err != nil {
		t.Fatalf("new refresh request: %v", err)
	}
	refreshReq.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshToken})

	refreshResp, err := s.httpCli.Do(refreshReq)
	if err != nil {
		t.Fatalf("refresh request failed: %v", err)
	}
	defer refreshResp.Body.Close()

	if refreshResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected refresh to be unauthorized after logout, got %d", refreshResp.StatusCode)
	}
}

func TestAuthRoutes_AttemptFlow_QRSuccess(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	s.tgMock.noAuthClientFn = func(ctx context.Context, _ tg.UpdateDispatcher, storage session.Storage) (services.TelegramClient, error) {
		mem := storage.(*session.StorageMemory)
		return &authFlowMockClient{
			qrLoginFn: func(ctx context.Context, _ qrlogin.LoggedIn, onToken func(context.Context, string) error) (*services.TelegramUser, error) {
				if err := onToken(ctx, "tg://login?token=test-qr-token"); err != nil {
					return nil, err
				}
				if err := storeMockSession(ctx, mem); err != nil {
					return nil, err
				}
				return &services.TelegramUser{ID: 8801, Username: "qruser", FirstName: "QR", LastName: "User"}, nil
			},
		}, nil
	}

	client := s.newClientWithToken("")
	attempt, err := client.AuthCreateAttempt(ctx, &api.AuthAttemptCreate{AuthType: api.AuthAttemptCreateAuthTypeQr})
	if err != nil {
		t.Fatalf("AuthCreateAttempt failed: %v", err)
	}
	if attempt.ID == "" {
		t.Fatalf("expected attempt id")
	}

	snapshot, err := waitForAttemptState(ctx, client, attempt.ID, api.AuthAttemptStateAuthenticated)
	if err != nil {
		t.Fatalf("waitForAttemptState authenticated: %v", err)
	}
	if token, ok := snapshot.Token.Get(); !ok || token == "" {
		t.Fatalf("expected authenticated snapshot token")
	}
	sessionVal, ok := snapshot.Session.Get()
	if !ok {
		t.Fatalf("expected authenticated snapshot session")
	}

	loginReq := &api.SessionCreate{
		Name:      sessionVal.Name,
		UserName:  sessionVal.UserName,
		UserId:    sessionVal.UserId,
		IsPremium: sessionVal.IsPremium,
		SessionId: sessionVal.SessionId,
		Hash:      sessionVal.Hash,
		Expires:   sessionVal.Expires,
		Session:   "1BvXNhK1zA5P-FAKE-SESSION-8801",
	}
	loginRes, err := client.AuthLogin(ctx, loginReq)
	if err != nil {
		t.Fatalf("AuthLogin from attempt session failed: %v", err)
	}
	if loginRes.SetCookie == "" {
		t.Fatalf("expected auth login cookie")
	}
}

func TestAuthRoutes_AttemptFlow_PhonePasswordSuccess(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	s.tgMock.noAuthClientFn = func(ctx context.Context, _ tg.UpdateDispatcher, storage session.Storage) (services.TelegramClient, error) {
		mem := storage.(*session.StorageMemory)
		return &authFlowMockClient{
			sendCodeFn: func(context.Context, string) (string, error) {
				return "hash-8802", nil
			},
			signInFn: func(context.Context, string, string, string) (*services.TelegramUser, error) {
				return nil, errPasswordRequired
			},
			passwordFn: func(ctx context.Context, password string) (*services.TelegramUser, error) {
				if password != "secret" {
					return nil, fmt.Errorf("bad password")
				}
				if err := storeMockSession(ctx, mem); err != nil {
					return nil, err
				}
				return &services.TelegramUser{ID: 8802, Username: "phoneuser", FirstName: "Phone", LastName: "User"}, nil
			},
		}, nil
	}
	s.tgMock.passwordAuthFn = func(err error) bool { return errors.Is(err, errPasswordRequired) }

	client := s.newClientWithToken("")
	attempt, err := client.AuthCreateAttempt(ctx, &api.AuthAttemptCreate{AuthType: api.AuthAttemptCreateAuthTypePhone, PhoneNo: api.NewOptString("+911234567890")})
	if err != nil {
		t.Fatalf("AuthCreateAttempt phone failed: %v", err)
	}

	codeSent, err := waitForAttemptState(ctx, client, attempt.ID, api.AuthAttemptStateCodeSent)
	if err != nil {
		t.Fatalf("waitForAttemptState code_sent: %v", err)
	}
	phoneCodeHash, ok := codeSent.PhoneCodeHash.Get()
	if !ok || phoneCodeHash != "hash-8802" {
		t.Fatalf("expected phoneCodeHash=hash-8802, got %+v", codeSent.PhoneCodeHash)
	}

	if err := client.AuthSignIn(ctx, &api.AuthAttemptSignIn{PhoneNo: "+911234567890", PhoneCode: "12345", PhoneCodeHash: phoneCodeHash}, api.AuthSignInParams{ID: attempt.ID}); err != nil {
		t.Fatalf("AuthSignIn failed: %v", err)
	}
	if _, err := waitForAttemptState(ctx, client, attempt.ID, api.AuthAttemptStatePasswordRequired); err != nil {
		t.Fatalf("waitForAttemptState password_required: %v", err)
	}

	if err := client.AuthPassword(ctx, &api.AuthAttemptPassword{Password: "secret"}, api.AuthPasswordParams{ID: attempt.ID}); err != nil {
		t.Fatalf("AuthPassword failed: %v", err)
	}
	authenticated, err := waitForAttemptState(ctx, client, attempt.ID, api.AuthAttemptStateAuthenticated)
	if err != nil {
		t.Fatalf("waitForAttemptState authenticated: %v", err)
	}
	sessionVal, ok := authenticated.Session.Get()
	if !ok {
		t.Fatalf("expected authenticated session in snapshot")
	}
	loginReq := &api.SessionCreate{
		Name:      sessionVal.Name,
		UserName:  sessionVal.UserName,
		UserId:    sessionVal.UserId,
		IsPremium: sessionVal.IsPremium,
		SessionId: sessionVal.SessionId,
		Hash:      sessionVal.Hash,
		Expires:   sessionVal.Expires,
		Session:   "1BvXNhK1zA5P-FAKE-SESSION-8802",
	}
	if _, err := client.AuthLogin(ctx, loginReq); err != nil {
		t.Fatalf("AuthLogin from password attempt failed: %v", err)
	}

	if err := client.AuthDeleteAttempt(ctx, api.AuthDeleteAttemptParams{ID: attempt.ID}); err != nil {
		t.Fatalf("AuthDeleteAttempt failed: %v", err)
	}
	_, err = client.AuthGetAttempt(ctx, api.AuthGetAttemptParams{ID: attempt.ID})
	if statusCode(err) != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d err=%v", statusCode(err), err)
	}
}

func seedSession(t *testing.T, s *suite, ctx context.Context, userID int64, sessionID, tgSession, refreshToken string) {
	t.Helper()
	s.ensureUserExists(userID)
	refreshHash := hashString(refreshToken)
	now := time.Now().UTC()
	if err := s.repos.Sessions.Create(ctx, &jetmodel.Sessions{
		ID:               uuid.MustParse(sessionID),
		UserID:           userID,
		TgSession:        tgSession,
		RefreshTokenHash: &refreshHash,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("seed session failed: %v", err)
	}
}

func mustToken(t *testing.T, s *suite, userID int64, sessionID string, ttl time.Duration) string {
	t.Helper()
	now := time.Now().UTC()
	claims := &types.JWTClaims{
		Name:      "Test User",
		UserName:  "test_user",
		IsPremium: false,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok, err := authpkg.Encode(s.cfg.JWT.Secret, claims)
	if err != nil {
		t.Fatalf("encode token: %v", err)
	}
	return tok
}

func hashString(v string) string {
	h := sha256.Sum256([]byte(v))
	return hex.EncodeToString(h[:])
}

func storeMockSession(ctx context.Context, storage *session.StorageMemory) error {
	data := &types.SessionData{Version: 1, Data: session.Data{DC: 2, AuthKey: []byte("mock-auth-key-1234567890")}}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return storage.StoreSession(ctx, b)
}

func waitForAttemptState(ctx context.Context, client *api.Client, attemptID string, want api.AuthAttemptState) (*api.AuthAttempt, error) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		attempt, err := client.AuthGetAttempt(ctx, api.AuthGetAttemptParams{ID: attemptID})
		if err != nil {
			return nil, err
		}
		if attempt.State == want {
			return attempt, nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return nil, fmt.Errorf("attempt %s did not reach state %s", attemptID, want)
}

func cookieByName(cookies []*http.Cookie, name string) (*http.Cookie, bool) {
	for _, c := range cookies {
		if c.Name == name {
			return c, true
		}
	}
	return nil, false
}
