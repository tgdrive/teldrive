package services

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/media"
	"github.com/tgdrive/teldrive/pkg/models"
)

// ResolveMedia always builds a local server URL after verifying account/share access.
func (a *apiService) ResolveMedia(r *http.Request, id string) (media.Source, error) {
	denied := errors.New("access denied")
	if _, err := uuid.Parse(id); err != nil {
		return media.Source{}, denied
	}
	var file models.File
	if err := a.db.WithContext(r.Context()).Where("id = ? AND type = 'file' AND status IN ('active','trash')", id).First(&file).Error; err != nil {
		return media.Source{}, denied
	}
	if shareID := chi.URLParam(r, "share"); shareID != "" {
		share, err := a.validFileShare(r, shareID)
		if err != nil || share.UserId != file.UserId {
			return media.Source{}, denied
		}
		var allowed int64
		query := `WITH RECURSIVE tree AS (SELECT id FROM teldrive.files WHERE id = ? AND user_id = ? UNION SELECT f.id FROM teldrive.files f JOIN tree t ON f.parent_id = t.id WHERE f.user_id = ?) SELECT COUNT(*) FROM tree WHERE id = ?`
		if err := a.db.WithContext(r.Context()).Raw(query, share.FileId, share.UserId, share.UserId, id).Scan(&allowed).Error; err != nil || allowed != 1 {
			return media.Source{}, denied
		}
		headers := ""
		if cookie, err := r.Cookie("teldrive_share"); err == nil {
			headers = "Cookie: " + cookie.String() + "\r\n"
		}
		return media.Source{URL: media.LocalURL(a.cnf.Server.Port, "/api/shares/"+url.PathEscape(shareID)+"/files/"+id+"/"+url.PathEscape(file.Name)), Headers: headers}, nil
	}
	cookie, err := r.Cookie("access_token")
	if err != nil {
		return media.Source{}, denied
	}
	claims, err := auth.VerifyUser(r.Context(), a.db, a.cache, a.cnf.JWT.Secret, cookie.Value)
	if err != nil || claims.Subject != fmt.Sprint(file.UserId) {
		return media.Source{}, denied
	}
	return media.Source{URL: media.LocalURL(a.cnf.Server.Port, "/api/files/"+id+"/"+url.PathEscape(file.Name)+"?hash="+url.QueryEscape(claims.Hash)), Headers: "Cookie: " + cookie.String() + "\r\n"}, nil
}
