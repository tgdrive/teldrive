package services

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/category"
	"github.com/tgdrive/teldrive/internal/database"
	"github.com/tgdrive/teldrive/internal/http_range"
	"github.com/tgdrive/teldrive/internal/md5"
	"github.com/tgdrive/teldrive/internal/reader"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/mapper"
	"github.com/tgdrive/teldrive/pkg/models"
	"github.com/tgdrive/teldrive/pkg/types"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrorStreamAbandoned = errors.New("stream abandoned")
	defaultContentType   = "application/octet-stream"
)

type buffer struct {
	Buf []byte
}

func (b *buffer) long() (int64, error) {
	v, err := b.uint64()
	if err != nil {
		return 0, err
	}
	return int64(v), nil

}
func (b *buffer) uint64() (uint64, error) {
	const size = 8
	if len(b.Buf) < size {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.LittleEndian.Uint64(b.Buf)
	b.Buf = b.Buf[size:]
	return v, nil
}

func randInt64() (int64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(rand.Reader, buf[:]); err != nil {
		return 0, err
	}
	b := &buffer{Buf: buf[:]}
	return b.long()
}
func isUUId(str string) bool {
	_, err := uuid.Parse(str)
	return err == nil
}

type fullFileDB struct {
	models.File
	Path string
}

func (a *apiService) getFileFromPath(path string, userId int64) (*models.File, error) {

	var res []models.File

	if err := a.db.Raw("select * from teldrive.get_file_from_path(?, ?, ?)", path, userId, true).
		Scan(&res).Error; err != nil {
		return nil, err

	}
	if len(res) == 0 {
		return nil, database.ErrNotFound
	}
	return &res[0], nil
}

func (a *apiService) FilesCategoryStats(ctx context.Context) ([]api.CategoryStats, error) {
	userId := auth.GetUser(ctx)
	var stats []api.CategoryStats
	if err := a.db.Model(&models.File{}).Select("category", "COUNT(*) as total_files", "coalesce(SUM(size),0) as total_size").
		Where(&models.File{UserId: userId, Type: "file", Status: "active"}).
		Order("category ASC").Group("category").Find(&stats).Error; err != nil {
		return nil, &apiError{err: err}
	}

	return stats, nil
}

func (a *apiService) FilesCopy(ctx context.Context, req *api.FileCopy, params api.FilesCopyParams) (*api.File, error) {

	userId := auth.GetUser(ctx)

	client, _ := tgc.AuthClient(ctx, &a.cnf.TG, auth.GetJWTUser(ctx).TgSession, a.middlewares...)

	var res []models.File

	if err := a.db.Model(&models.File{}).Where("id = ?", params.ID).Find(&res).Error; err != nil {
		return nil, &apiError{err: err}
	}
	if len(res) == 0 {
		return nil, &apiError{err: errors.New("file not found"), code: 404}
	}

	file := res[0]

	newIds := []api.Part{}

	channelId, err := getDefaultChannel(a.db, a.cache, userId)
	if err != nil {
		return nil, &apiError{err: err}
	}

	err = tgc.RunWithAuth(ctx, client, "", func(ctx context.Context) error {

		ids := utils.Map(file.Parts, func(part api.Part) int { return part.ID })
		messages, err := tgc.GetMessages(ctx, client.API(), ids, *file.ChannelId)

		if err != nil {
			return err
		}

		channel, err := tgc.GetChannelById(ctx, client.API(), channelId)

		if err != nil {
			return err
		}
		for i, message := range messages {
			item := message.(*tg.Message)
			media := item.Media.(*tg.MessageMediaDocument)
			document := media.Document.(*tg.Document)

			id, _ := randInt64()
			request := tg.MessagesSendMediaRequest{
				Silent:   true,
				Peer:     &tg.InputPeerChannel{ChannelID: channel.ChannelID, AccessHash: channel.AccessHash},
				Media:    &tg.InputMediaDocument{ID: document.AsInput()},
				RandomID: id,
			}
			res, err := client.API().MessagesSendMedia(ctx, &request)

			if err != nil {
				return err
			}

			updates := res.(*tg.Updates)

			var msg *tg.Message

			for _, update := range updates.Updates {
				channelMsg, ok := update.(*tg.UpdateNewChannelMessage)
				if ok {
					msg = channelMsg.Message.(*tg.Message)
					break
				}

			}
			p := api.Part{ID: msg.ID}
			if file.Parts[i].Salt.Value != "" {
				p.Salt = file.Parts[i].Salt
			}
			newIds = append(newIds, p)

		}
		return nil
	})

	if err != nil {
		return nil, &apiError{err: err}
	}

	if len(newIds) != len(file.Parts) {
		return nil, &apiError{err: errors.New("failed to copy all file parts")}
	}

	var parentId string
	if !isUUId(req.Destination) {
		var destRes []models.File
		if err := a.db.Raw("select * from teldrive.create_directories(?, ?)", userId, req.Destination).
			Scan(&destRes).Error; err != nil {
			return nil, &apiError{err: err}
		}
		parentId = destRes[0].ID
	} else {
		parentId = req.Destination
	}

	dbFile := models.File{}

	dbFile.Name = req.NewName.Or(file.Name)
	dbFile.Size = file.Size
	dbFile.Type = string(file.Type)
	dbFile.MimeType = file.MimeType
	if len(newIds) > 0 {
		dbFile.Parts = datatypes.NewJSONSlice(newIds)
	}
	dbFile.UserId = userId
	dbFile.Status = "active"
	dbFile.ParentId = utils.Ptr(parentId)
	dbFile.ChannelId = &channelId
	dbFile.Encrypted = file.Encrypted
	dbFile.Category = string(file.Category)
	if req.UpdatedAt.IsSet() && !req.UpdatedAt.Value.IsZero() {
		dbFile.UpdatedAt = req.UpdatedAt.Value
	} else {
		dbFile.UpdatedAt = time.Now().UTC()
	}

	if err := a.db.Create(&dbFile).Error; err != nil {
		return nil, &apiError{err: err}
	}

	return mapper.ToFileOut(dbFile), nil
}

func (a *apiService) FilesCreate(ctx context.Context, fileIn *api.File) (*api.File, error) {
	userId := auth.GetUser(ctx)

	var (
		fileDB    models.File
		parent    *models.File
		err       error
		path      string
		channelId int64
	)

	if fileIn.Path.Value == "" && fileIn.ParentId.Value == "" {
		return nil, &apiError{err: errors.New("parent id or path is required"), code: 409}
	}

	if fileIn.Path.Value != "" {
		path = strings.ReplaceAll(fileIn.Path.Value, "//", "/")
		if path != "/" {
			path = strings.TrimSuffix(path, "/")
		}
	}

	if path != "" && fileIn.ParentId.Value == "" {
		parent, err = a.getFileFromPath(path, userId)
		if err != nil {
			return nil, &apiError{err: err, code: 404}
		}
		fileDB.ParentId = utils.Ptr(parent.ID)
	} else if fileIn.ParentId.Value != "" {
		fileDB.ParentId = utils.Ptr(fileIn.ParentId.Value)

	}

	if fileIn.Type == "folder" {
		fileDB.MimeType = "drive/folder"
		fileDB.Parts = nil
	} else if fileIn.Type == "file" {
		if fileIn.ChannelId.Value == 0 {
			channelId, err = getDefaultChannel(a.db, a.cache, userId)
			if err != nil {
				return nil, &apiError{err: err}
			}
		} else {
			channelId = fileIn.ChannelId.Value
		}
		fileDB.ChannelId = &channelId
		fileDB.MimeType = fileIn.MimeType.Value
		fileDB.Category = string(category.GetCategory(fileIn.Name))
		if len(fileIn.Parts) > 0 {
			fileDB.Parts = datatypes.NewJSONSlice(mapParts(fileIn.Parts))
		}
		fileDB.Size = utils.Ptr(fileIn.Size.Or(0))
	}
	fileDB.Name = fileIn.Name
	fileDB.Type = string(fileIn.Type)
	fileDB.UserId = userId
	fileDB.Status = "active"
	fileDB.Encrypted = fileIn.Encrypted.Value
	if fileIn.UpdatedAt.IsSet() && !fileIn.UpdatedAt.Value.IsZero() {
		fileDB.UpdatedAt = fileIn.UpdatedAt.Value
	} else {
		fileDB.UpdatedAt = time.Now().UTC()
	}
	if err := a.db.Create(&fileDB).Error; err != nil {
		if database.IsKeyConflictErr(err) {
			return nil, &apiError{err: errors.New("file already exists"), code: 409}
		}
		return nil, &apiError{err: err}
	}
	return mapper.ToFileOut(fileDB), nil
}

func (a *apiService) FilesCreateShare(ctx context.Context, req *api.FileShareCreate, params api.FilesCreateShareParams) error {
	userId := auth.GetUser(ctx)

	var fileShare models.FileShare

	if req.Password.Value != "" {
		bytes, err := bcrypt.GenerateFromPassword([]byte(req.Password.Value), bcrypt.MinCost)
		if err != nil {
			return &apiError{err: err}
		}
		fileShare.Password = utils.Ptr(string(bytes))
	}

	fileShare.FileId = params.ID
	if req.ExpiresAt.IsSet() {
		fileShare.ExpiresAt = utils.Ptr(req.ExpiresAt.Value)
	}
	fileShare.UserId = userId

	if err := a.db.Create(&fileShare).Error; err != nil {
		return &apiError{err: err}
	}

	return nil
}

func (a *apiService) FilesDelete(ctx context.Context, req *api.FileDelete) error {
	userId := auth.GetUser(ctx)
	if err := a.db.Exec("call teldrive.delete_files_bulk($1 , $2)", req.Ids, userId).Error; err != nil {
		return &apiError{err: err}
	}

	return nil
}

func (a *apiService) FilesDeleteShare(ctx context.Context, params api.FilesDeleteShareParams) error {
	userId := auth.GetUser(ctx)

	var deletedShare models.FileShare

	if err := a.db.Clauses(clause.Returning{}).Where("file_id = ?", params.ID).Where("user_id = ?", userId).
		Delete(&deletedShare).Error; err != nil {
		return &apiError{err: err}
	}
	if deletedShare.ID != "" {
		a.cache.Delete(cache.Key("shared", deletedShare.ID))
	}

	return nil
}

func (a *apiService) FilesEditShare(ctx context.Context, req *api.FileShareCreate, params api.FilesEditShareParams) error {
	userId := auth.GetUser(ctx)

	var fileShareUpdate models.FileShare

	if req.Password.Value != "" {
		bytes, err := bcrypt.GenerateFromPassword([]byte(req.Password.Value), bcrypt.MinCost)
		if err != nil {
			return &apiError{err: err}
		}
		fileShareUpdate.Password = utils.Ptr(string(bytes))
	}
	if req.ExpiresAt.IsSet() {
		fileShareUpdate.ExpiresAt = utils.Ptr(req.ExpiresAt.Value)
	}

	if err := a.db.Model(&models.FileShare{}).Where("file_id = ?", params.ID).Where("user_id = ?", userId).
		Updates(fileShareUpdate).Error; err != nil {
		return &apiError{err: err}
	}

	return nil
}

func (a *apiService) FilesGetById(ctx context.Context, params api.FilesGetByIdParams) (*api.File, error) {
	var result []fullFileDB
	if err := a.db.Model(&models.File{}).Select("*",
		"(select get_path_from_file_id as path from teldrive.get_path_from_file_id(id))").
		Where("id = ?", params.ID).Scan(&result).Error; err != nil {
		return nil, &apiError{err: err}
	}
	if len(result) == 0 {
		return nil, &apiError{err: errors.New("file not found"), code: 404}
	}
	res := mapper.ToFileOut(result[0].File)
	res.Path = api.NewOptString(result[0].Path)
	if result[0].ChannelId != nil {
		res.ChannelId = api.NewOptInt64(*result[0].ChannelId)
	}

	return res, nil
}

func (a *apiService) FilesList(ctx context.Context, params api.FilesListParams) (*api.FileList, error) {
	userId := auth.GetUser(ctx)

	queryBuilder := &fileQueryBuilder{db: a.db}

	return queryBuilder.execute(&params, userId)
}

func (a *apiService) FilesMkdir(ctx context.Context, req *api.FileMkDir) error {
	userId := auth.GetUser(ctx)

	if err := a.db.Exec("select * from teldrive.create_directories(?, ?)", userId, req.Path).Error; err != nil {
		return &apiError{err: err}
	}
	return nil
}

func (a *apiService) FilesMove(ctx context.Context, req *api.FileMove) error {
	userId := auth.GetUser(ctx)
	items := pgtype.Array[string]{
		Elements: req.Ids,
		Valid:    true,
		Dims:     []pgtype.ArrayDimension{{Length: int32(len(req.Ids)), LowerBound: 1}},
	}
	if !isUUId(req.Destination) {
		r, err := a.getFileFromPath(req.Destination, userId)
		if err != nil {
			return &apiError{err: err}
		}
		req.Destination = r.ID
	}
	if err := a.db.Model(&models.File{}).Where("id = any(?)", items).Where("user_id = ?", userId).
		Update("parent_id", req.Destination).Error; err != nil {
		return &apiError{err: err}
	}

	return nil

}

func (a *apiService) FilesShareByid(ctx context.Context, params api.FilesShareByidParams) (*api.FileShare, error) {
	userId := auth.GetUser(ctx)
	var result []models.FileShare

	notFoundErr := &apiError{err: errors.New("invalid share"), code: 404}
	if err := a.db.Model(&models.FileShare{}).Where("file_id = ?", params.ID).Where("user_id = ?", userId).
		Find(&result).Error; err != nil {
		if database.IsRecordNotFoundErr(err) {
			return nil, notFoundErr
		}
		return nil, &apiError{err: err}
	}

	if len(result) == 0 {
		return nil, notFoundErr
	}
	res := &api.FileShare{
		ID: result[0].ID,
	}
	if result[0].Password != nil {
		res.Protected = true
	}
	if result[0].ExpiresAt != nil {
		res.ExpiresAt = api.NewOptDateTime(*result[0].ExpiresAt)
	}
	return res, nil
}

func (a *apiService) FilesStream(ctx context.Context, params api.FilesStreamParams) (api.FilesStreamRes, error) {
	return nil, nil
}

func (a *apiService) FilesUpdate(ctx context.Context, req *api.FileUpdate, params api.FilesUpdateParams) (*api.File, error) {

	updateDb := models.File{}
	if req.Name.Value != "" {
		updateDb.Name = req.Name.Value
	}
	if len(req.Parts) > 0 {
		updateDb.Parts = datatypes.NewJSONSlice(mapParts(req.Parts))
	}
	if req.Size.Value != 0 {
		updateDb.Size = utils.Ptr(req.Size.Value)
	}
	if req.UpdatedAt.IsSet() && !req.UpdatedAt.Value.IsZero() {
		updateDb.UpdatedAt = req.UpdatedAt.Value
	} else if !req.UpdatedAt.IsSet() && params.Skiputs.Value == "0" {
		updateDb.UpdatedAt = time.Now().UTC()
	}

	if err := a.db.Model(&models.File{}).Where("id = ?", params.ID).Updates(updateDb).Error; err != nil {
		return nil, &apiError{err: err}
	}

	a.cache.Delete(cache.Key("files", params.ID))

	file := models.File{}
	if err := a.db.Where("id = ?", params.ID).First(&file).Error; err != nil {
		return nil, &apiError{err: err}
	}
	return mapper.ToFileOut(file), nil
}

func (a *apiService) FilesUpdateParts(ctx context.Context, req *api.FilePartsUpdate, params api.FilesUpdatePartsParams) error {

	userId := auth.GetUser(ctx)

	var file models.File

	updatePayload := models.File{
		Size: utils.Ptr(req.Size),
	}
	if req.ChannelId.Value == 0 {
		channelId, err := getDefaultChannel(a.db, a.cache, userId)
		if err != nil {
			return &apiError{err: err}
		}
		updatePayload.ChannelId = &channelId
	} else {
		updatePayload.ChannelId = &req.ChannelId.Value
	}
	if len(req.Parts) > 0 {
		updatePayload.Parts = datatypes.NewJSONSlice(mapParts(req.Parts))
	}
	if req.Name.Value != "" {
		updatePayload.Name = req.Name.Value
	}
	if req.ParentId.Value != "" {
		updatePayload.ParentId = utils.Ptr(req.ParentId.Value)
	}

	updatePayload.UpdatedAt = req.UpdatedAt

	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", params.ID).First(&file).Error; err != nil {
			return err
		}
		if err := tx.Model(models.File{}).Where("id = ?", params.ID).Updates(updatePayload).Error; err != nil {
			return err
		}
		if req.UploadId.Value != "" {
			if err := tx.Where("upload_id = ?", req.UploadId.Value).Delete(&models.Upload{}).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return &apiError{err: err}
	}

	keys := []string{cache.Key("files", params.ID)}
	if len(file.Parts) > 0 && file.ChannelId != nil {
		ids := utils.Map(file.Parts, func(part api.Part) int { return part.ID })
		client, _ := tgc.AuthClient(ctx, &a.cnf.TG, auth.GetJWTUser(ctx).TgSession, a.middlewares...)
		tgc.DeleteMessages(ctx, client, *file.ChannelId, ids)
		keys = append(keys, cache.Key("files", "messages", params.ID))
		for _, part := range file.Parts {
			keys = append(keys, cache.Key("files", "location", params.ID, part.ID))
		}

	}
	a.cache.Delete(keys...)

	return nil
}

