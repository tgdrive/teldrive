package authn

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/principal"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
)

var (
	ErrInvalidInput      = errors.New("invalid authentication input")
	ErrFlowNotFound      = errors.New("Telegram login flow not found or expired")
	ErrInvalidCredential = errors.New("invalid authentication credential")
	ErrUserNotAllowed    = errors.New("Telegram user is not allowed")
	ErrSessionNotFound   = errors.New("session not found")
	ErrAPIKeyNotFound    = errors.New("API key not found")
)

type Config struct {
	SigningKey      string
	Issuer          string
	AllowedUsers    []string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	LoginFlowTTL    time.Duration
}

type Service struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
	cipher  *secureblob.Cipher
	login   TelegramLogin
	config  Config
	random  io.Reader
	now     func() time.Time
}

type FlowResult struct {
	ID               uuid.UUID
	ExpiresAt        time.Time
	PasswordRequired bool
}

type QRFlowResult struct {
	ID               uuid.UUID
	ExpiresAt        time.Time
	QRURL            string
	QRExpiresAt      time.Time
	PasswordRequired bool
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int32
}

type AccessRenewal struct {
	AccessToken string
	ExpiresIn   int32
}

type VerifyResult struct {
	Flow   *FlowResult
	QRFlow *QRFlowResult
	Tokens *TokenPair
}

type APIKeyCreated struct {
	Row    *sqlcgen.ApiKey
	Secret string
}

type ListAPIKeysInput struct {
	UserID         int64
	AfterCreatedAt *time.Time
	AfterID        *uuid.UUID
	Limit          int32
}

type ListSessionsInput struct {
	UserID         int64
	AfterCreatedAt *time.Time
	AfterID        *uuid.UUID
	Limit          int32
}

type accessClaims struct {
	SessionID uuid.UUID `json:"sid"`
	Roles     []string  `json:"roles,omitempty"`
	jwt.RegisteredClaims
}

func (s *Service) RefreshTokenTTL() time.Duration {
	if s == nil {
		return 0
	}
	return s.config.RefreshTokenTTL
}

func NewService(pool *pgxpool.Pool, cipher *secureblob.Cipher, login TelegramLogin, cfg Config) (*Service, error) {
	if pool == nil || cipher == nil || login == nil || len(cfg.SigningKey) < 32 || strings.TrimSpace(cfg.Issuer) == "" || cfg.AccessTokenTTL <= 0 || cfg.RefreshTokenTTL <= 0 || cfg.LoginFlowTTL <= 0 {
		return nil, ErrInvalidInput
	}
	cfg.AllowedUsers = normalizeAllowedUsers(cfg.AllowedUsers)
	return &Service{
		pool: pool, queries: sqlcgen.New(pool), cipher: cipher, login: login, config: cfg,
		random: rand.Reader, now: time.Now,
	}, nil
}

func (s *Service) StartLogin(ctx context.Context, phone string) (*FlowResult, error) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return nil, ErrInvalidInput
	}
	step, err := s.login.Start(ctx, phone)
	if err != nil {
		return nil, err
	}
	phoneCiphertext, err := s.cipher.Seal("login-phone", []byte(phone))
	if err != nil {
		return nil, err
	}
	stateCiphertext, err := s.cipher.Seal("login-state", step.State)
	if err != nil {
		return nil, err
	}
	id := uuid.New()
	expires := s.now().UTC().Add(s.config.LoginFlowTTL)
	row, err := s.queries.CreateTelegramLoginFlow(ctx, sqlcgen.CreateTelegramLoginFlowParams{
		ID: dbtypes.UUID(id), Method: sqlcgen.TelegramLoginMethodPhone,
		PhoneNumberCiphertext: phoneCiphertext, TelegramStateCiphertext: stateCiphertext,
		PasswordRequired: step.PasswordRequired, ExpiresAt: dbtypes.Time(expires),
	})
	if err != nil {
		return nil, fmt.Errorf("create Telegram login flow: %w", err)
	}
	return &FlowResult{ID: id, ExpiresAt: row.ExpiresAt.Time, PasswordRequired: row.PasswordRequired}, nil
}

