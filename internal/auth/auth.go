package auth

import (
	"context"
	"fmt"
	"strconv"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/pkg/models"
	"github.com/tgdrive/teldrive/pkg/types"
	"gorm.io/gorm"
)

type authContextKey string

const authKey authContextKey = "authUser"

func Encode(secret string, claims *types.JWTClaims) (string, error) {

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(secret))
}

func Decode(secret string, token string) (*types.JWTClaims, error) {
	claims := &types.JWTClaims{}

	tkn, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
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

func GetUser(c context.Context) int64 {
	authUser, _ := c.Value(authKey).(*types.JWTClaims)
	userId, _ := strconv.ParseInt(authUser.Subject, 10, 64)
	return userId
}

func GetJWTUser(c context.Context) *types.JWTClaims {
	authUser, _ := c.Value(authKey).(*types.JWTClaims)
	return authUser
}

func VerifyUser(db *gorm.DB, cache cache.Cacher, secret, authCookie string) (*types.JWTClaims, error) {
	claims, err := Decode(secret, authCookie)

	if err != nil {
		return nil, err
	}

	var session *models.Session

	session, err = GetSessionByHash(db, cache, claims.Hash)

	if err != nil {
		return nil, fmt.Errorf("invalid session")
	}

	claims.TgSession = session.Session

	return claims, nil
}

func GetSessionByHash(db *gorm.DB, cache cache.Cacher, hash string) (*models.Session, error) {
	var session models.Session
	key := fmt.Sprintf("sessions:%s", hash)

	err := cache.Get(key, &session)

	if err != nil {
		if err := db.Model(&models.Session{}).Where("hash = ?", hash).First(&session).Error; err != nil {
			return nil, err
		}
		cache.Set(key, &session, 0)
	}

	return &session, nil

}

type securityHandler struct {
	db    *gorm.DB
	cache cache.Cacher
	cfg   *config.JWTConfig
}

func (s *securityHandler) HandleApiKeyAuth(ctx context.Context, operationName api.OperationName, t api.ApiKeyAuth) (context.Context, error) {
	return s.handleAuth(ctx, t.APIKey)
}

func (s *securityHandler) HandleBearerAuth(ctx context.Context, operationName api.OperationName, t api.BearerAuth) (context.Context, error) {
	return s.handleAuth(ctx, t.Token)
}

func (s *securityHandler) handleAuth(ctx context.Context, token string) (context.Context, error) {
	claims, err := VerifyUser(s.db, s.cache, s.cfg.Secret, token)
	if err != nil {
		return nil, &ogenerrors.SecurityError{Err: err}
	}
	return context.WithValue(ctx, authKey, claims), nil
}

func NewSecurityHandler(db *gorm.DB, cache cache.Cacher, cfg *config.JWTConfig) api.SecurityHandler {
	return &securityHandler{db: db, cache: cache, cfg: cfg}
}

var _ api.SecurityHandler = (*securityHandler)(nil)