func (e *extendedService) FilesStream(w http.ResponseWriter, r *http.Request, fileId string, userId int64) {
	ctx := r.Context()
	var (
		session *models.Session
		err     error
		user    *types.JWTClaims
	)
	if userId == 0 {

		authHash := r.URL.Query().Get("hash")
		if authHash == "" {
			cookie, err := r.Cookie(authCookieName)
			if err != nil {
				http.Error(w, "missing token or authash", http.StatusUnauthorized)
				return
			}
			user, err = auth.VerifyUser(e.api.db, e.api.cache, e.api.cnf.JWT.Secret, cookie.Value)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
			}
			userId, _ := strconv.ParseInt(user.Subject, 10, 64)
			session = &models.Session{UserId: userId, Session: user.TgSession}
		} else {
			session, err = auth.GetSessionByHash(e.api.db, e.api.cache, authHash)
			if err != nil {
				http.Error(w, "invalid hash", http.StatusBadRequest)
				return
			}
		}
	} else {
		session = &models.Session{UserId: userId}
	}

	file, err := cache.Fetch(e.api.cache, cache.Key("files", fileId), 0, func() (*models.File, error) {
		var result models.File
		if err := e.api.db.Model(&result).Where("id = ?", fileId).First(&result).Error; err != nil {
			return nil, err
		}
		return &result, nil
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Accept-Ranges", "bytes")

	var start, end int64

	rangeHeader := r.Header.Get("Range")
	contentType := defaultContentType

	if file.MimeType != "" {
		contentType = file.MimeType
	}

	if file.Size == nil || *file.Size == 0 {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", "0")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": file.Name}))
		w.WriteHeader(http.StatusOK)
		return
	}

	status := http.StatusOK
	if rangeHeader == "" {
		start = 0
		end = *file.Size - 1
	} else {
		ranges, err := http_range.Parse(rangeHeader, *file.Size)
		if err == http_range.ErrNoOverlap {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", *file.Size))
			http.Error(w, http_range.ErrNoOverlap.Error(), http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(ranges) > 1 {
			http.Error(w, "multiple ranges are not supported", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		start = ranges[0].Start
		end = ranges[0].End
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, *file.Size))
		status = http.StatusPartialContent

	}

	contentLength := end - start + 1

	w.Header().Set("Content-Type", contentType)

	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	w.Header().Set("E-Tag", fmt.Sprintf("\"%s\"", md5.FromString(fileId+strconv.FormatInt(*file.Size, 10))))
	w.Header().Set("Last-Modified", file.UpdatedAt.UTC().Format(http.TimeFormat))

	disposition := "inline"

	download := r.URL.Query().Get("download") == "1"

	if download {
		disposition = "attachment"
	}

	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": file.Name}))

	w.WriteHeader(status)

	if r.Method == "HEAD" {
		return
	}

	tokens, err := getBotsToken(e.api.db, e.api.cache, session.UserId, *file.ChannelId)

	if err != nil {
		http.Error(w, "failed to get bots", http.StatusInternalServerError)
		return
	}

	var (
		lr           io.ReadCloser
		client       *telegram.Client
		multiThreads int
		token        string
	)

	multiThreads = e.api.cnf.TG.Stream.MultiThreads
	middlewares := tgc.NewMiddleware(&e.api.cnf.TG, tgc.WithFloodWait(),
		tgc.WithRecovery(ctx),
		tgc.WithRetry(5),
		tgc.WithRateLimit())
	if e.api.cnf.TG.DisableStreamBots || len(tokens) == 0 {
		client, err = tgc.AuthClient(ctx, &e.api.cnf.TG, session.Session, middlewares...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		multiThreads = 0

	} else {
		e.api.worker.Set(tokens, *file.ChannelId)

		token, _ = e.api.worker.Next(*file.ChannelId)

		client, err = tgc.BotClient(ctx, e.api.boltdb, &e.api.cnf.TG, token, middlewares...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if download {
		multiThreads = 0
	}

	if r.Method != "HEAD" {
		handleStream := func() error {
			parts, err := getParts(ctx, client, e.api.cache, file)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return nil
			}
			lr, err = reader.NewLinearReader(ctx, client.API(), e.api.cache, file, parts, start, end, &e.api.cnf.TG, multiThreads)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return nil
			}
			if lr == nil {
				http.Error(w, "failed to initialise reader", http.StatusInternalServerError)
				return nil
			}

			_, err = io.CopyN(w, lr, contentLength)
			if err != nil {
				lr.Close()
			}
			return nil
		}
		tgc.RunWithAuth(ctx, client, token, func(ctx context.Context) error {
			return handleStream()
		})

	}
}

func (e *extendedService) SharesStream(w http.ResponseWriter, r *http.Request, shareId, fileId string) {
	share, err := e.api.validFileShare(r, shareId)
	if err != nil && errors.Is(err, ErrEmptyAuth) {
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	e.FilesStream(w, r, fileId, share.UserId)
}

func mapParts(_parts []api.Part) []api.Part {
	return utils.Map(_parts, func(part api.Part) api.Part {
		p := api.Part{ID: part.ID}
		if part.Salt.Value != "" {
			p.Salt = part.Salt
		}
		return p
	})

}
