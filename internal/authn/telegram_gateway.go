package authn

import (
	"context"
	"errors"
	"time"
)

var (
	ErrCodeInvalid       = errors.New("Telegram login code is invalid")
	ErrPasswordRequired  = errors.New("Telegram two-step password is required")
	ErrPasswordInvalid   = errors.New("Telegram two-step password is invalid")
	ErrLoginStateInvalid = errors.New("Telegram login state is invalid")
)

type TelegramUser struct {
	ID          int64
	DisplayName string
	Username    string
	Premium     bool
}

type LoginStep struct {
	State            []byte
	User             *TelegramUser
	Session          []byte
	PasswordRequired bool
	QRURL            string
	QRExpiresAt      time.Time
}

type TelegramLogin interface {
	Start(context.Context, string) (LoginStep, error)
	StartQR(context.Context) (LoginStep, error)
	PollQR(context.Context, []byte) (LoginStep, error)
	VerifyCode(context.Context, string, []byte, string) (LoginStep, error)
	VerifyPassword(context.Context, []byte, string) (LoginStep, error)
}