func (s *Service) StartQR(ctx context.Context) (*QRFlowResult, error) {
	step, err := s.login.StartQR(ctx)
	if err != nil {
		return nil, err
	}
	if step.User != nil || len(step.State) == 0 || (!step.PasswordRequired && strings.TrimSpace(step.QRURL) == "") {
		return nil, ErrLoginStateInvalid
	}
	stateCiphertext, err := s.cipher.Seal("login-state", step.State)
	if err != nil {
		return nil, err
	}
	id := uuid.New()
	expires := s.now().UTC().Add(s.config.LoginFlowTTL)
	row, err := s.queries.CreateTelegramLoginFlow(ctx, sqlcgen.CreateTelegramLoginFlowParams{
		ID: dbtypes.UUID(id), Method: sqlcgen.TelegramLoginMethodQr,
		TelegramStateCiphertext: stateCiphertext, PasswordRequired: step.PasswordRequired,
		ExpiresAt: dbtypes.Time(expires),
	})
	if err != nil {
		return nil, fmt.Errorf("create Telegram QR login flow: %w", err)
	}
	return &QRFlowResult{
		ID: id, ExpiresAt: row.ExpiresAt.Time, QRURL: step.QRURL,
		QRExpiresAt: step.QRExpiresAt, PasswordRequired: row.PasswordRequired,
	}, nil
}

