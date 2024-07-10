package auth

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/divyam234/teldrive/internal/cache"
	"github.com/divyam234/teldrive/pkg/models"
	"github.com/divyam234/teldrive/pkg/types"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

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

func GetUser(c *gin.Context) (int64, string) {
	val, _ := c.Get("jwtUser")
	jwtUser := val.(*types.JWTClaims)
	userId, _ := strconv.ParseInt(jwtUser.Subject, 10, 64)
	return userId, jwtUser.TgSession
}

func VerifyUser(c *gin.Context, db *gorm.DB, cache *cache.Cache, secret string) (*types.JWTClaims, error) {
	var token string
	cookie, err := c.Request.Cookie("user-session")

	if err != nil {
		authHeader := c.GetHeader("Authorization")
		bearerToken := strings.Split(authHeader, "Bearer ")
		if len(bearerToken) != 2 {
			return nil, fmt.Errorf("missing auth token")
		}
		token = bearerToken[1]
	} else {
		token = cookie.Value
	}

	claims, err := Decode(secret, token)

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

func GetSessionByHash(db *gorm.DB, cache *cache.Cache, hash string) (*models.Session, error) {
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
