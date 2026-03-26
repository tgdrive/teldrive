package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"github.com/tgdrive/teldrive/pkg/types"
)

type authContextKey string

const (
	authKey       authContextKey = "authUser"
	authSourceKey authContextKey = "authSource"
)

type AuthSource string

const (
	AuthSourceUnknown AuthSource = ""
	AuthSourceCookie  AuthSource = "cookie"
	AuthSourceBearer  AuthSource = "bearer"
	AuthSourceAPIKey  AuthSource = "api_key"
	AuthSourceSession AuthSource = "session_hash"
)

func Encode(secret string, claims *types.JWTClaims) (string, error) {

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(secret))
}

func Decode(secret string, token string) (*types.JWTClaims, error) {
	claims := &types.JWTClaims{}

	tkn, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Method.Alg())
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if !tkn.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, err
}

func User(c context.Context) int64 {
	authUser, ok := c.Value(authKey).(*types.JWTClaims)
	if !ok || authUser == nil {
		return 0
	}
	userId, err := strconv.ParseInt(authUser.Subject, 10, 64)
	if err != nil {
		return 0
	}
	return userId
}

func JWTUser(c context.Context) *types.JWTClaims {
	authUser, ok := c.Value(authKey).(*types.JWTClaims)
	if !ok {
		return nil
	}
	return authUser
}

func WithJWTUser(ctx context.Context, claims *types.JWTClaims) context.Context {
	return context.WithValue(ctx, authKey, claims)
}

func WithAuthSource(ctx context.Context, source AuthSource) context.Context {
	return context.WithValue(ctx, authSourceKey, source)
}

func Source(ctx context.Context) AuthSource {
	source, ok := ctx.Value(authSourceKey).(AuthSource)
	if !ok {
		return AuthSourceUnknown
	}
	return source
}

func WithUser(ctx context.Context, userID int64, tgSession string) context.Context {
	claims := &types.JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: strconv.FormatInt(userID, 10)},
		TgSession:        tgSession,
	}
	return WithJWTUser(ctx, claims)
}

func VerifyUser(ctx context.Context, sessions repositories.SessionRepository, cache cache.Cacher, secret, authCookie string) (*types.JWTClaims, error) {
	claims, err := Decode(secret, authCookie)

	if err != nil {
		return nil, err
	}

	var session *jetmodel.Sessions

	session, err = SessionByID(ctx, sessions, cache, claims.SessionID)

	if err != nil {
		return nil, fmt.Errorf("invalid session")
	}

	claims.TgSession = session.TgSession

	return claims, nil
}

func SessionByID(ctx context.Context, sessions repositories.SessionRepository, cache cache.Cacher, id uuid.UUID) (*jetmodel.Sessions, error) {
	var session jetmodel.Sessions
	key := fmt.Sprintf("sessions:%s", id)

	err := cache.Get(ctx, key, &session)

	if err != nil {
		fetched, err := sessions.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		session = *fetched
		cache.Set(ctx, key, &session, 0)
	}

	return &session, nil

}

type securityHandler struct {
	sessions repositories.SessionRepository
	apiKeys  repositories.APIKeyRepository
	cache    cache.Cacher
	cfg      *config.JWTConfig
}

type cachedAPIKeyAuth struct {
	SessionID string
	TgSession string
	UserID    int64
}

func (s *securityHandler) HandleAccessTokenCookieAuth(ctx context.Context, operationName api.OperationName, t api.AccessTokenCookieAuth) (context.Context, error) {
	return s.handleJWTAuth(ctx, t.APIKey, AuthSourceCookie)
}

func (s *securityHandler) HandleXApiKeyHeaderAuth(ctx context.Context, operationName api.OperationName, t api.XApiKeyHeaderAuth) (context.Context, error) {
	return s.handleAPIKeyAuth(ctx, t.APIKey)
}

func (s *securityHandler) HandleBearerAuth(ctx context.Context, operationName api.OperationName, t api.BearerAuth) (context.Context, error) {
	return s.handleJWTAuth(ctx, t.Token, AuthSourceBearer)
}

