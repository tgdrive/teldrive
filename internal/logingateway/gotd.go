package logingateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/tgdrive/teldrive/v2/internal/authn"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	"github.com/tgdrive/teldrive/v2/internal/telethonsession"
)

type loginState struct {
	Session       string    `json:"session"`
	PhoneCodeHash string    `json:"phone_code_hash,omitempty"`
	QRURL         string    `json:"qr_url,omitempty"`
	QRExpiresAt   time.Time `json:"qr_expires_at,omitempty"`
}

type qrExportResult struct {
	User             *tg.User
	URL              string
	ExpiresAt        time.Time
	PasswordRequired bool
}

type GotdTelegramLogin struct {
	factory *telegramstore.Factory
}

func New(factory *telegramstore.Factory) (*GotdTelegramLogin, error) {
	if factory == nil {
		return nil, telegramstore.ErrTelegramConfiguration
	}
	return &GotdTelegramLogin{factory: factory}, nil
}

func (g *GotdTelegramLogin) Start(ctx context.Context, phone string) (authn.LoginStep, error) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return authn.LoginStep{}, authn.ErrLoginStateInvalid
	}
	memory := &session.StorageMemory{}
	client, err := g.factory.New(memory)
	if err != nil {
		return authn.LoginStep{}, err
	}
	var codeHash string
	if err := client.Run(ctx, func(runCtx context.Context) error {
		sent, err := client.Auth().SendCode(runCtx, phone, auth.SendCodeOptions{})
		if err != nil {
			return err
		}
		switch value := sent.(type) {
		case *tg.AuthSentCode:
			codeHash = value.PhoneCodeHash
		case *tg.AuthSentCodeSuccess:
			return errors.New("Telegram session is already authorized")
		case *tg.AuthSentCodePaymentRequired:
			return errors.New("Telegram requires payment before sending a login code")
		default:
			return fmt.Errorf("unexpected Telegram send-code response %T", sent)
		}
		return nil
	}); err != nil {
		return authn.LoginStep{}, fmt.Errorf("send Telegram login code: %w", err)
	}
	state, err := encodeLoginState(memory, codeHash)
	if err != nil {
		return authn.LoginStep{}, err
	}
	return authn.LoginStep{State: state}, nil
}

func (g *GotdTelegramLogin) StartQR(ctx context.Context) (authn.LoginStep, error) {
	memory := &session.StorageMemory{}
	client, err := g.factory.New(memory)
	if err != nil {
		return authn.LoginStep{}, err
	}
	var result qrExportResult
	if err := client.Run(ctx, func(runCtx context.Context) error {
		var exportErr error
		result, exportErr = g.exportQR(runCtx, client)
		return exportErr
	}); err != nil {
		return authn.LoginStep{}, fmt.Errorf("start Telegram QR login: %w", err)
	}
	if result.User != nil {
		return completeLoginStep(memory, result.User)
	}
	state, err := encodeState(memory, loginState{QRURL: result.URL, QRExpiresAt: result.ExpiresAt})
	if err != nil {
		return authn.LoginStep{}, err
	}
	return authn.LoginStep{
		State: state, PasswordRequired: result.PasswordRequired,
		QRURL: result.URL, QRExpiresAt: result.ExpiresAt,
	}, nil
}

func (g *GotdTelegramLogin) PollQR(ctx context.Context, encodedState []byte) (authn.LoginStep, error) {
	state, memory, err := decodeLoginState(encodedState)
	if err != nil || state.PhoneCodeHash != "" {
		return authn.LoginStep{}, authn.ErrLoginStateInvalid
	}
	client, err := g.factory.New(memory)
	if err != nil {
		return authn.LoginStep{}, err
	}
	var result qrExportResult
	if err := client.Run(ctx, func(runCtx context.Context) error {
		var exportErr error
		result, exportErr = g.exportQR(runCtx, client)
		return exportErr
	}); err != nil {
		return authn.LoginStep{}, fmt.Errorf("poll Telegram QR login: %w", err)
	}
	if result.User != nil {
		return completeLoginStep(memory, result.User)
	}
	if result.URL != "" {
		state.QRURL = result.URL
		state.QRExpiresAt = result.ExpiresAt
	}
	next, err := encodeState(memory, state)
	if err != nil {
		return authn.LoginStep{}, err
	}
	return authn.LoginStep{
		State: next, PasswordRequired: result.PasswordRequired,
		QRURL: state.QRURL, QRExpiresAt: state.QRExpiresAt,
	}, nil
}

