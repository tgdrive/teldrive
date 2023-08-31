package types

import (
	"github.com/go-jose/go-jose/v3/jwt"
	"github.com/gotd/td/tg"
)

type AppError struct {
	Error error
	Code  int
}

type Part struct {
	Location *tg.InputDocumentFileLocation
	Size     int64
	Start    int64
	End      int64
}

type JWTClaims struct {
	jwt.Claims
	TgSession string `json:"tgSession"`
	UserID    int64  `json:"userId"`
	Name      string `json:"name"`
	UserName  string `json:"userName"`
	Bot       bool   `json:"bot"`
	IsPremium bool   `json:"isPremium"`
}

type TgSession struct {
	Sesssion  string `json:"session"`
	UserID    int64  `json:"userId"`
	Bot       bool   `json:"bot"`
	UserName  string `json:"userName"`
	Name      string `json:"name"`
	IsPremium bool   `json:"isPremium"`
}

type Session struct {
	UserID    int64  `json:"userId"`
	Name      string `json:"name"`
	UserName  string `json:"userName"`
	IsPremium bool   `json:"isPremium"`
	Expires   string `json:"expires"`
}
