//go:build integration

package authn

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestLoginRefreshAPIKeyAndLogoutAgainstRealPostgres(t *testing.T) {
	db := testpostgres.New(t)
	cipher, err := secureblob.NewWithKey(bytes.Repeat([]byte{5}, 32), bytes.NewReader(bytes.Repeat([]byte{9}, 24*20)))
	if err != nil {
		t.Fatal(err)
	}
	gateway := &fakeTelegramLogin{}
	service, err := NewService(db.Pool, cipher, gateway, Config{
		SigningKey: "0123456789abcdef0123456789abcdef", Issuer: "test",
		AccessTokenTTL: time.Hour, RefreshTokenTTL: 24 * time.Hour, LoginFlowTTL: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	service.random = bytes.NewReader(bytes.Join([][]byte{bytes.Repeat([]byte{7}, 32), bytes.Repeat([]byte{8}, 32), bytes.Repeat([]byte{9}, 32), bytes.Repeat([]byte{10}, 32), bytes.Repeat([]byte{11}, 32)}, nil))
	testNow := time.Now().UTC()
	service.now = func() time.Time { return testNow }
	ctx := context.Background()

	flow, err := service.StartLogin(ctx, "+15551234567")
	if err != nil {
		t.Fatalf("StartLogin() error = %v", err)
	}
	if _, err := service.VerifyCode(ctx, uuid.New(), "12345"); !errors.Is(err, ErrFlowNotFound) {
		t.Fatalf("unknown flow error = %v", err)
	}
	if flow.ID == uuid.Nil || flow.PasswordRequired {
		t.Fatalf("flow = %#v", flow)
	}
	expiredFlow, err := service.StartLogin(ctx, "+15557654321")
	if err != nil {
		t.Fatalf("StartLogin(expired) error = %v", err)
	}
	if _, err := db.Pool.Exec(ctx, "UPDATE telegram_login_flows SET expires_at=now()-interval '1 minute' WHERE id=$1", expiredFlow.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.VerifyCode(ctx, expiredFlow.ID, "12345"); !errors.Is(err, ErrFlowNotFound) {
		t.Fatalf("expired flow error = %v", err)
	}
	pending, err := service.VerifyCode(ctx, flow.ID, "12345")
	if err != nil {
		t.Fatalf("VerifyCode() error = %v", err)
	}
	if pending.Flow == nil || !pending.Flow.PasswordRequired || pending.Tokens != nil {
		t.Fatalf("pending result = %#v", pending)
	}
	completed, err := service.VerifyPassword(ctx, flow.ID, "correct horse battery staple")
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if completed.Tokens == nil || completed.Tokens.AccessToken == "" || completed.Tokens.RefreshToken == "" {
		t.Fatalf("completed result = %#v", completed)
	}

	identity, err := service.AuthenticateBearer(ctx, completed.Tokens.AccessToken)
	if err != nil {
		t.Fatalf("AuthenticateBearer() error = %v", err)
	}
	if identity.UserID != 1001 || identity.SessionID == uuid.Nil || identity.Source != "bearer" {
		t.Fatalf("identity = %#v", identity)
	}
	user, err := service.GetUser(ctx, 1001)
	if err != nil || user.UserID != 1001 || !user.Premium {
		t.Fatalf("GetUser() = %#v, %v", user, err)
	}

	secondSessionID := uuid.New()
	if _, err := service.queries.CreateSession(ctx, sqlcgen.CreateSessionParams{
		ID: dbtypes.UUID(secondSessionID), UserID: 1001,
		TelegramSession:  []byte("encrypted-second-session"),
		RefreshTokenHash: bytes.Repeat([]byte{42}, 32),
		ExpiresAt:        dbtypes.Time(testNow.Add(12 * time.Hour)),
	}); err != nil {
		t.Fatalf("create second session: %v", err)
	}
	sessions, err := service.ListSessions(ctx, ListSessionsInput{UserID: 1001, Limit: 10})
	if err != nil || len(sessions) != 2 {
		t.Fatalf("ListSessions() = %#v, %v", sessions, err)
	}
	firstSessionID, _ := dbtypes.GoogleUUID(sessions[0].ID)
	firstSessionTime := sessions[0].CreatedAt.Time
	nextSessions, err := service.ListSessions(ctx, ListSessionsInput{
		UserID: 1001, AfterID: &firstSessionID, AfterCreatedAt: &firstSessionTime, Limit: 1,
	})
	if err != nil || len(nextSessions) != 1 {
		t.Fatalf("ListSessions(next) = %#v, %v", nextSessions, err)
	}
	if err := service.RevokeSession(ctx, 1001, secondSessionID); err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
	}
	if err := service.RevokeSession(ctx, 2002, secondSessionID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("cross-user RevokeSession() error = %v", err)
	}
	renewed, err := service.RenewAccess(ctx, completed.Tokens.RefreshToken)
	if err != nil || renewed.AccessToken == "" || renewed.ExpiresIn <= 0 {
		t.Fatalf("RenewAccess() = %#v, %v", renewed, err)
	}
	renewedIdentity, err := service.AuthenticateBearer(ctx, renewed.AccessToken)
	if err != nil || renewedIdentity.UserID != 1001 || renewedIdentity.SessionID != identity.SessionID {
		t.Fatalf("renewed access identity = %#v, %v", renewedIdentity, err)
	}

	rotated, err := service.Refresh(ctx, completed.Tokens.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if rotated.RefreshToken == completed.Tokens.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	if _, err := service.Refresh(ctx, completed.Tokens.RefreshToken); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("old refresh token error = %v", err)
	}

	expires := service.now().Add(time.Hour)
	createdKey, err := service.CreateAPIKey(ctx, 1001, "automation", &expires)
	if err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}
	if !strings.HasPrefix(createdKey.Secret, "tdk_") {
		t.Fatalf("API key = %q", createdKey.Secret)
	}
	apiIdentity, err := service.AuthenticateAPIKey(ctx, createdKey.Secret)
	if err != nil || apiIdentity.UserID != 1001 || apiIdentity.Source != "api_key" {
		t.Fatalf("API key identity = %#v, %v", apiIdentity, err)
	}
	secondKey, err := service.CreateAPIKey(ctx, 1001, "second", nil)
	if err != nil {
		t.Fatalf("CreateAPIKey(second) error = %v", err)
	}
	defaultKeys, err := service.ListAPIKeys(ctx, ListAPIKeysInput{UserID: 1001})
	if err != nil || len(defaultKeys) != 2 {
		t.Fatalf("ListAPIKeys(default) = %#v, %v", defaultKeys, err)
	}
	keys, err := service.ListAPIKeys(ctx, ListAPIKeysInput{UserID: 1001, Limit: 10})
	if err != nil || len(keys) != 2 {
		t.Fatalf("ListAPIKeys() = %#v, %v", keys, err)
	}
	page, err := service.ListAPIKeys(ctx, ListAPIKeysInput{UserID: 1001, Limit: 1})
	if err != nil || len(page) != 1 {
		t.Fatalf("ListAPIKeys(page) = %#v, %v", page, err)
	}
	pageID, _ := dbtypes.GoogleUUID(page[0].ID)
	pageTime := page[0].CreatedAt.Time
	nextPage, err := service.ListAPIKeys(ctx, ListAPIKeysInput{UserID: 1001, AfterID: &pageID, AfterCreatedAt: &pageTime, Limit: 500})
	if err != nil || len(nextPage) != 1 {
		t.Fatalf("ListAPIKeys(next) = %#v, %v", nextPage, err)
	}
	keyID, _ := dbtypes.GoogleUUID(createdKey.Row.ID)
	if err := service.RevokeAPIKey(ctx, 1001, keyID); err != nil {
		t.Fatalf("RevokeAPIKey() error = %v", err)
	}
	if _, err := service.AuthenticateAPIKey(ctx, createdKey.Secret); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("revoked API key error = %v", err)
	}
	secondKeyID, _ := dbtypes.GoogleUUID(secondKey.Row.ID)
	if err := service.RevokeAPIKey(ctx, 1001, secondKeyID); err != nil {
		t.Fatalf("RevokeAPIKey(second) error = %v", err)
	}

	if err := service.Logout(ctx, identity); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := service.AuthenticateBearer(ctx, completed.Tokens.AccessToken); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("logged out bearer error = %v", err)
	}

	var phoneCiphertext, stateCiphertext, sessionCiphertext []byte
	if err := db.Pool.QueryRow(ctx, "SELECT phone_number_ciphertext, telegram_state_ciphertext FROM telegram_login_flows WHERE id=$1", flow.ID).Scan(&phoneCiphertext, &stateCiphertext); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, "SELECT telegram_session FROM sessions WHERE user_id=1001").Scan(&sessionCiphertext); err != nil {
		t.Fatal(err)
	}
	for name, ciphertext := range map[string][]byte{"phone": phoneCiphertext, "state": stateCiphertext, "session": sessionCiphertext} {
		if bytes.Contains(ciphertext, []byte("+15551234567")) || bytes.Contains(ciphertext, []byte("password-state")) || bytes.Contains(ciphertext, []byte("authorized-session")) {
			t.Fatalf("%s ciphertext leaked plaintext: %q", name, ciphertext)
		}
	}
	if gateway.startCalls != 2 || gateway.codeCalls != 1 || gateway.passwordCalls != 1 || gateway.active != 0 {
		t.Fatalf("gateway calls/active = start %d code %d password %d active %d", gateway.startCalls, gateway.codeCalls, gateway.passwordCalls, gateway.active)
	}
}