func (g *GotdTelegramLogin) exportQR(ctx context.Context, client *telegram.Client) (qrExportResult, error) {
	appID, appHash, ok := g.factory.AppCredentials()
	if !ok || client == nil {
		return qrExportResult{}, telegramstore.ErrTelegramConfiguration
	}
	result, err := client.API().AuthExportLoginToken(ctx, &tg.AuthExportLoginTokenRequest{
		APIID: appID, APIHash: appHash,
	})
	if tgerr.Is(err, "SESSION_PASSWORD_NEEDED") || errors.Is(err, auth.ErrPasswordAuthNeeded) {
		return qrExportResult{PasswordRequired: true}, nil
	}
	if err != nil {
		return qrExportResult{}, err
	}
	switch value := result.(type) {
	case *tg.AuthLoginToken:
		token := qrlogin.NewToken(value.Token, value.Expires)
		return qrExportResult{URL: token.URL(), ExpiresAt: token.Expires()}, nil
	case *tg.AuthLoginTokenSuccess:
		user, err := authorizationUser(value.Authorization)
		if err != nil {
			return qrExportResult{}, err
		}
		return qrExportResult{User: user}, nil
	case *tg.AuthLoginTokenMigrateTo:
		imported, err := importMigratedQRToken(
			ctx,
			value.DCID,
			value.Token,
			client.MigrateTo,
			client.API().AuthImportLoginToken,
		)
		if tgerr.Is(err, "SESSION_PASSWORD_NEEDED") || errors.Is(err, auth.ErrPasswordAuthNeeded) {
			return qrExportResult{PasswordRequired: true}, nil
		}
		if err != nil {
			return qrExportResult{}, fmt.Errorf("migrate Telegram QR login to DC %d: %w", value.DCID, err)
		}
		success, ok := imported.(*tg.AuthLoginTokenSuccess)
		if !ok {
			return qrExportResult{}, fmt.Errorf("unexpected Telegram QR import response %T", imported)
		}
		user, err := authorizationUser(success.Authorization)
		if err != nil {
			return qrExportResult{}, err
		}
		return qrExportResult{User: user}, nil
	default:
		return qrExportResult{}, fmt.Errorf("unexpected Telegram QR export response %T", result)
	}
}

func importMigratedQRToken(
	ctx context.Context,
	dcID int,
	token []byte,
	migrate func(context.Context, int) error,
	importToken func(context.Context, []byte) (tg.AuthLoginTokenClass, error),
) (tg.AuthLoginTokenClass, error) {
	if dcID <= 0 || len(token) == 0 || migrate == nil || importToken == nil {
		return nil, authn.ErrLoginStateInvalid
	}
	if err := migrate(ctx, dcID); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	result, err := importToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}
	return result, nil
}
func authorizationUser(value tg.AuthAuthorizationClass) (*tg.User, error) {
	authorization, ok := value.(*tg.AuthAuthorization)
	if !ok {
		return nil, authn.ErrLoginStateInvalid
	}
	user, ok := authorization.User.AsNotEmpty()
	if !ok || user.ID <= 0 {
		return nil, authn.ErrLoginStateInvalid
	}
	return user, nil
}

