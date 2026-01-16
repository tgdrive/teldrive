package services

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/appcontext"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/database"
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

	if err := bcrypt.CompareHashAndPassword([]byte(*result[0].Password), []byte(req.Password)); err != nil {
		return &apiError{err: ErrInvalidPassword, code: http.StatusForbidden}
	}
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

	if share.Password != nil {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			return nil, &apiError{err: ErrEmptyAuth, code: http.StatusUnauthorized}
		}
		bytes, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authHeader, "Basic "))
		if err != nil {
			return nil, &apiError{err: err}
		}
		password := strings.Split(string(bytes), ":")[1]

		if err := bcrypt.CompareHashAndPassword([]byte(*share.Password), []byte(password)); err != nil {
			return nil, &apiError{err: ErrInvalidPassword, code: http.StatusUnauthorized}
		}

	}
	return share, nil
}
