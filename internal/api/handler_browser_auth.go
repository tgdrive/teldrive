package api

import (
	"context"
	"net/http"
	"time"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/authn"
)

const (
	browserAccessCookieName  = "teldrive_access"
	browserRefreshCookieName = "teldrive_refresh"
)

type browserCookieSecureContextKey struct{}

func WithBrowserCookieSecure(ctx context.Context, secure bool) context.Context {
	return context.WithValue(ctx, browserCookieSecureContextKey{}, secure)
}

func browserCookieSecure(ctx context.Context) bool {
	secure, _ := ctx.Value(browserCookieSecureContextKey{}).(bool)
	return secure
}

func (h *Handler) BrowserTelegramLoginVerifyCode(ctx context.Context, req *gen.TelegramCodeVerifyRequest, _ gen.BrowserTelegramLoginVerifyCodeParams) (gen.BrowserTelegramLoginVerifyCodeRes, error) {
	if h.Auth == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	result, err := h.Auth.VerifyCode(ctx, googleUUID(req.FlowId), req.Code)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if result.Flow != nil {
		response := loginFlowResponse(result.Flow)
		return &response, nil
	}
	return h.browserSessionResponse(ctx, result.Tokens)
}

func (h *Handler) BrowserTelegramLoginVerifyPassword(ctx context.Context, req *gen.TelegramPasswordVerifyRequest, _ gen.BrowserTelegramLoginVerifyPasswordParams) (gen.BrowserTelegramLoginVerifyPasswordRes, error) {
	if h.Auth == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	result, err := h.Auth.VerifyPassword(ctx, googleUUID(req.FlowId), req.Password)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return h.browserSessionResponse(ctx, result.Tokens)
}

func (h *Handler) BrowserTelegramQRLoginPoll(ctx context.Context, req *gen.TelegramQRLoginPollRequest, _ gen.BrowserTelegramQRLoginPollParams) (gen.BrowserTelegramQRLoginPollRes, error) {
	if h.Auth == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	result, err := h.Auth.PollQR(ctx, googleUUID(req.FlowId))
	if err != nil {
		return nil, mapServiceError(err)
	}
	if result.QRFlow != nil {
		response := qrLoginFlowResponse(result.QRFlow)
		return &response, nil
	}
	return h.browserSessionResponse(ctx, result.Tokens)
}

func (h *Handler) RefreshBrowserSession(ctx context.Context, params gen.RefreshBrowserSessionParams) (gen.RefreshBrowserSessionRes, error) {
	if h.Auth == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	tokens, err := h.Auth.Refresh(ctx, params.TeldriveRefresh)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return h.browserSessionResponse(ctx, tokens)
}

func (h *Handler) LogoutBrowserSession(ctx context.Context) (gen.LogoutBrowserSessionRes, error) {
	if h.Auth == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	identity, ok := IdentityFromContext(ctx)
	if !ok {
		return nil, mapServiceError(ErrUnauthenticated)
	}
	if err := h.Auth.Logout(ctx, identity); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.LogoutBrowserSessionNoContent{SetCookie: h.expiredBrowserCookies(ctx)}, nil
}

func (h *Handler) browserSessionResponse(ctx context.Context, tokens *authn.TokenPair) (*gen.BrowserSessionHeaders, error) {
	if h == nil || h.Auth == nil || tokens == nil || tokens.AccessToken == "" || tokens.RefreshToken == "" || tokens.ExpiresIn <= 0 {
		return nil, ErrOperationUnavailable
	}
	now := time.Now().UTC()
	accessTTL := time.Duration(tokens.ExpiresIn) * time.Second
	refreshTTL := h.Auth.RefreshTokenTTL()
	return &gen.BrowserSessionHeaders{
		SetCookie: []string{
			h.browserCookie(ctx, browserAccessCookieName, tokens.AccessToken, accessTTL).String(),
			h.browserCookie(ctx, browserRefreshCookieName, tokens.RefreshToken, refreshTTL).String(),
		},
		Response: gen.BrowserSession{
			Authenticated: gen.BrowserSessionAuthenticatedTrue,
			ExpiresAt:     now.Add(accessTTL),
		},
	}, nil
}

func (h *Handler) browserCookie(ctx context.Context, name, value string, ttl time.Duration) *http.Cookie {
	maxAge := int(ttl / time.Second)
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		Expires:  time.Now().UTC().Add(ttl),
		HttpOnly: true,
		Secure:   browserCookieSecure(ctx),
		SameSite: http.SameSiteLaxMode,
	}
}

func (h *Handler) expiredBrowserCookies(ctx context.Context) []string {
	expires := time.Unix(1, 0).UTC()
	cookies := make([]string, 0, 2)
	for _, name := range []string{browserAccessCookieName, browserRefreshCookieName} {
		cookies = append(cookies, (&http.Cookie{
			Name:     name,
			Path:     "/",
			MaxAge:   -1,
			Expires:  expires,
			HttpOnly: true,
			Secure:   browserCookieSecure(ctx),
			SameSite: http.SameSiteLaxMode,
		}).String())
	}
	return cookies
}
