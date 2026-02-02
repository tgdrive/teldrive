package integration

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/events"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/pkg/models"
	"github.com/tgdrive/teldrive/pkg/services"
	"github.com/tgdrive/teldrive/pkg/types"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	testUserID    = 123456789
	testUserName  = "testuser"
	testJWTSecret = "testsecret"
)

func createDummyUser(db *gorm.DB) error {
	user := models.User{
		UserId:    testUserID,
		Name:      "Test User",
		UserName:  testUserName,
		IsPremium: false,
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&user).Error
}

func createSession(db *gorm.DB) (string, error) {
	session := "1AgAAAAAAAAAAAA..." // Dummy session string
	tokenhash := md5.Sum([]byte(session))
	hexToken := hex.EncodeToString(tokenhash[:])

	// Create DB Session
	s := models.Session{
		UserId:      testUserID,
		Session:     session,
		Hash:        hexToken,
		SessionDate: int(time.Now().Unix()),
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&s).Error; err != nil {
		return "", err
	}

	// Generate JWT
	now := time.Now().UTC()
	claims := &types.JWTClaims{
		Name:      "Test User",
		UserName:  testUserName,
		IsPremium: false,
		Hash:      hexToken,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(testUserID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	}
	return auth.Encode(testJWTSecret, claims)
}

func newTestApiService(db *gorm.DB) api.Handler {
	cnf := &config.ServerCmdConfig{
		JWT: config.JWTConfig{
			Secret:       testJWTSecret,
			SessionTime:  24 * time.Hour,
			AllowedUsers: []string{testUserName},
		},
		TG: config.TGConfig{
			Uploads: config.TGUpload{
				Retention: 7 * 24 * time.Hour,
			},
		},
	}
	c := cache.NewCache(context.Background(), config.CacheConfig{}.MaxSize, nil,nil)
	botSelector := tgc.NewBotSelector(nil)
	ev := events.NewBroadcaster(context.Background(), db, nil, time.Duration(10*time.Second), events.BroadcasterConfig{}, zap.NewNop())
	return services.NewApiService(db, cnf, c, botSelector, ev)
}
