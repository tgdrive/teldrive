package api

import (
	"context"
	"net/http"
	"time"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/authn"
)

const (
	accessCookieName  = "teldrive_access"
	refreshCookieName = "teldrive_refresh"
)

type cookieSecureContextKey struct{}

func WithCookieSecure(ctx context.Context, secure bool) context.Context {
	return context.WithValue(ctx, cookieSecureContextKey{}, secure)
}

func cookieSecure(ctx context.Context) bool {
	secure, _ := ctx.Value(cookieSecureContextKey{}).(bool)
	return secure
}

func (h *Handler) CookieTelegramLoginVerifyCode(ctx context.Context, req *gen.TelegramCodeVerifyRequest, _ gen.CookieTelegramLoginVerifyCodeParams) (gen.CookieTelegramLoginVerifyCodeRes, error) {
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
	return h.cookieSessionResponse(ctx, result.Tokens)
}

func (h *Handler) CookieTelegramLoginVerifyPassword(ctx context.Context, req *gen.TelegramPasswordVerifyRequest, _ gen.CookieTelegramLoginVerifyPasswordParams) (gen.CookieTelegramLoginVerifyPasswordRes, error) {
	if h.Auth == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	result, err := h.Auth.VerifyPassword(ctx, googleUUID(req.FlowId), req.Password)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return h.cookieSessionResponse(ctx, result.Tokens)
}

func (h *Handler) CookieTelegramQRLoginPoll(ctx context.Context, req *gen.TelegramQRLoginPollRequest, _ gen.CookieTelegramQRLoginPollParams) (gen.CookieTelegramQRLoginPollRes, error) {
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
	return h.cookieSessionResponse(ctx, result.Tokens)
}

func (h *Handler) RefreshCookieSession(ctx context.Context, params gen.RefreshCookieSessionParams) (gen.RefreshCookieSessionRes, error) {
	if h.Auth == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	tokens, err := h.Auth.Refresh(ctx, params.TeldriveRefresh)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return h.cookieSessionResponse(ctx, tokens)
}

func (h *Handler) LogoutCookieSession(ctx context.Context) (gen.LogoutCookieSessionRes, error) {
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
	return &gen.LogoutCookieSessionNoContent{SetCookie: h.expiredCookies(ctx)}, nil
}

func (h *Handler) cookieSessionResponse(ctx context.Context, tokens *authn.TokenPair) (*gen.CookieSessionHeaders, error) {
	if h == nil || h.Auth == nil || tokens == nil || tokens.AccessToken == "" || tokens.RefreshToken == "" || tokens.ExpiresIn <= 0 {
		return nil, ErrOperationUnavailable
	}
	now := time.Now().UTC()
	accessTTL := time.Duration(tokens.ExpiresIn) * time.Second
	refreshTTL := h.Auth.RefreshTokenTTL()
	return &gen.CookieSessionHeaders{
		SetCookie: []string{
			h.cookie(ctx, accessCookieName, tokens.AccessToken, accessTTL).String(),
			h.cookie(ctx, refreshCookieName, tokens.RefreshToken, refreshTTL).String(),
		},
		Response: gen.CookieSession{
			Authenticated: gen.CookieSessionAuthenticatedTrue,
			ExpiresAt:     now.Add(accessTTL),
		},
	}, nil
}

func (h *Handler) cookie(ctx context.Context, name, value string, ttl time.Duration) *http.Cookie {
	maxAge := int(ttl / time.Second)
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		Expires:  time.Now().UTC().Add(ttl),
		HttpOnly: true,
		Secure:   cookieSecure(ctx),
		SameSite: http.SameSiteLaxMode,
	}
}

func (h *Handler) expiredCookies(ctx context.Context) []string {
	expires := time.Unix(1, 0).UTC()
	cookies := make([]string, 0, 2)
	for _, name := range []string{accessCookieName, refreshCookieName} {
		cookies = append(cookies, (&http.Cookie{
			Name:     name,
			Path:     "/",
			MaxAge:   -1,
			Expires:  expires,
			HttpOnly: true,
			Secure:   cookieSecure(ctx),
			SameSite: http.SameSiteLaxMode,
		}).String())
	}
	return cookies
}
