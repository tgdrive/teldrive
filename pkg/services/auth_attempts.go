package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/pkg/types"
	"go.uber.org/zap"
)

const authAttemptTTL = 10 * time.Minute

type authAttemptManager struct {
	mu       sync.RWMutex
	attempts map[string]*authAttempt
}

type authAttempt struct {
	id             string
	authType       api.AuthAttemptAuthType
	state          api.AuthAttemptState
	commands       chan authAttemptCommand
	subscribers    map[chan []byte]struct{}
	latest         []byte
	token          string
	phoneCodeHash  string
	session        *api.Session
	message        string
	mu             sync.RWMutex
	cancel         context.CancelFunc
	done           chan struct{}
	closeOnce      sync.Once
	sessionStorage *session.StorageMemory
	tgClient       TelegramClient
	loggedIn       qrlogin.LoggedIn
}

type authAttemptCommand struct {
	run func(ctx context.Context) error
}

type authAttemptEvent struct {
	Type          string             `json:"type"`
	Token         string             `json:"token,omitempty"`
	PhoneCodeHash string             `json:"phoneCodeHash,omitempty"`
	Session       *api.SessionCreate `json:"session,omitempty"`
	Message       string             `json:"message,omitempty"`
}

func newAuthAttemptManager() *authAttemptManager {
	return &authAttemptManager{attempts: make(map[string]*authAttempt)}
}

func (m *authAttemptManager) set(attempt *authAttempt) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts[attempt.id] = attempt
}

func (m *authAttemptManager) get(id string) (*authAttempt, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	attempt, ok := m.attempts[id]
	return attempt, ok
}

func (m *authAttemptManager) delete(id string) {
	m.mu.Lock()
	attempt, ok := m.attempts[id]
	if ok {
		delete(m.attempts, id)
	}
	m.mu.Unlock()
	if ok {
		attempt.close()
	}
}

func newAuthAttempt(id string, authType api.AuthAttemptAuthType, cancel context.CancelFunc, tgClient TelegramClient, loggedIn qrlogin.LoggedIn, storage *session.StorageMemory) *authAttempt {
	return &authAttempt{
		id:             id,
		authType:       authType,
		state:          api.AuthAttemptStateCreated,
		commands:       make(chan authAttemptCommand, 8),
		subscribers:    make(map[chan []byte]struct{}),
		cancel:         cancel,
		done:           make(chan struct{}),
		sessionStorage: storage,
		tgClient:       tgClient,
		loggedIn:       loggedIn,
	}
}

func (a *authAttempt) snapshot() *api.AuthAttempt {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := &api.AuthAttempt{ID: a.id, AuthType: a.authType, State: a.state}
	if a.token != "" {
		out.Token.SetTo(a.token)
	}
	if a.phoneCodeHash != "" {
		out.PhoneCodeHash.SetTo(a.phoneCodeHash)
	}
	if a.session != nil {
		out.Session.SetTo(*a.session)
	}
	if a.message != "" {
		out.Message.SetTo(a.message)
	}
	return out
}

func (a *authAttempt) enqueue(cmd authAttemptCommand) error {
	select {
	case <-a.done:
		return errors.New("auth attempt closed")
	case a.commands <- cmd:
		return nil
	}
}

func (a *authAttempt) subscribe() (chan []byte, []byte) {
	ch := make(chan []byte, 8)
	a.mu.Lock()
	a.subscribers[ch] = struct{}{}
	latest := append([]byte(nil), a.latest...)
	a.mu.Unlock()
	return ch, latest
}

func (a *authAttempt) unsubscribe(ch chan []byte) {
	a.mu.Lock()
	if _, ok := a.subscribers[ch]; ok {
		delete(a.subscribers, ch)
		close(ch)
	}
	a.mu.Unlock()
}

func (a *authAttempt) publish(event authAttemptEvent) {
	a.mu.Lock()
	switch event.Type {
	case "token":
		a.state = api.AuthAttemptStateQrPending
		a.token = event.Token
		a.message = event.Message
	case "phone_code":
		a.state = api.AuthAttemptStateCodeSent
		a.phoneCodeHash = event.PhoneCodeHash
		a.message = event.Message
	case "password_required":
		a.state = api.AuthAttemptStatePasswordRequired
		a.message = event.Message
	case "success":
		a.state = api.AuthAttemptStateAuthenticated
		if event.Session != nil {
			session := api.Session{
				Name:      event.Session.Name,
				UserName:  event.Session.UserName,
				UserId:    event.Session.UserId,
				IsPremium: event.Session.IsPremium,
				SessionId: event.Session.SessionId,
				Hash:      event.Session.Hash,
				Expires:   event.Session.Expires,
			}
			a.session = &session
		}
		a.message = event.Message
	case "error":
		a.state = api.AuthAttemptStateFailed
		a.message = event.Message
	}
	a.mu.Unlock()

	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	a.mu.Lock()
	a.latest = payload
	for ch := range a.subscribers {
		select {
		case ch <- payload:
		default:
		}
	}
	a.mu.Unlock()
}

