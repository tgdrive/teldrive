package api

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
)

const shareDownloadCookieName = "teldrive_share_download"

func (h *Handler) AuthorizePublicShareDownload(ctx context.Context, params gen.AuthorizePublicShareDownloadParams) (gen.AuthorizePublicShareDownloadRes, error) {
	if h.Shares == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	session, err := h.Shares.CreateDownloadSession(ctx, params.Token, params.XSharePassword.Or(""))
	if err != nil {
		return nil, mapServiceError(err)
	}
	cookie := h.shareDownloadCookie(ctx, params.Token, session.Token, session.ExpiresAt)
	return &gen.AuthorizePublicShareDownloadNoContent{
		SetCookie:    cookie.String(),
		CacheControl: gen.AuthorizePublicShareDownloadNoContentCacheControlNoStore,
	}, nil
}

func (h *Handler) shareDownloadCookie(ctx context.Context, token, secret string, expiresAt time.Time) *http.Cookie {
	cookie := h.cookie(ctx, shareDownloadCookieName, secret, time.Until(expiresAt))
	cookie.Path = "/api/v1/public/shares/" + url.PathEscape(token) + "/"
	cookie.Expires = expiresAt
	cookie.SameSite = http.SameSiteStrictMode
	return cookie
}
