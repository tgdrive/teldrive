package services

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/database"
	"github.com/tgdrive/teldrive/pkg/mapper"
	"github.com/tgdrive/teldrive/pkg/models"
	"github.com/tgdrive/teldrive/pkg/schemas"
	"github.com/tgdrive/teldrive/pkg/types"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type ShareService struct {
	db    *gorm.DB
	fs    *FileService
	cache cache.Cacher
}

var (
	ErrShareNotFound   = errors.New("share not found")
	ErrInvalidPassword = errors.New("invalid password")
	ErrShareExpired    = errors.New("share expired")
)

func NewShareService(db *gorm.DB, fs *FileService, cache cache.Cacher) *ShareService {
	return &ShareService{db: db, fs: fs, cache: cache}
}

func (ss *ShareService) GetShareById(shareId string) (*schemas.FileShareOut, *types.AppError) {

	var result []models.FileShare

	if err := ss.db.Model(&models.FileShare{}).Where("id = ?", shareId).Find(&result).Error; err != nil {
		return nil, &types.AppError{Error: err}
	}

	if len(result) == 0 {
		return nil, &types.AppError{Error: ErrShareNotFound, Code: http.StatusNotFound}
	}

	if result[0].ExpiresAt != nil && result[0].ExpiresAt.Before(time.Now().UTC()) {
		return nil, &types.AppError{Error: ErrShareExpired, Code: http.StatusNotFound}
	}

	res := &schemas.FileShareOut{
		ExpiresAt: result[0].ExpiresAt,
		Protected: result[0].Password != nil,
		UserID:    result[0].UserID,
	}

	return res, nil
}

func (ss *ShareService) ShareUnlock(shareId string, payload *schemas.ShareAccess) *types.AppError {

	var result []models.FileShare

	if err := ss.db.Model(&models.FileShare{}).Where("id = ?", shareId).Find(&result).Error; err != nil {
		return &types.AppError{Error: err}
	}

	if len(result) == 0 {
		return &types.AppError{Error: ErrShareNotFound, Code: http.StatusNotFound}
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*result[0].Password), []byte(payload.Password)); err != nil {
		return &types.AppError{Error: ErrInvalidPassword, Code: http.StatusUnauthorized}
	}
	return nil
}

func (ss *ShareService) ListShareFiles(shareId string, query *schemas.ShareFileQuery, auth string) (*schemas.FileResponse, *types.AppError) {

	var (
		userId   int64
		fileType string
		fileId   string
	)

	var result []schemas.FileShare

	key := "shares:" + shareId

	if err := ss.cache.Get(key, &result); err != nil {
		if err := ss.db.Model(&models.FileShare{}).Where("file_shares.id = ?", shareId).
			Select("file_shares.*", "f.type").
			Joins("left join teldrive.files as f on f.id = file_shares.file_id").
			Scan(&result).Error; err != nil {
			return nil, &types.AppError{Error: err}
		}

		if len(result) == 0 {
			return nil, &types.AppError{Error: ErrShareNotFound, Code: http.StatusNotFound}
		}
		ss.cache.Set(key, result, 0)
	}

	if result[0].Password != nil {
		if auth == "" {
			return nil, &types.AppError{Error: ErrInvalidPassword, Code: http.StatusUnauthorized}
		}
		bytes, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
		password := strings.Split(string(bytes), ":")[1]
		if err != nil {
			return nil, &types.AppError{Error: err}
		}
		if err := bcrypt.CompareHashAndPassword([]byte(*result[0].Password), []byte(password)); err != nil {
			return nil, &types.AppError{Error: ErrInvalidPassword, Code: http.StatusUnauthorized}
		}

	}

	userId = result[0].UserId

	fileType = "folder"

	fileId = query.ParentID

	if query.ParentID == "" {
		fileType = result[0].Type
		fileId = result[0].FileId
	}

	if fileType == "folder" {
		return ss.fs.ListFiles(userId, &schemas.FileQuery{
			ParentID: fileId,
			Limit:    query.Limit,
			Page:     query.Page,
			Order:    query.Order,
			Sort:     query.Sort,
			Op:       "list"})
	} else {
		var file models.File
		if err := ss.db.Where("id = ?", fileId).First(&file).Error; err != nil {
			if database.IsRecordNotFoundErr(err) {
				return nil, &types.AppError{Error: database.ErrNotFound, Code: http.StatusNotFound}
			}
			return nil, &types.AppError{Error: err}
		}
		return &schemas.FileResponse{Files: []schemas.FileOut{*mapper.ToFileOut(file)}}, nil
	}

}

func (ss *ShareService) StreamSharedFile(c *gin.Context, download bool) {

	shareID := c.Param("shareID")

	res, err := ss.GetShareById(shareID)

	if err != nil {
		http.Error(c.Writer, err.Error.Error(), err.Code)
		return
	}
	ss.fs.GetFileStream(c, download, res)
}