func TestQRLoginResumesAcrossServiceInstancesAndEnforcesAllowlist(t *testing.T) {
	db := testpostgres.New(t)
	cipher, err := secureblob.NewWithKey(bytes.Repeat([]byte{6}, 32), bytes.NewReader(bytes.Repeat([]byte{8}, 24*20)))
	if err != nil {
		t.Fatal(err)
	}
	gateway := &fakeQRLogin{username: "alloweduser"}
	cfg := Config{
		SigningKey: "0123456789abcdef0123456789abcdef", Issuer: "test",
		AllowedUsers: []string{"@AllowedUser"}, AccessTokenTTL: time.Hour,
		RefreshTokenTTL: 24 * time.Hour, LoginFlowTTL: 10 * time.Minute,
	}
	first, err := NewService(db.Pool, cipher, gateway, cfg)
	if err != nil {
		t.Fatal(err)
	}
	first.random = bytes.NewReader(bytes.Repeat([]byte{12}, 64))
	flow, err := first.StartQR(context.Background())
	if err != nil {
		t.Fatalf("StartQR() error = %v", err)
	}
	if flow.ID == uuid.Nil || flow.QRURL != "tg://login?token=first" || flow.PasswordRequired {
		t.Fatalf("StartQR() = %#v", flow)
	}

	// A new service instance proves the flow is resumed from PostgreSQL rather
	// than process-local state.
	second, err := NewService(db.Pool, cipher, gateway, cfg)
	if err != nil {
		t.Fatal(err)
	}
	second.random = bytes.NewReader(bytes.Repeat([]byte{13}, 64))
	pending, err := second.PollQR(context.Background(), flow.ID)
	if err != nil {
		t.Fatalf("PollQR(pending) error = %v", err)
	}
	if pending.QRFlow == nil || pending.QRFlow.QRURL != "tg://login?token=second" || pending.Tokens != nil {
		t.Fatalf("PollQR(pending) = %#v", pending)
	}
	completed, err := second.PollQR(context.Background(), flow.ID)
	if err != nil {
		t.Fatalf("PollQR(completed) error = %v", err)
	}
	if completed.Tokens == nil || completed.Tokens.AccessToken == "" || completed.Tokens.RefreshToken == "" {
		t.Fatalf("PollQR(completed) = %#v", completed)
	}

	var method string
	var phone []byte
	var state []byte
	if err := db.Pool.QueryRow(context.Background(), `
SELECT method::text, phone_number_ciphertext, telegram_state_ciphertext
FROM telegram_login_flows WHERE id=$1`, flow.ID).Scan(&method, &phone, &state); err != nil {
		t.Fatal(err)
	}
	if method != "qr" || phone != nil || bytes.Contains(state, []byte("qr-state")) {
		t.Fatalf("persisted QR flow method=%q phone=%q state=%q", method, phone, state)
	}

	blockedGateway := &fakeQRLogin{username: "blockeduser", completeOnFirstPoll: true}
	blocked, err := NewService(db.Pool, cipher, blockedGateway, cfg)
	if err != nil {
		t.Fatal(err)
	}
	blockedFlow, err := blocked.StartQR(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := blocked.PollQR(context.Background(), blockedFlow.ID); !errors.Is(err, ErrUserNotAllowed) {
		t.Fatalf("blocked PollQR() error = %v", err)
	}
	var blockedSessions int
	if err := db.Pool.QueryRow(context.Background(), "SELECT count(*) FROM sessions WHERE user_id=$1", int64(2002)).Scan(&blockedSessions); err != nil {
		t.Fatal(err)
	}
	if blockedSessions != 0 {
		t.Fatalf("blocked user sessions = %d", blockedSessions)
	}
}

type fakeTelegramLogin struct {
	mu            sync.Mutex
	startCalls    int
	codeCalls     int
	passwordCalls int
	active        int
}

func (f *fakeTelegramLogin) Start(context.Context, string) (LoginStep, error) {
	f.mu.Lock()
	f.startCalls++
	f.active++
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.active--; f.mu.Unlock() }()
	return LoginStep{State: []byte("code-state")}, nil
}