func (s *securityHandler) HandleSessionHashAuth(ctx context.Context, operationName api.OperationName, t api.SessionHashAuth) (context.Context, error) {
	if operationName != api.FilesStreamOperation {
		return nil, &ogenerrors.SecurityError{Err: ErrAuthSessionInvalid}
	}
	session, err := SessionByID(ctx, s.sessions, s.cache, uuid.MustParse(t.APIKey))
	if err != nil {
		return nil, &ogenerrors.SecurityError{Err: ErrAuthSessionInvalid}
	}
	claims := &types.JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: strconv.FormatInt(session.UserID, 10)},
		SessionID:        session.ID,
		TgSession:        session.TgSession,
	}
	ctx = context.WithValue(ctx, authKey, claims)
	ctx = context.WithValue(ctx, authSourceKey, AuthSourceSession)
	return ctx, nil
}

func (s *securityHandler) handleJWTAuth(ctx context.Context, token string, source AuthSource) (context.Context, error) {
	claims, err := VerifyUser(ctx, s.sessions, s.cache, s.cfg.Secret, token)
	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			return nil, &ogenerrors.SecurityError{Err: ErrAuthTokenExpired}
		case errors.Is(err, jwt.ErrTokenNotValidYet):
			return nil, &ogenerrors.SecurityError{Err: ErrAuthTokenInvalid}
		case errors.Is(err, jwt.ErrTokenMalformed), errors.Is(err, jwt.ErrTokenUnverifiable), errors.Is(err, jwt.ErrTokenSignatureInvalid):
			return nil, &ogenerrors.SecurityError{Err: ErrAuthTokenInvalid}
		case errors.Is(err, repositories.ErrNotFound):
			return nil, &ogenerrors.SecurityError{Err: ErrAuthSessionInvalid}
		default:
			if err.Error() == "invalid session" {
				return nil, &ogenerrors.SecurityError{Err: ErrAuthSessionInvalid}
			}
			return nil, &ogenerrors.SecurityError{Err: ErrAuthTokenInvalid}
		}
	}
	ctx = context.WithValue(ctx, authKey, claims)
	ctx = context.WithValue(ctx, authSourceKey, source)
	return ctx, nil
}

func (s *securityHandler) handleAPIKeyAuth(ctx context.Context, token string) (context.Context, error) {
	claims, err := s.verifyAPIKey(ctx, token)
	if err != nil {
		return nil, &ogenerrors.SecurityError{Err: err}
	}

	ctx = context.WithValue(ctx, authKey, claims)
	ctx = context.WithValue(ctx, authSourceKey, AuthSourceAPIKey)
	return ctx, nil
}

func (s *securityHandler) verifyAPIKey(ctx context.Context, token string) (*types.JWTClaims, error) {
	tokenHash := hashToken(token)
	cacheKey := cache.KeyAPIKeyAuth(tokenHash)

	cached, err := cache.Fetch(ctx, s.cache, cacheKey, 24*time.Hour, func() (cachedAPIKeyAuth, error) {
		key, err := s.apiKeys.GetActiveByTokenHash(ctx, tokenHash, time.Now().UTC())
		if err != nil {
			return cachedAPIKeyAuth{}, ErrAuthAPIKeyInvalid
		}

		sessions, err := s.sessions.GetByUserID(ctx, key.UserID)
		if err != nil {
			return cachedAPIKeyAuth{}, ErrAuthAPIKeySessionMiss
		}
		if len(sessions) == 0 {
			return cachedAPIKeyAuth{}, ErrAuthAPIKeySessionMiss
		}

		_ = s.apiKeys.TouchLastUsed(ctx, key.ID, time.Now().UTC())

		return cachedAPIKeyAuth{
			UserID:    key.UserID,
			SessionID: sessions[0].ID.String(),
			TgSession: sessions[0].TgSession,
		}, nil
	})
	if err != nil {
		return nil, err
	}

	claims := &types.JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: strconv.FormatInt(cached.UserID, 10)},
		SessionID:        uuid.MustParse(cached.SessionID),
		TgSession:        cached.TgSession,
	}

	return claims, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func NewSecurityHandler(sessions repositories.SessionRepository, apiKeys repositories.APIKeyRepository, cache cache.Cacher, cfg *config.JWTConfig) api.SecurityHandler {
	return &securityHandler{sessions: sessions, apiKeys: apiKeys, cache: cache, cfg: cfg}
}

var _ api.SecurityHandler = (*securityHandler)(nil)