func (a *authAttempt) close() {
	a.closeOnce.Do(func() {
		a.cancel()
		close(a.done)
		a.mu.Lock()
		for ch := range a.subscribers {
			close(ch)
		}
		a.subscribers = map[chan []byte]struct{}{}
		a.mu.Unlock()
	})
}

func (a *apiService) AuthCreateAttempt(ctx context.Context, req *api.AuthAttemptCreate) (*api.AuthAttempt, error) {
	attemptID := uuid.NewString()
	attemptCtx, cancel := context.WithTimeout(context.Background(), authAttemptTTL)
	dispatcher, loggedIn := a.telegram.NewQRLogin()
	storage := &session.StorageMemory{}
	tgClient, err := a.telegram.NoAuthClient(attemptCtx, dispatcher, storage)
	if err != nil {
		cancel()
		return nil, &apiError{err: err}
	}
	attempt := newAuthAttempt(attemptID, api.AuthAttemptAuthType(req.AuthType), cancel, tgClient, loggedIn, storage)
	a.authAttempts.set(attempt)
	go a.runAuthAttempt(attemptCtx, attempt)
	if req.AuthType == api.AuthAttemptCreateAuthTypeQr {
		if err := attempt.enqueue(authAttemptCommand{run: func(runCtx context.Context) error {
			return a.executeQRLogin(runCtx, attempt)
		}}); err != nil {
			a.authAttempts.delete(attemptID)
			return nil, &apiError{err: err, code: 409}
		}
	} else if phoneNo, ok := req.PhoneNo.Get(); ok && phoneNo != "" {
		if err := attempt.enqueue(authAttemptCommand{run: func(runCtx context.Context) error {
			hash, err := attempt.tgClient.SendCode(runCtx, phoneNo)
			if errors.Is(err, context.Canceled) {
				return nil
			}
			if err != nil {
				attempt.publish(authAttemptEvent{Type: "error", Message: err.Error()})
				return nil
			}
			attempt.publish(authAttemptEvent{Type: "phone_code", PhoneCodeHash: hash})
			return nil
		}}); err != nil {
			a.authAttempts.delete(attemptID)
			return nil, &apiError{err: err, code: 409}
		}
	}
	return attempt.snapshot(), nil
}

func (a *apiService) AuthGetAttempt(ctx context.Context, params api.AuthGetAttemptParams) (*api.AuthAttempt, error) {
	attempt, err := a.getAuthAttempt(params.ID)
	if err != nil {
		return nil, err
	}
	return attempt.snapshot(), nil
}

func (a *apiService) AuthDeleteAttempt(ctx context.Context, params api.AuthDeleteAttemptParams) error {
	if _, ok := a.authAttempts.get(params.ID); !ok {
		return &apiError{err: errors.New("auth attempt not found"), code: 404}
	}
	a.authAttempts.delete(params.ID)
	return nil
}

func (a *apiService) AuthSendCode(ctx context.Context, req *api.AuthAttemptSendCode, params api.AuthSendCodeParams) error {
	attempt, err := a.getAuthAttempt(params.ID)
	if err != nil {
		return err
	}
	if err := attempt.enqueue(authAttemptCommand{run: func(runCtx context.Context) error {
		hash, err := attempt.tgClient.SendCode(runCtx, req.PhoneNo)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			attempt.publish(authAttemptEvent{Type: "error", Message: err.Error()})
			return nil
		}
		attempt.publish(authAttemptEvent{Type: "phone_code", PhoneCodeHash: hash})
		return nil
	}}); err != nil {
		return &apiError{err: err, code: 409}
	}
	return nil
}

func (a *apiService) AuthSignIn(ctx context.Context, req *api.AuthAttemptSignIn, params api.AuthSignInParams) error {
	attempt, err := a.getAuthAttempt(params.ID)
	if err != nil {
		return err
	}
	if err := attempt.enqueue(authAttemptCommand{run: func(runCtx context.Context) error {
		user, err := attempt.tgClient.SignIn(runCtx, req.PhoneNo, req.PhoneCode, req.PhoneCodeHash)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if a.telegram.IsPasswordAuthNeeded(err) {
			attempt.publish(authAttemptEvent{Type: "password_required", Message: "2FA required"})
			return nil
		}
		if err != nil && strings.Contains(err.Error(), "PHONE_CODE_INVALID") {
			attempt.publish(authAttemptEvent{Type: "error", Message: "PHONE_CODE_INVALID"})
			return nil
		}
		if err != nil {
			attempt.publish(authAttemptEvent{Type: "error", Message: err.Error()})
			return nil
		}
		return a.completeAuthAttempt(runCtx, attempt, user)
	}}); err != nil {
		return &apiError{err: err, code: 409}
	}
	return nil
}