func (f *fakeTelegramLogin) StartQR(context.Context) (LoginStep, error) {
	return LoginStep{}, ErrLoginStateInvalid
}

func (f *fakeTelegramLogin) PollQR(context.Context, []byte) (LoginStep, error) {
	return LoginStep{}, ErrLoginStateInvalid
}
func (f *fakeTelegramLogin) VerifyCode(context.Context, string, []byte, string) (LoginStep, error) {
	f.mu.Lock()
	f.codeCalls++
	f.active++
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.active--; f.mu.Unlock() }()
	return LoginStep{State: []byte("password-state"), PasswordRequired: true}, nil
}

func (f *fakeTelegramLogin) VerifyPassword(context.Context, []byte, string) (LoginStep, error) {
	f.mu.Lock()
	f.passwordCalls++
	f.active++
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.active--; f.mu.Unlock() }()
	return LoginStep{
		User:    &TelegramUser{ID: 1001, DisplayName: "Test User", Username: "testuser", Premium: true},
		Session: []byte("authorized-session"),
	}, nil
}

type fakeQRLogin struct {
	mu                  sync.Mutex
	polls               int
	username            string
	completeOnFirstPoll bool
}

func (f *fakeQRLogin) Start(context.Context, string) (LoginStep, error) {
	return LoginStep{}, ErrLoginStateInvalid
}

func (f *fakeQRLogin) VerifyCode(context.Context, string, []byte, string) (LoginStep, error) {
	return LoginStep{}, ErrLoginStateInvalid
}

func (f *fakeQRLogin) VerifyPassword(context.Context, []byte, string) (LoginStep, error) {
	return LoginStep{}, ErrLoginStateInvalid
}

func (f *fakeQRLogin) StartQR(context.Context) (LoginStep, error) {
	return LoginStep{
		State: []byte("qr-state-first"), QRURL: "tg://login?token=first",
		QRExpiresAt: time.Now().UTC().Add(time.Minute),
	}, nil
}

func (f *fakeQRLogin) PollQR(context.Context, []byte) (LoginStep, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polls++
	if !f.completeOnFirstPoll && f.polls == 1 {
		return LoginStep{
			State: []byte("qr-state-second"), QRURL: "tg://login?token=second",
			QRExpiresAt: time.Now().UTC().Add(time.Minute),
		}, nil
	}
	userID := int64(1001)
	if f.username == "blockeduser" {
		userID = 2002
	}
	return LoginStep{
		User:    &TelegramUser{ID: userID, DisplayName: "QR User", Username: f.username},
		Session: []byte("qr-authorized-session"),
	}, nil
}
