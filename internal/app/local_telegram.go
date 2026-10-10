package app

import (
	"context"
	"errors"

	"github.com/tgdrive/teldrive/v2/internal/authn"
	"github.com/tgdrive/teldrive/v2/internal/bots"
)

var ErrLocalTelegramLoginUnavailable = errors.New("Telegram login is unavailable with the filesystem backend")

type localTelegramLogin struct{}

func (localTelegramLogin) Start(context.Context, string) (authn.LoginStep, error) {
	return authn.LoginStep{}, ErrLocalTelegramLoginUnavailable
}

func (localTelegramLogin) StartQR(context.Context) (authn.LoginStep, error) {
	return authn.LoginStep{}, ErrLocalTelegramLoginUnavailable
}

func (localTelegramLogin) PollQR(context.Context, []byte) (authn.LoginStep, error) {
	return authn.LoginStep{}, ErrLocalTelegramLoginUnavailable
}

func (localTelegramLogin) VerifyCode(context.Context, string, []byte, string) (authn.LoginStep, error) {
	return authn.LoginStep{}, ErrLocalTelegramLoginUnavailable
}

func (localTelegramLogin) VerifyPassword(context.Context, []byte, string) (authn.LoginStep, error) {
	return authn.LoginStep{}, ErrLocalTelegramLoginUnavailable
}

type localBotVerifier struct{}

func (localBotVerifier) Verify(context.Context, string) (bots.Identity, error) {
	return bots.Identity{}, bots.ErrNotBot
}

var _ authn.TelegramLogin = localTelegramLogin{}
var _ bots.Verifier = localBotVerifier{}
