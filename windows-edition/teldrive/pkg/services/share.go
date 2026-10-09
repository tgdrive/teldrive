package services

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/appcontext"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/database"
	"github.com/tgdrive/teldrive/internal/shareauth"
	"github.com/tgdrive/teldrive/pkg/mapper"
	"github.com/tgdrive/teldrive/pkg/models"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrShareNotFound   = errors.New("share not found")
	ErrInvalidPassword = errors.New("invalid password")
	ErrEmptyAuth       = errors.New("empty auth")
	ErrShareExpired    = errors.New("share expired")
)

// Omitted fields retain their values. Empty password and zero expiresAt clear them.
func shareUpdateValues(req *api.FileShareCreate) (map[string]interface{}, error) {
	updates := map[string]interface{}{}
	if req.Password.IsSet() {
		updates["password"] = nil
		if req.Password.Value != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(req.Password.Value), bcrypt.MinCost)
			if err != nil {
				return nil, err
			}
			updates["password"] = string(hash)
		}
	}
	if req.ExpiresAt.IsSet() {
		updates["expires_at"] = nil
		if !req.ExpiresAt.Value.IsZero() {
			updates["expires_at"] = req.ExpiresAt.Value
		}
	}
	return updates, nil
}

type fileShare struct {
	models.FileShare
	Type api.FileShareInfoType
	Name string
	Path string
}

func (a *apiService) shareGetById(id string) (*fileShare, error) {
	var result []struct {
		models.FileShare
		Type api.FileShareInfoType `gorm:"column:type"`
		Name string                `gorm:"column:name"`
	}

	if err := a.db.Model(&models.FileShare{}).Where("file_shares.id = ?", id).
		Select("file_shares.*", "f.type", "f.name").
		Joins("left join teldrive.files as f on f.id = file_shares.file_id").
		Scan(&result).Error; err != nil {
		return nil, &apiError{err: err}
	}

	if len(result) == 0 {
		return nil, &apiError{err: ErrShareNotFound, code: http.StatusNotFound}
	}

	if result[0].ExpiresAt != nil && result[0].ExpiresAt.Before(time.Now().UTC()) {
		return nil, &apiError{err: ErrShareExpired, code: http.StatusNotFound}
	}

	path, err := a.getFullPath(a.db, result[0].FileId)
	if err != nil {
		return nil, &apiError{err: err}
	}

	return &fileShare{
		FileShare: result[0].FileShare,
		Type:      result[0].Type,
		Name:      result[0].Name,
		Path:      path,
	}, nil
}

func (a *apiService) SharesGetById(ctx context.Context, params api.SharesGetByIdParams) (*api.FileShareInfo, error) {
	share, err := a.shareGetById(params.ID)

	if err != nil {
		return nil, err
	}
	res := &api.FileShareInfo{
		Protected: share.Password != nil,
		UserId:    share.UserId,
		Type:      share.Type,
		Name:      share.Name,
	}
	if share.ExpiresAt != nil {
		res.ExpiresAt = api.NewOptDateTime(*share.ExpiresAt)
	}
	return res, nil
}

func (a *apiService) SharesUnlock(ctx context.Context, req *api.ShareUnlock, params api.SharesUnlockParams) error {
	var result []models.FileShare

	if err := a.db.Model(&models.FileShare{}).Where("id = ?", params.ID).Find(&result).Error; err != nil {
		return &apiError{err: err}
	}

	if len(result) == 0 {
		return &apiError{err: ErrShareNotFound, code: http.StatusNotFound}
	}

	if result[0].ExpiresAt != nil && !result[0].ExpiresAt.After(time.Now()) {
		return &apiError{err: ErrShareExpired, code: http.StatusNotFound}
	}
	if result[0].Password == nil {
		return nil
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*result[0].Password), []byte(req.Password)); err != nil {
		return &apiError{err: ErrInvalidPassword, code: http.StatusForbidden}
	}
	c := ctx.(*appcontext.Context)
	expires := time.Now().Add(time.Hour)
	if result[0].ExpiresAt != nil && result[0].ExpiresAt.Before(expires) {
		expires = *result[0].ExpiresAt
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name: "teldrive_share", Value: shareauth.Issue(params.ID, *result[0].Password, expires),
		Path: "/api/shares/" + params.ID, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure:  c.Request.TLS != nil || c.Request.Header.Get("X-Forwarded-Proto") == "https",
		Expires: expires,
	})
	return nil
}

func (a *apiService) SharesListFiles(ctx context.Context, params api.SharesListFilesParams) (*api.FileList, error) {
	c := ctx.(*appcontext.Context)
	share, err := a.validFileShare(c.Request, params.ID)
	if err != nil {
		return nil, err
	}
	fileType := share.Type

	if fileType == api.FileShareInfoTypeFolder {
		queryBuilder := &fileQueryBuilder{db: a.db}
		return queryBuilder.execute(&api.FilesListParams{
			Path:      api.NewOptString(share.Path + params.Path.Or("")),
			Limit:     params.Limit,
			Page:      params.Page,
			Status:    api.NewOptFileQueryStatus(api.FileQueryStatusActive),
			Order:     api.NewOptFileQueryOrder(api.FileQueryOrder(string(params.Order.Value))),
			Sort:      api.NewOptFileQuerySort(api.FileQuerySort(string(params.Sort.Value))),
			Operation: api.NewOptFileQueryOperation(api.FileQueryOperationList)}, share.UserId)
	} else {
		var file models.File
		if err := a.db.Where("id = ?", share.FileId).First(&file).Error; err != nil {
			if database.IsRecordNotFoundErr(err) {
				return nil, &apiError{err: database.ErrNotFound, code: http.StatusNotFound}
			}
			return nil, &apiError{err: err}
		}
		return &api.FileList{Items: []api.File{*mapper.ToFileOut(file)},
			Meta: api.Meta{Count: 1, TotalPages: 1, CurrentPage: 1}}, nil
	}

}
func (a *apiService) validFileShare(r *http.Request, id string) (*fileShare, error) {

	share, err := cache.FetchArg(r.Context(), a.cache, cache.KeyShare(id), 0, a.shareGetById, id)

	if err != nil {
		return nil, &apiError{err: err}
	}
	if share.ExpiresAt != nil && !share.ExpiresAt.After(time.Now()) {
		return nil, &apiError{err: ErrShareExpired, code: http.StatusNotFound}
	}
	var fileCount int64
	if err := a.db.WithContext(r.Context()).Model(&models.File{}).Where("id = ? AND user_id = ? AND status IN ('active','trash')", share.FileId, share.UserId).Count(&fileCount).Error; err != nil || fileCount == 0 {
		return nil, &apiError{err: ErrShareNotFound, code: http.StatusNotFound}
	}

	if share.Password != nil {
		if cookie, err := r.Cookie("teldrive_share"); err == nil && shareauth.Valid(cookie.Value, id, *share.Password, time.Now()) {
			return share, nil
		}
		_, password, ok := r.BasicAuth()
		if !ok {
			return nil, &apiError{err: ErrEmptyAuth, code: http.StatusUnauthorized}
		}
		if err := bcrypt.CompareHashAndPassword([]byte(*share.Password), []byte(password)); err != nil {
			return nil, &apiError{err: ErrInvalidPassword, code: http.StatusUnauthorized}
		}

	}
	return share, nil
}
