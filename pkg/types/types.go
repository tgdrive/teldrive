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
	Start    int64
	End      int64
}

type JWTClaims struct {
	jwt.Claims
	TgSession string `json:"tgSession"`
	Name      string `json:"name"`
	UserName  string `json:"userName"`
	Bot       bool   `json:"bot"`
	IsPremium bool   `json:"isPremium"`
	Hash      string `json:"hash"`
}