func (g *GotdTelegramLogin) VerifyCode(ctx context.Context, phone string, encodedState []byte, code string) (authn.LoginStep, error) {
	state, memory, err := decodeLoginState(encodedState)
	if err != nil || state.PhoneCodeHash == "" {
		return authn.LoginStep{}, authn.ErrLoginStateInvalid
	}
	client, err := g.factory.New(memory)
	if err != nil {
		return authn.LoginStep{}, err
	}
	var user *tg.User
	err = client.Run(ctx, func(runCtx context.Context) error {
		if _, err := client.Auth().SignIn(runCtx, phone, strings.TrimSpace(code), state.PhoneCodeHash); err != nil {
			if errors.Is(err, auth.ErrPasswordAuthNeeded) {
				return authn.ErrPasswordRequired
			}
			if strings.Contains(err.Error(), "PHONE_CODE_INVALID") || strings.Contains(err.Error(), "PHONE_CODE_EXPIRED") {
				return authn.ErrCodeInvalid
			}
			return err
		}
		user, err = client.Self(runCtx)
		return err
	})
	if errors.Is(err, authn.ErrPasswordRequired) {
		next, encodeErr := encodeLoginState(memory, state.PhoneCodeHash)
		if encodeErr != nil {
			return authn.LoginStep{}, encodeErr
		}
		return authn.LoginStep{State: next, PasswordRequired: true}, nil
	}
	if err != nil {
		return authn.LoginStep{}, fmt.Errorf("verify Telegram login code: %w", err)
	}
	return completeLoginStep(memory, user)
}

func (g *GotdTelegramLogin) VerifyPassword(ctx context.Context, encodedState []byte, password string) (authn.LoginStep, error) {
	_, memory, err := decodeLoginState(encodedState)
	if err != nil {
		return authn.LoginStep{}, err
	}
	client, err := g.factory.New(memory)
	if err != nil {
		return authn.LoginStep{}, err
	}
	var user *tg.User
	err = client.Run(ctx, func(runCtx context.Context) error {
		if _, err := client.Auth().Password(runCtx, password); err != nil {
			if strings.Contains(err.Error(), "PASSWORD_HASH_INVALID") {
				return authn.ErrPasswordInvalid
			}
			return err
		}
		user, err = client.Self(runCtx)
		return err
	})
	if err != nil {
		return authn.LoginStep{}, fmt.Errorf("verify Telegram password: %w", err)
	}
	return completeLoginStep(memory, user)
}

func encodeLoginState(memory *session.StorageMemory, phoneCodeHash string) ([]byte, error) {
	return encodeState(memory, loginState{PhoneCodeHash: phoneCodeHash})
}

func encodeState(memory *session.StorageMemory, state loginState) ([]byte, error) {
	data, err := memory.Bytes(nil)
	if err != nil {
		return nil, fmt.Errorf("serialize Telegram login session: %w", err)
	}
	state.Session = base64.RawURLEncoding.EncodeToString(data)
	encoded, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("serialize Telegram login state: %w", err)
	}
	return encoded, nil
}

func decodeLoginState(encoded []byte) (loginState, *session.StorageMemory, error) {
	var state loginState
	if err := json.Unmarshal(encoded, &state); err != nil || state.Session == "" {
		return loginState{}, nil, authn.ErrLoginStateInvalid
	}
	data, err := base64.RawURLEncoding.DecodeString(state.Session)
	if err != nil {
		return loginState{}, nil, authn.ErrLoginStateInvalid
	}
	memory := &session.StorageMemory{}
	if err := memory.StoreSession(context.Background(), data); err != nil {
		return loginState{}, nil, authn.ErrLoginStateInvalid
	}
	return state, memory, nil
}

func completeLoginStep(memory *session.StorageMemory, user *tg.User) (authn.LoginStep, error) {
	if user == nil || user.ID <= 0 {
		return authn.LoginStep{}, authn.ErrLoginStateInvalid
	}
	raw, err := memory.Bytes(nil)
	if err != nil {
		return authn.LoginStep{}, fmt.Errorf("serialize authorized Telegram session: %w", err)
	}
	encoded, err := telethonsession.EncodeGotd(context.Background(), raw)
	if err != nil {
		return authn.LoginStep{}, fmt.Errorf("encode authorized Telegram session as Telethon: %w", err)
	}
	displayName := strings.TrimSpace(strings.TrimSpace(user.FirstName) + " " + strings.TrimSpace(user.LastName))
	return authn.LoginStep{
		User:    &authn.TelegramUser{ID: user.ID, DisplayName: displayName, Username: user.Username, Premium: user.Premium},
		Session: []byte(encoded),
	}, nil
}

var _ authn.TelegramLogin = (*GotdTelegramLogin)(nil)
