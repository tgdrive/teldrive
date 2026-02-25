package services

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/cache"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
	"github.com/tgdrive/teldrive/pkg/mapper"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrShareNotFound   = errors.New("share not found")
	ErrInvalidPassword = errors.New("invalid password")
	ErrEmptyAuth       = errors.New("empty auth")
	ErrShareExpired    = errors.New("share expired")
	ErrInvalidShareTok = errors.New("invalid share token")
)

const shareCookieName = "share_token"

type shareTokenClaims struct {
	ShareID string `json:"shareId"`
	UserID  int64  `json:"userId"`
	jwt.RegisteredClaims
}

type fileShare struct {
	ID        string
	FileID    string
	Password  *string
	ExpiresAt *time.Time
	UserID    int64
	Type      api.FileShareInfoType
	Name      string
	Path      string
}

func (a *apiService) shareGetById(ctx context.Context, id string) (*fileShare, error) {
	shareID, err := uuid.Parse(id)
	if err != nil {
		return nil, &apiError{err: err, code: http.StatusBadRequest}
	}
	share, err := a.repo.Shares.GetByID(ctx, shareID)
	if err != nil {
		return nil, &apiError{err: ErrShareNotFound, code: http.StatusNotFound}
	}
	if share.ExpiresAt != nil && share.ExpiresAt.Before(time.Now().UTC()) {
		return nil, &apiError{err: ErrShareExpired, code: http.StatusNotFound}
	}
	file, err := a.repo.Files.GetByID(ctx, share.FileID)
	if err != nil {
		return nil, &apiError{err: err}
	}
	path, err := a.repo.Files.GetFullPath(ctx, share.FileID)
	if err != nil {
		return nil, &apiError{err: err}
	}

	return &fileShare{
		ID:        share.ID.String(),
		FileID:    share.FileID.String(),
		Password:  share.Password,
		ExpiresAt: share.ExpiresAt,
		UserID:    share.UserID,
		Type:      api.FileShareInfoType(file.Type),
		Name:      file.Name,
		Path:      path,
	}, nil
}

func (a *apiService) SharesGetById(ctx context.Context, params api.SharesGetByIdParams) (*api.FileShareInfo, error) {
	share, err := a.shareGetById(ctx, params.ID)

	if err != nil {
		return nil, err
	}
	res := &api.FileShareInfo{
		Protected: share.Password != nil,
		UserId:    share.UserID,
		Type:      share.Type,
		Name:      share.Name,
	}
	if share.ExpiresAt != nil {
		res.ExpiresAt = api.NewOptDateTime(*share.ExpiresAt)
	}
	return res, nil
}