func (s *Service) PollQR(ctx context.Context, flowID uuid.UUID) (*VerifyResult, error) {
	if flowID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.withFlowLock(ctx, flowID, func(conn *pgxpool.Conn, flow *sqlcgen.TelegramLoginFlow) (*VerifyResult, error) {
		if flow.Method != sqlcgen.TelegramLoginMethodQr {
			return nil, ErrInvalidInput
		}
		state, err := s.cipher.Open("login-state", flow.TelegramStateCiphertext)
		if err != nil {
			return nil, err
		}
		step, err := s.login.PollQR(ctx, state)
		if err != nil {
			return nil, err
		}
		if step.User != nil {
			return s.completeLogin(ctx, conn, flowID, step)
		}
		if len(step.State) == 0 || (!step.PasswordRequired && strings.TrimSpace(step.QRURL) == "") {
			return nil, ErrLoginStateInvalid
		}
		ciphertext, err := s.cipher.Seal("login-state", step.State)
		if err != nil {
			return nil, err
		}
		updated, err := sqlcgen.New(conn).UpdateTelegramLoginFlowState(ctx, sqlcgen.UpdateTelegramLoginFlowStateParams{
			TelegramStateCiphertext: ciphertext, PasswordRequired: step.PasswordRequired, ID: flow.ID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFlowNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("update Telegram QR login flow: %w", err)
		}
		id, _ := dbtypes.GoogleUUID(updated.ID)
		return &VerifyResult{QRFlow: &QRFlowResult{
			ID: id, ExpiresAt: updated.ExpiresAt.Time, QRURL: step.QRURL,
			QRExpiresAt: step.QRExpiresAt, PasswordRequired: updated.PasswordRequired,
		}}, nil
	})
}
func (s *Service) VerifyCode(ctx context.Context, flowID uuid.UUID, code string) (*VerifyResult, error) {
	if flowID == uuid.Nil || strings.TrimSpace(code) == "" {
		return nil, ErrInvalidInput
	}
	return s.withFlowLock(ctx, flowID, func(conn *pgxpool.Conn, flow *sqlcgen.TelegramLoginFlow) (*VerifyResult, error) {
		if flow.Method != sqlcgen.TelegramLoginMethodPhone {
			return nil, ErrInvalidInput
		}
		phone, state, err := s.decryptFlow(flow)
		if err != nil || phone == "" {
			return nil, ErrLoginStateInvalid
		}
		step, err := s.login.VerifyCode(ctx, phone, state, code)
		if err != nil {
			return nil, err
		}
		if step.PasswordRequired {
			return s.persistPendingFlow(ctx, conn, flow, step)
		}
		return s.completeLogin(ctx, conn, flowID, step)
	})
}

func (s *Service) VerifyPassword(ctx context.Context, flowID uuid.UUID, password string) (*VerifyResult, error) {
	if flowID == uuid.Nil || password == "" {
		return nil, ErrInvalidInput
	}
	return s.withFlowLock(ctx, flowID, func(conn *pgxpool.Conn, flow *sqlcgen.TelegramLoginFlow) (*VerifyResult, error) {
		if !flow.PasswordRequired {
			return nil, ErrInvalidInput
		}
		_, state, err := s.decryptFlow(flow)
		if err != nil {
			return nil, err
		}
		step, err := s.login.VerifyPassword(ctx, state, password)
		if err != nil {
			return nil, err
		}
		return s.completeLogin(ctx, conn, flowID, step)
	})
}

func (s *Service) withFlowLock(ctx context.Context, flowID uuid.UUID, fn func(*pgxpool.Conn, *sqlcgen.TelegramLoginFlow) (*VerifyResult, error)) (result *VerifyResult, err error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire login flow connection: %w", err)
	}
	defer conn.Release()
	lockID := flowLockID(flowID)
	queries := sqlcgen.New(conn)
	if err := queries.AcquireAdvisoryLock(ctx, lockID); err != nil {
		return nil, fmt.Errorf("lock login flow: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, unlockErr := queries.ReleaseAdvisoryLock(unlockCtx, lockID)
		if unlockErr != nil && err == nil {
			err = fmt.Errorf("unlock login flow: %w", unlockErr)
		}
	}()
	flow, err := queries.GetTelegramLoginFlow(ctx, dbtypes.UUID(flowID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFlowNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get Telegram login flow: %w", err)
	}
	return fn(conn, flow)
}

func (s *Service) decryptFlow(flow *sqlcgen.TelegramLoginFlow) (string, []byte, error) {
	if flow == nil {
		return "", nil, ErrLoginStateInvalid
	}
	var phone string
	if flow.Method == sqlcgen.TelegramLoginMethodPhone {
		if len(flow.PhoneNumberCiphertext) == 0 {
			return "", nil, ErrLoginStateInvalid
		}
		plain, err := s.cipher.Open("login-phone", flow.PhoneNumberCiphertext)
		if err != nil {
			return "", nil, err
		}
		phone = string(plain)
	}
	state, err := s.cipher.Open("login-state", flow.TelegramStateCiphertext)
	if err != nil {
		return "", nil, err
	}
	return phone, state, nil
}

func (s *Service) persistPendingFlow(ctx context.Context, conn *pgxpool.Conn, flow *sqlcgen.TelegramLoginFlow, step LoginStep) (*VerifyResult, error) {
	ciphertext, err := s.cipher.Seal("login-state", step.State)
	if err != nil {
		return nil, err
	}
	updated, err := sqlcgen.New(conn).UpdateTelegramLoginFlowState(ctx, sqlcgen.UpdateTelegramLoginFlowStateParams{
		TelegramStateCiphertext: ciphertext, PasswordRequired: true, ID: flow.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFlowNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update Telegram login flow: %w", err)
	}
	id, _ := dbtypes.GoogleUUID(updated.ID)
	return &VerifyResult{Flow: &FlowResult{ID: id, ExpiresAt: updated.ExpiresAt.Time, PasswordRequired: true}}, nil
}

func (s *Service) completeLogin(ctx context.Context, conn *pgxpool.Conn, flowID uuid.UUID, step LoginStep) (*VerifyResult, error) {
	if step.User == nil || step.User.ID <= 0 || len(step.Session) == 0 {
		return nil, ErrLoginStateInvalid
	}
	if !s.userAllowed(step.User.Username) {
		return nil, ErrUserNotAllowed
	}
	telegramSession, err := s.cipher.Seal("telegram-session", step.Session)
	if err != nil {
		return nil, err
	}
	refreshToken, refreshHash, err := s.newOpaqueToken("tdr_")
	if err != nil {
		return nil, err
	}
	sessionID := uuid.New()
	expires := s.now().UTC().Add(s.config.RefreshTokenTTL)

	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin login transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	if _, err := q.UpsertUser(ctx, sqlcgen.UpsertUserParams{
		UserID: step.User.ID, DisplayName: dbtypes.OptionalText(nonEmpty(step.User.DisplayName)),
		Username: dbtypes.OptionalText(nonEmpty(step.User.Username)), Premium: step.User.Premium,
	}); err != nil {
		return nil, fmt.Errorf("upsert authenticated user: %w", err)
	}
	if _, err := q.CreateSession(ctx, sqlcgen.CreateSessionParams{
		ID: dbtypes.UUID(sessionID), UserID: step.User.ID,
		TelegramSession: telegramSession, RefreshTokenHash: refreshHash,
		ExpiresAt: dbtypes.Time(expires),
	}); err != nil {
		return nil, fmt.Errorf("create authenticated session: %w", err)
	}
	if _, err := q.CompleteTelegramLoginFlow(ctx, dbtypes.UUID(flowID)); err != nil {
		return nil, fmt.Errorf("complete Telegram login flow: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit authenticated session: %w", err)
	}
	access, err := s.issueAccessToken(step.User.ID, sessionID)
	if err != nil {
		return nil, err
	}
	return &VerifyResult{Tokens: &TokenPair{AccessToken: access, RefreshToken: refreshToken, ExpiresIn: ttlSeconds(s.config.AccessTokenTTL)}}, nil
}

func (s *Service) RenewAccess(ctx context.Context, refreshToken string) (*AccessRenewal, error) {
	refreshHash := hashToken(strings.TrimSpace(refreshToken))
	if len(refreshHash) == 0 {
		return nil, ErrInvalidCredential
	}
	sessionRow, err := s.queries.GetSessionByRefreshTokenHash(ctx, refreshHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredential
	}
	if err != nil {
		return nil, fmt.Errorf("get refresh session: %w", err)
	}
	sessionID, ok := dbtypes.GoogleUUID(sessionRow.ID)
	if !ok {
		return nil, ErrSessionNotFound
	}
	access, err := s.issueAccessToken(sessionRow.UserID, sessionID)
	if err != nil {
		return nil, err
	}
	_ = s.queries.TouchSession(ctx, sessionRow.ID)
	return &AccessRenewal{AccessToken: access, ExpiresIn: ttlSeconds(s.config.AccessTokenTTL)}, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	oldHash := hashToken(strings.TrimSpace(refreshToken))
	if len(oldHash) == 0 {
		return nil, ErrInvalidCredential
	}
	sessionRow, err := s.queries.GetSessionByRefreshTokenHash(ctx, oldHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredential
	}
	if err != nil {
		return nil, fmt.Errorf("get refresh session: %w", err)
	}
	sessionID, ok := dbtypes.GoogleUUID(sessionRow.ID)
	if !ok {
		return nil, ErrSessionNotFound
	}
	newToken, newHash, err := s.newOpaqueToken("tdr_")
	if err != nil {
		return nil, err
	}
	if _, err := s.queries.RotateSessionRefreshToken(ctx, sqlcgen.RotateSessionRefreshTokenParams{
		NewRefreshTokenHash: newHash, SessionID: sessionRow.ID, OldRefreshTokenHash: oldHash,
	}); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredential
	} else if err != nil {
		return nil, fmt.Errorf("rotate refresh token: %w", err)
	}
	access, err := s.issueAccessToken(sessionRow.UserID, sessionID)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: newToken, ExpiresIn: ttlSeconds(s.config.AccessTokenTTL)}, nil
}

func (s *Service) AuthenticateBearer(ctx context.Context, raw string) (principal.Identity, error) {
	claims := &accessClaims{}
	token, err := jwt.ParseWithClaims(strings.TrimSpace(raw), claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidCredential
		}
		return []byte(s.config.SigningKey), nil
	}, jwt.WithIssuer(s.config.Issuer), jwt.WithExpirationRequired())
	if err != nil || !token.Valid || claims.Subject == "" || claims.SessionID == uuid.Nil {
		return principal.Identity{}, ErrInvalidCredential
	}
	userID, err := parseUserID(claims.Subject)
	if err != nil {
		return principal.Identity{}, ErrInvalidCredential
	}
	if _, err := s.queries.GetActiveSession(ctx, sqlcgen.GetActiveSessionParams{SessionID: dbtypes.UUID(claims.SessionID), UserID: userID}); errors.Is(err, pgx.ErrNoRows) {
		return principal.Identity{}, ErrInvalidCredential
	} else if err != nil {
		return principal.Identity{}, err
	}
	_ = s.queries.TouchSession(ctx, dbtypes.UUID(claims.SessionID))
	return principal.Identity{UserID: userID, SessionID: claims.SessionID, Roles: append([]string(nil), claims.Roles...), Source: "bearer"}, nil
}