func (a *apiService) AuthPassword(ctx context.Context, req *api.AuthAttemptPassword, params api.AuthPasswordParams) error {
	attempt, err := a.getAuthAttempt(params.ID)
	if err != nil {
		return err
	}
	if err := attempt.enqueue(authAttemptCommand{run: func(runCtx context.Context) error {
		user, err := attempt.tgClient.Password(runCtx, req.Password)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			attempt.publish(authAttemptEvent{Type: "error", Message: err.Error()})
			return nil
		}
		return a.completeAuthAttempt(runCtx, attempt, user)
	}}); err != nil {
		return &apiError{err: err, code: 409}
	}
	return nil
}

func (a *apiService) getAuthAttempt(id string) (*authAttempt, error) {
	attempt, ok := a.authAttempts.get(id)
	if !ok {
		return nil, &apiError{err: errors.New("auth attempt not found"), code: 404}
	}
	return attempt, nil
}

func (a *apiService) runAuthAttempt(ctx context.Context, attempt *authAttempt) {
	logger := logging.Component("AUTH").With(zap.String("attempt_id", attempt.id), zap.String("auth_type", string(attempt.authType)))
	defer attempt.close()
	err := attempt.tgClient.Run(ctx, func(runCtx context.Context) error {
		for {
			select {
			case <-runCtx.Done():
				return nil
			case cmd := <-attempt.commands:
				if cmd.run == nil {
					continue
				}
				if err := cmd.run(runCtx); err != nil {
					attempt.publish(authAttemptEvent{Type: "error", Message: err.Error()})
				}
			}
		}
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("auth.attempt_run_failed", zap.Error(err))
		attempt.publish(authAttemptEvent{Type: "error", Message: err.Error()})
	}
	a.scheduleAttemptCleanup(attempt.id, time.Minute)
}

func (a *apiService) executeQRLogin(ctx context.Context, attempt *authAttempt) error {
	user, err := attempt.tgClient.QRLogin(ctx, attempt.loggedIn, func(ctx context.Context, tokenURL string) error {
		attempt.publish(authAttemptEvent{Type: "token", Token: tokenURL})
		return nil
	})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	if a.telegram.IsSessionPasswordNeeded(err) {
		attempt.publish(authAttemptEvent{Type: "password_required", Message: "2FA required"})
		return nil
	}
	if err != nil {
		attempt.publish(authAttemptEvent{Type: "error", Message: err.Error()})
		return nil
	}
	return a.completeAuthAttempt(ctx, attempt, user)
}

func (a *apiService) completeAuthAttempt(ctx context.Context, attempt *authAttempt, user *TelegramUser) error {
	if !checkUserIsAllowed(a.cnf.JWT.AllowedUsers, user.Username) {
		attempt.publish(authAttemptEvent{Type: "error", Message: "user not allowed"})
		_ = a.telegram.LogOut(ctx, attempt.tgClient)
		return nil
	}
	res, err := attempt.sessionStorage.LoadSession(ctx)
	if err != nil {
		attempt.publish(authAttemptEvent{Type: "error", Message: err.Error()})
		return nil
	}
	sessionData := &types.SessionData{}
	if err := json.Unmarshal(res, sessionData); err != nil {
		attempt.publish(authAttemptEvent{Type: "error", Message: err.Error()})
		return nil
	}
	attempt.publish(authAttemptEvent{Type: "success", Message: "success", Session: prepareSession(user, sessionData)})
	a.scheduleAttemptCleanup(attempt.id, time.Minute)
	return nil
}

func (a *apiService) scheduleAttemptCleanup(id string, after time.Duration) {
	go func() {
		timer := time.NewTimer(after)
		defer timer.Stop()
		<-timer.C
		if attempt, ok := a.authAttempts.get(id); ok {
			attempt.mu.Lock()
			if attempt.state != api.AuthAttemptStateAuthenticated {
				attempt.state = api.AuthAttemptStateExpired
				attempt.message = "expired"
			}
			attempt.mu.Unlock()
		}
		a.authAttempts.delete(id)
	}()
}