func (a *apiService) issueShareToken(share *jetmodel.FileShares) (string, time.Time, error) {
	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	if share.ExpiresAt != nil && share.ExpiresAt.Before(expiresAt) {
		expiresAt = *share.ExpiresAt
	}
	claims := &shareTokenClaims{
		ShareID: share.ID.String(),
		UserID:  share.UserID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   share.ID.String(),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(a.cnf.JWT.Secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func (a *apiService) validateShareToken(token, shareID string) error {
	if token == "" {
		return ErrEmptyAuth
	}
	parsed, err := jwt.ParseWithClaims(token, &shareTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(a.cnf.JWT.Secret), nil
	})
	if err != nil {
		return ErrInvalidShareTok
	}
	claims, ok := parsed.Claims.(*shareTokenClaims)
	if !ok || !parsed.Valid {
		return ErrInvalidShareTok
	}
	if claims.ShareID != shareID {
		return ErrInvalidShareTok
	}
	return nil
}

func (a *apiService) SharesUnlock(ctx context.Context, req *api.ShareUnlock, params api.SharesUnlockParams) (*api.SharesUnlockNoContent, error) {
	shareID, err := uuid.Parse(params.ID)
	if err != nil {
		return nil, &apiError{err: err, code: http.StatusBadRequest}
	}
	share, err := a.repo.Shares.GetByID(ctx, shareID)
	if err != nil {
		return nil, &apiError{err: ErrShareNotFound, code: http.StatusNotFound}
	}
	if share.Password == nil {
		return &api.SharesUnlockNoContent{SetCookie: setCookie(ctx, shareCookieName, "", -1)}, nil
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*share.Password), []byte(req.Password)); err != nil {
		return nil, &apiError{err: ErrInvalidPassword, code: http.StatusForbidden}
	}
	token, expiresAt, err := a.issueShareToken(share)
	if err != nil {
		return nil, &apiError{err: err}
	}
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	return &api.SharesUnlockNoContent{SetCookie: setCookie(ctx, shareCookieName, token, maxAge)}, nil
}

func (a *apiService) SharesListFiles(ctx context.Context, params api.SharesListFilesParams) (*api.FileList, error) {
	share, err := a.validFileShare(ctx, params.ID, params.ShareToken.Or(""))
	if err != nil {
		return nil, err
	}
	fileType := share.Type

	if fileType == api.FileShareInfoTypeFolder {
		qParams := repositories.FileQueryParams{
			UserID:    share.UserID,
			Operation: "list",
			Status:    "active",
			Path:      share.Path + params.Path.Or(""),
			Sort:      string(params.Sort.Value),
			Order:     string(params.Order.Value),
			Limit:     params.Limit.Value,
			Cursor:    params.Cursor.Value,
		}

		res, err := a.repo.Files.List(ctx, qParams)
		if err != nil {
			return nil, &apiError{err: err}
		}

		items := make([]api.File, 0, len(res))
		for _, item := range res {
			items = append(items, *mapper.ToJetFileOut(item))
		}

		var nextCursor api.OptString
		if len(res) > 0 && len(res) == qParams.Limit {
			last := res[len(res)-1]
			cursorVal := last.UpdatedAt.Format(time.RFC3339Nano)
			switch strings.ToLower(qParams.Sort) {
			case "name":
				cursorVal = last.Name
			case "size":
				if last.Size != nil {
					cursorVal = strconv.FormatInt(*last.Size, 10)
				}
			case "id":
				cursorVal = last.ID.String()
			}
			nextCursor.SetTo(cursorVal + ":" + last.ID.String())
		}

		return &api.FileList{Items: items, Meta: api.Meta{NextCursor: nextCursor}}, nil
	} else {
		fileID, err := uuid.Parse(share.FileID)
		if err != nil {
			return nil, &apiError{err: err, code: http.StatusBadRequest}
		}
		file, err := a.repo.Files.GetByID(ctx, fileID)
		if err != nil {
			if errors.Is(err, repositories.ErrNotFound) {
				return nil, &apiError{err: ErrShareNotFound, code: http.StatusNotFound}
			}
			return nil, &apiError{err: err}
		}
		return &api.FileList{Items: []api.File{*mapper.ToJetFileOut(*file)}, Meta: api.Meta{}}, nil
	}

}
func (a *apiService) validFileShare(ctx context.Context, id string, shareToken string) (*fileShare, error) {

	share, err := cache.Fetch(ctx, a.cache, cache.KeyShare(id), 0, func() (*fileShare, error) {
		return a.shareGetById(ctx, id)
	})

	if err != nil {
		return nil, &apiError{err: err}
	}

	if share.Password != nil {
		if shareToken == "" {
			return nil, &apiError{err: ErrEmptyAuth, code: http.StatusUnauthorized}
		}
		if err := a.validateShareToken(shareToken, share.ID); err != nil {
			if errors.Is(err, ErrEmptyAuth) {
				return nil, &apiError{err: err, code: http.StatusUnauthorized}
			}
			return nil, &apiError{err: ErrInvalidPassword, code: http.StatusUnauthorized}
		}
	}
	return share, nil
}