func (s *Service) AuthenticateAPIKey(ctx context.Context, raw string) (principal.Identity, error) {
	hash := hashToken(strings.TrimSpace(raw))
	if len(hash) == 0 {
		return principal.Identity{}, ErrInvalidCredential
	}
	row, err := s.queries.GetActiveAPIKeyByHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return principal.Identity{}, ErrInvalidCredential
	}
	if err != nil {
		return principal.Identity{}, err
	}
	_ = s.queries.TouchAPIKey(ctx, row.ID)
	return principal.Identity{UserID: row.UserID, Roles: []string{"user"}, Source: "api_key"}, nil
}

func (s *Service) GetUser(ctx context.Context, userID int64) (*sqlcgen.User, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	row, err := s.queries.GetUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return row, nil
}

func (s *Service) Logout(ctx context.Context, identity principal.Identity) error {
	if identity.UserID <= 0 || identity.SessionID == uuid.Nil || identity.Source != "bearer" {
		return ErrSessionNotFound
	}
	count, err := s.queries.RevokeSession(ctx, sqlcgen.RevokeSessionParams{SessionID: dbtypes.UUID(identity.SessionID), UserID: identity.UserID})
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if count == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (s *Service) ListSessions(ctx context.Context, in ListSessionsInput) ([]*sqlcgen.Session, error) {
	if in.UserID <= 0 {
		return nil, ErrInvalidInput
	}
	if in.Limit <= 0 {
		in.Limit = 100
	}
	if in.Limit > 200 {
		in.Limit = 200
	}
	rows, err := s.queries.ListSessions(ctx, sqlcgen.ListSessionsParams{
		UserID: in.UserID, AfterCreatedAt: dbtypes.OptionalTime(in.AfterCreatedAt),
		AfterID: dbtypes.OptionalUUID(in.AfterID), PageSize: in.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return rows, nil
}

func (s *Service) RevokeSession(ctx context.Context, userID int64, sessionID uuid.UUID) error {
	if userID <= 0 || sessionID == uuid.Nil {
		return ErrInvalidInput
	}
	count, err := s.queries.RevokeSession(ctx, sqlcgen.RevokeSessionParams{
		SessionID: dbtypes.UUID(sessionID), UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if count == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (s *Service) CreateAPIKey(ctx context.Context, userID int64, name string, expiresAt *time.Time) (*APIKeyCreated, error) {
	name = strings.TrimSpace(name)
	if userID <= 0 || name == "" || len(name) > 120 || (expiresAt != nil && !expiresAt.After(s.now())) {
		return nil, ErrInvalidInput
	}
	secret, hash, err := s.newOpaqueToken("tdk_")
	if err != nil {
		return nil, err
	}
	prefix := secret
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}
	row, err := s.queries.CreateAPIKey(ctx, sqlcgen.CreateAPIKeyParams{
		ID: dbtypes.UUID(uuid.New()), UserID: userID, Name: name, KeyPrefix: prefix,
		SecretHash: hash, ExpiresAt: dbtypes.OptionalTime(expiresAt),
	})
	if err != nil {
		return nil, fmt.Errorf("create API key: %w", err)
	}
	return &APIKeyCreated{Row: row, Secret: secret}, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, in ListAPIKeysInput) ([]*sqlcgen.ApiKey, error) {
	if in.UserID <= 0 {
		return nil, ErrInvalidInput
	}
	if in.Limit <= 0 {
		in.Limit = 100
	}
	if in.Limit > 200 {
		in.Limit = 200
	}
	rows, err := s.queries.ListAPIKeys(ctx, sqlcgen.ListAPIKeysParams{
		UserID: in.UserID, AfterCreatedAt: dbtypes.OptionalTime(in.AfterCreatedAt),
		AfterID: dbtypes.OptionalUUID(in.AfterID), PageSize: in.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list API keys: %w", err)
	}
	return rows, nil
}

func (s *Service) RevokeAPIKey(ctx context.Context, userID int64, keyID uuid.UUID) error {
	if userID <= 0 || keyID == uuid.Nil {
		return ErrInvalidInput
	}
	count, err := s.queries.RevokeAPIKey(ctx, sqlcgen.RevokeAPIKeyParams{ID: dbtypes.UUID(keyID), UserID: userID})
	if err != nil {
		return fmt.Errorf("revoke API key: %w", err)
	}
	if count == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}

func (s *Service) issueAccessToken(userID int64, sessionID uuid.UUID) (string, error) {
	now := s.now().UTC()
	claims := accessClaims{
		SessionID: sessionID, Roles: []string{"user"},
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: s.config.Issuer, Subject: fmt.Sprintf("%d", userID),
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(s.config.AccessTokenTTL)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.config.SigningKey))
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return token, nil
}

func (s *Service) newOpaqueToken(prefix string) (string, []byte, error) {
	buffer := make([]byte, 32)
	if _, err := io.ReadFull(s.random, buffer); err != nil {
		return "", nil, fmt.Errorf("generate opaque token: %w", err)
	}
	token := prefix + base64.RawURLEncoding.EncodeToString(buffer)
	return token, hashToken(token), nil
}

func hashToken(value string) []byte {
	if value == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func flowLockID(id uuid.UUID) int64 {
	digest := sha256.Sum256(append([]byte("teldrive/login-flow/"), id[:]...))
	return int64(binary.BigEndian.Uint64(digest[:8]))
}

func parseUserID(value string) (int64, error) {
	var result int64
	_, err := fmt.Sscan(value, &result)
	if err != nil || result <= 0 {
		return 0, ErrInvalidCredential
	}
	return result, nil
}

func ttlSeconds(ttl time.Duration) int32 {
	seconds := ttl / time.Second
	if seconds > time.Duration(^uint32(0)>>1) {
		return int32(^uint32(0) >> 1)
	}
	return int32(seconds)
}

func normalizeAllowedUsers(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		username := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "@")))
		if username == "" {
			continue
		}
		if _, exists := seen[username]; exists {
			continue
		}
		seen[username] = struct{}{}
		result = append(result, username)
	}
	return result
}

func (s *Service) userAllowed(username string) bool {
	if len(s.config.AllowedUsers) == 0 {
		return true
	}
	candidate := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(username), "@")))
	return slices.Contains(s.config.AllowedUsers, candidate)
}
func nonEmpty(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
