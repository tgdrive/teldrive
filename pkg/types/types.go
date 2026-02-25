package types

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/gotd/td/session"
)

type Part struct {
	DecryptedSize int64
	Size          int64
	Salt          string
	ID            int64
}

type JWTClaims struct {
	jwt.RegisteredClaims
	Name      string `json:"name"`
	UserName  string `json:"userName"`
	IsPremium bool   `json:"isPremium"`
	SessionID string `json:"sessionId"`
	TgSession string `json:"tgSession,omitempty"`
}

type SessionData struct {
	Version int
	Data    session.Data
}

type BotInfo struct {
	ID         int64
	UserName   string
	AccessHash int64
	Token      string
}
