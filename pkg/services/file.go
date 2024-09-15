package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/WinterYukky/gorm-extra-clause-plugin/exclause"
	"github.com/gin-gonic/gin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/category"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/database"
	"github.com/tgdrive/teldrive/internal/http_range"
	"github.com/tgdrive/teldrive/internal/kv"
	"github.com/tgdrive/teldrive/internal/md5"
	"github.com/tgdrive/teldrive/internal/reader"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/mapper"
	"github.com/tgdrive/teldrive/pkg/models"
	"github.com/tgdrive/teldrive/pkg/schemas"
	"github.com/tgdrive/teldrive/pkg/types"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrorStreamAbandoned = errors.New("stream abandoned")
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

type FileService struct {
	db        *gorm.DB
	cnf       *config.Config
	botWorker *tgc.BotWorker
	cache     cache.Cacher
	kv        kv.KV
	logger    *zap.SugaredLogger
}

func NewFileService(
	db *gorm.DB,
	cnf *config.Config,
	worker *tgc.StreamWorker,
	botWorker *tgc.BotWorker,
	kv kv.KV,
	cache cache.Cacher,
	logger *zap.SugaredLogger) *FileService {
	return &FileService{db: db, cnf: cnf, botWorker: botWorker, cache: cache, kv: kv, logger: logger}
}

func (fs *FileService) CreateFile(c *gin.Context, userId int64, fileIn *schemas.FileIn) (*schemas.FileOut, *types.AppError) {

	var (
		fileDB models.File
		parent *models.File
		err    error
	)

	fileIn.Path = strings.TrimSpace(fileIn.Path)

	if fileIn.Path != "" && fileIn.ParentID == "" {
		parent, err = fs.getFileFromPath(fileIn.Path, userId)
		if err != nil {
			return nil, &types.AppError{Error: err, Code: http.StatusNotFound}
		}
		fileDB.ParentID = sql.NullString{
			String: parent.Id,
			Valid:  true,
		}
	} else if fileIn.ParentID != "" {
		fileDB.ParentID = sql.NullString{
			String: fileIn.ParentID,
			Valid:  true,
		}

	} else {
		return nil, &types.AppError{Error: fmt.Errorf("parent id or path is required"), Code: http.StatusBadRequest}
	}

	if fileIn.Type == "folder" {
		fileDB.MimeType = "drive/folder"
		fileDB.Parts = nil
	} else if fileIn.Type == "file" {
		channelId := fileIn.ChannelID
		if fileIn.ChannelID == 0 {
			var err error
			channelId, err = getDefaultChannel(fs.db, fs.cache, userId)
			if err != nil {
				return nil, &types.AppError{Error: err, Code: http.StatusNotFound}
			}
		}
		fileDB.ChannelID = &channelId
		fileDB.MimeType = fileIn.MimeType
		fileDB.Category = string(category.GetCategory(fileIn.Name))
		fileDB.Parts = datatypes.NewJSONSlice(fileIn.Parts)
		fileDB.Size = &fileIn.Size
	}
	fileDB.Name = fileIn.Name
	fileDB.Type = fileIn.Type
	fileDB.UserID = userId
	fileDB.Status = "active"
	fileDB.Encrypted = fileIn.Encrypted

	if err := fs.db.Create(&fileDB).Error; err != nil {
		if database.IsKeyConflictErr(err) {
			return nil, &types.AppError{Error: database.ErrKeyConflict, Code: http.StatusConflict}
		}
		return nil, &types.AppError{Error: err}
	}

	res := mapper.ToFileOut(fileDB)

	return res, nil
}

func (fs *FileService) UpdateFile(id string, userId int64, update *schemas.FileUpdate) (*schemas.FileOut, *types.AppError) {
	var (
		files []models.File
		chain *gorm.DB
	)

	updateDb := models.File{
		Name:      update.Name,
		UpdatedAt: update.UpdatedAt,
		Size:      update.Size,
	}

	if len(update.Parts) > 0 {
		updateDb.Parts = datatypes.NewJSONSlice(update.Parts)
	}
	chain = fs.db.Model(&files).Clauses(clause.Returning{}).Where("id = ?", id).Updates(updateDb)

	if chain.Error != nil {
		return nil, &types.AppError{Error: chain.Error}
	}
	if chain.RowsAffected == 0 {
		return nil, &types.AppError{Error: database.ErrNotFound, Code: http.StatusNotFound}
	}

	fs.cache.Delete(fmt.Sprintf("files:%s", id))

	return mapper.ToFileOut(files[0]), nil

}

func (fs *FileService) GetFileByID(id string) (*schemas.FileOutFull, *types.AppError) {
	var file models.File
	if err := fs.db.Where("id = ?", id).First(&file).Error; err != nil {
		if database.IsRecordNotFoundErr(err) {
			return nil, &types.AppError{Error: database.ErrNotFound, Code: http.StatusNotFound}
		}
		return nil, &types.AppError{Error: err}
	}

	return mapper.ToFileOutFull(file), nil
}

func (fs *FileService) ListFiles(userId int64, fquery *schemas.FileQuery) (*schemas.FileResponse, *types.AppError) {

	query := fs.db.Where("user_id = ?", userId).Where("status = ?", "active")

	if fquery.Op == "list" {
		if fquery.Path != "" && fquery.ParentID == "" {
			query.Where("parent_id in (SELECT id FROM teldrive.get_file_from_path(?, ?, ?))", fquery.Path, userId, true)
		}
		if fquery.ParentID != "" {
			query.Where("parent_id = ?", fquery.ParentID)
		}
	} else if fquery.Op == "find" {
		if fquery.DeepSearch && fquery.Query != "" && fquery.Path != "" {
			query.Where("files.id in (select id  from subdirs)")
		}
		if fquery.UpdatedAt != "" {
			dateFilters := strings.Split(fquery.UpdatedAt, ",")
			for _, dateFilter := range dateFilters {
				parts := strings.Split(dateFilter, ":")
				if len(parts) == 2 {
					op, date := parts[0], parts[1]
					t, err := time.Parse(time.DateOnly, date)
					if err != nil {
						return nil, &types.AppError{Error: err}
					}
					formattedDate := t.Format(time.RFC3339)
					switch op {
					case "gte":
						query.Where("updated_at >= ?", formattedDate)
					case "lte":
						query.Where("updated_at <= ?", formattedDate)
					case "eq":
						query.Where("updated_at = ?", formattedDate)
					case "gt":
						query.Where("updated_at > ?", formattedDate)
					case "lt":
						query.Where("updated_at < ?", formattedDate)
					}
				}
			}
		}

		if fquery.Query != "" {
			query = query.Where("name &@~ REGEXP_REPLACE(?, '[.,-_]', ' ', 'g')", fquery.Query)
		}

		if fquery.Category != "" {
			categories := strings.Split(fquery.Category, ",")
			var filterQuery *gorm.DB
			if categories[0] == "folder" {
				filterQuery = fs.db.Where("type = ?", categories[0])
			} else {
				filterQuery = fs.db.Where("category = ?", categories[0])
			}
			if len(categories) > 1 {
				for _, category := range categories[1:] {
					if category == "folder" {
						filterQuery.Or("type = ?", category)
					} else {
						filterQuery.Or("category = ?", category)
					}
				}
			}
			query.Where(filterQuery)
		}

		if fquery.Name != "" {
			query.Where("name = ?", fquery.Name)
		}
		if fquery.ParentID != "" {
			query.Where("parent_id = ?", fquery.ParentID)
		}
		if fquery.ParentID == "" && fquery.Path != "" && fquery.Query == "" {
			query.Where("parent_id in (SELECT id FROM teldrive.get_file_from_path(?, ?, ?))", fquery.Path, userId, true)
		}
		if fquery.Type != "" {
			query.Where("type = ?", fquery.Type)
		}

		if fquery.Shared != nil && *fquery.Shared {
			query.Where("id in (SELECT file_id FROM teldrive.file_shares where user_id = ?)", userId)
		}
	}

	orderField := utils.CamelToSnake(fquery.Sort)

	var op string

	if fquery.Page == 1 {
		if fquery.Order == "asc" {
			op = ">="
		} else {
			op = "<="
		}
	} else {
		if fquery.Order == "asc" {
			op = ">"
		} else {
			op = "<"
		}

	}

	var fileQuery *gorm.DB

	if fquery.DeepSearch && fquery.Query != "" && fquery.Path != "" {
		fileQuery = fs.db.Clauses(exclause.With{Recursive: true, CTEs: []exclause.CTE{{Name: "subdirs",
			Subquery: exclause.Subquery{DB: fs.db.Model(&models.File{}).Select("id", "parent_id").
				Where("id in (SELECT id FROM teldrive.get_file_from_path(?, ?, ?))", fquery.Path, userId, true).
				Clauses(exclause.NewUnion("ALL ?",
					fs.db.Table("teldrive.files as f").Select("f.id", "f.parent_id").
						Joins("inner join subdirs ON f.parent_id = subdirs.id")))}}}})
	}

	if fileQuery == nil {
		fileQuery = fs.db
	}

	fileQuery = fileQuery.Clauses(exclause.NewWith("ranked_scores", fs.db.Model(&models.File{}).Select(orderField, "count(*) OVER () as total",
		fmt.Sprintf("ROW_NUMBER() OVER (ORDER BY %s %s) AS rank", orderField, strings.ToUpper(fquery.Order))).Where(query))).
		Model(&models.File{}).Select("*", "(select total from ranked_scores limit 1) as total").
		Where(fmt.Sprintf("%s %s (SELECT %s FROM ranked_scores WHERE rank = ?)", orderField, op, orderField),
			max((fquery.Page-1)*fquery.Limit, 1)).
		Where(query).Order(getOrder(fquery)).Limit(fquery.Limit)

	files := []schemas.FileOut{}

	if err := fileQuery.Scan(&files).Error; err != nil {
		if strings.Contains(err.Error(), "file not found") {
			return nil, &types.AppError{Error: database.ErrNotFound, Code: http.StatusNotFound}
		}
		return nil, &types.AppError{Error: err}
	}

	count := 0

	if len(files) > 0 {
		count = files[0].Total
	}

	for i := range files {
		files[i].Total = 0
	}

	res := &schemas.FileResponse{Files: files,
		Meta: schemas.Meta{Count: count, TotalPages: int(math.Ceil(float64(count) / float64(fquery.Limit))),
			CurrentPage: fquery.Page}}

	return res, nil
}

func (fs *FileService) getFileFromPath(path string, userId int64) (*models.File, error) {

	var res []models.File

	if err := fs.db.Raw("select * from teldrive.get_file_from_path(?, ?, ?)", path, userId, true).
		Scan(&res).Error; err != nil {
		return nil, err

	}
	if len(res) == 0 {
		return nil, database.ErrNotFound
	}
	return &res[0], nil
}

func (fs *FileService) MakeDirectory(userId int64, payload *schemas.MkDir) (*schemas.FileOut, *types.AppError) {
	var files []models.File

	if err := fs.db.Raw("select * from teldrive.create_directories(?, ?)", userId, payload.Path).
		Scan(&files).Error; err != nil {
		return nil, &types.AppError{Error: err}
	}

	file := mapper.ToFileOut(files[0])

	return file, nil
}

func (fs *FileService) MoveFiles(userId int64, payload *schemas.FileOperation) (*schemas.Message, *types.AppError) {

	if err := fs.db.Exec("select * from teldrive.move_items($1 , $2 , $3)", payload.Files, payload.Destination, userId).Error; err != nil {
		return nil, &types.AppError{Error: err}
	}

	return &schemas.Message{Message: "files moved"}, nil
}

func (fs *FileService) DeleteFiles(userId int64, payload *schemas.DeleteOperation) (*schemas.Message, *types.AppError) {

	if payload.Source != "" {
		if err := fs.db.Exec("call teldrive.delete_folder_recursive($1 , $2)", payload.Source, userId).Error; err != nil {
			return nil, &types.AppError{Error: err}
		}
	} else if payload.Source == "" && len(payload.Files) > 0 {
		if err := fs.db.Exec("call teldrive.delete_files_bulk($1 , $2)", payload.Files, userId).Error; err != nil {
			return nil, &types.AppError{Error: err}
		}

	}

	return &schemas.Message{Message: "files deleted"}, nil
}

func (fs *FileService) CreateShare(fileId string, userId int64, payload *schemas.FileShareIn) *types.AppError {

	var fileShare models.FileShare

	if payload.Password != "" {
		bytes, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.MinCost)
		if err != nil {
			return &types.AppError{Error: err}
		}
		fileShare.Password = utils.StringPointer(string(bytes))
	}

	fileShare.FileID = fileId
	fileShare.ExpiresAt = payload.ExpiresAt
	fileShare.UserID = userId

	if err := fs.db.Create(&fileShare).Error; err != nil {
		return &types.AppError{Error: err}
	}

	return nil
}

func (fs *FileService) UpdateShare(fileId string, userId int64, payload *schemas.FileShareIn) *types.AppError {

	var fileShareUpdate models.FileShare

	if payload.Password != "" {
		bytes, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.MinCost)
		if err != nil {
			return &types.AppError{Error: err}
		}
		fileShareUpdate.Password = utils.StringPointer(string(bytes))
	}

	fileShareUpdate.ExpiresAt = payload.ExpiresAt

	if err := fs.db.Model(&models.FileShare{}).Where("file_id = ?", fileId).Where("user_id = ?", userId).
		Updates(fileShareUpdate).Error; err != nil {
		return &types.AppError{Error: err}
	}

	return nil
}

func (fs *FileService) GetShareByFileId(fileId string, userId int64) (*schemas.FileShareOut, *types.AppError) {

	var result []models.FileShare

	if err := fs.db.Model(&models.FileShare{}).Where("file_id = ?", fileId).Where("user_id = ?", userId).
		Find(&result).Error; err != nil {
		return nil, &types.AppError{Error: err}
	}

	if len(result) == 0 {
		return nil, nil
	}

	res := &schemas.FileShareOut{ID: result[0].ID, ExpiresAt: result[0].ExpiresAt, Protected: result[0].Password != nil}

	return res, nil
}

func (fs *FileService) DeleteShare(fileId string, userId int64) *types.AppError {

	var deletedShare models.FileShare

	if err := fs.db.Clauses(clause.Returning{}).Where("file_id = ?", fileId).Where("user_id = ?", userId).
		Delete(&deletedShare).Error; err != nil {
		return &types.AppError{Error: err}
	}

	if deletedShare.ID != "" {
		fs.cache.Delete(fmt.Sprintf("shares:%s", deletedShare.ID))
	}

	return nil
}

func (fs *FileService) UpdateParts(c *gin.Context, id string, userId int64, payload *schemas.PartUpdate) (*schemas.Message, *types.AppError) {

	var file models.File

	updatePayload := models.File{
		UpdatedAt: payload.UpdatedAt,
		Size:      utils.Int64Pointer(payload.Size),
	}

	if len(payload.Parts) > 0 {
		updatePayload.Parts = datatypes.NewJSONSlice(payload.Parts)
	}

	err := fs.db.Transaction(func(tx *gorm.DB) error {

		if err := tx.Where("id = ?", id).First(&file).Error; err != nil {
			return err
		}

		if err := tx.Model(models.File{}).Where("id = ?", id).Updates(updatePayload).Error; err != nil {
			return err
		}

		if payload.UploadId != "" {
			if err := tx.Where("upload_id = ?", payload.UploadId).Delete(&models.Upload{}).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, &types.AppError{Error: err}
	}

	if len(file.Parts) > 0 && file.ChannelID != nil {
		_, session := auth.GetUser(c)
		ids := []int{}
		for _, part := range file.Parts {
			ids = append(ids, int(part.ID))
		}
		client, _ := tgc.AuthClient(c, &fs.cnf.TG, session)
		tgc.DeleteMessages(c, client, *file.ChannelID, ids)
		keys := []string{fmt.Sprintf("files:%s", id), fmt.Sprintf("files:messages:%s:%d", id, userId)}
		for _, part := range file.Parts {
			keys = append(keys, fmt.Sprintf("files:location:%d:%s:%d", userId, id, part.ID))

		}
		fs.cache.Delete(keys...)

	}
	fs.cache.Delete(fmt.Sprintf("files:%s", id))

	return &schemas.Message{Message: "file updated"}, nil
}

func (fs *FileService) MoveDirectory(userId int64, payload *schemas.DirMove) (*schemas.Message, *types.AppError) {

	if err := fs.db.Exec("select * from teldrive.move_directory(? , ? , ?)", payload.Source,
		payload.Destination, userId).Error; err != nil {
		return nil, &types.AppError{Error: err}
	}

	return &schemas.Message{Message: "directory moved"}, nil
}

func (fs *FileService) GetCategoryStats(userId int64) ([]schemas.FileCategoryStats, *types.AppError) {

	var stats []schemas.FileCategoryStats

	if err := fs.db.Model(&models.File{}).Select("category", "COUNT(*) as total_files", "coalesce(SUM(size),0) as total_size").
		Where(&models.File{UserID: userId, Type: "file", Status: "active"}).
		Order("category ASC").Group("category").Find(&stats).Error; err != nil {
		return nil, &types.AppError{Error: err}
	}

	return stats, nil
}

func (fs *FileService) CopyFile(c *gin.Context) (*schemas.FileOut, *types.AppError) {

	var payload schemas.Copy

	if err := c.ShouldBindJSON(&payload); err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusBadRequest}
	}

	userId, session := auth.GetUser(c)

	client, _ := tgc.AuthClient(c, &fs.cnf.TG, session)

	var res []models.File

	if err := fs.db.Model(&models.File{}).Where("id = ?", payload.ID).Find(&res).Error; err != nil {
		return nil, &types.AppError{Error: err}
	}

	file := mapper.ToFileOutFull(res[0])

	newIds := []schemas.Part{}

	channelId, err := getDefaultChannel(fs.db, fs.cache, userId)
	if err != nil {
		return nil, &types.AppError{Error: err}
	}

	err = tgc.RunWithAuth(c, client, "", func(ctx context.Context) error {
		ids := []int{}

		for _, part := range file.Parts {
			ids = append(ids, int(part.ID))
		}
		messages, err := tgc.GetMessages(c, client.API(), ids, file.ChannelID)

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
			res, err := client.API().MessagesSendMedia(c, &request)

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
			newIds = append(newIds, schemas.Part{ID: int64(msg.ID), Salt: file.Parts[i].Salt})

		}
		return nil
	})

	if err != nil {
		return nil, &types.AppError{Error: err}
	}

	var destRes []models.File

	if err := fs.db.Raw("select * from teldrive.create_directories(?, ?)", userId, payload.Destination).Scan(&destRes).Error; err != nil {
		return nil, &types.AppError{Error: err}
	}

	dest := destRes[0]

	dbFile := models.File{}

	dbFile.Name = payload.Name
	dbFile.Size = &file.Size
	dbFile.Type = file.Type
	dbFile.MimeType = file.MimeType
	dbFile.Parts = datatypes.NewJSONSlice(newIds)
	dbFile.UserID = userId
	dbFile.Status = "active"
	dbFile.ParentID = sql.NullString{
		String: dest.Id,
		Valid:  true,
	}
	dbFile.ChannelID = &channelId
	dbFile.Encrypted = file.Encrypted
	dbFile.Category = file.Category

	if err := fs.db.Create(&dbFile).Error; err != nil {
		return nil, &types.AppError{Error: err}
	}

	return mapper.ToFileOut(dbFile), nil
}

func (fs *FileService) GetFileStream(c *gin.Context, download bool, sharedFile *schemas.FileShareOut) {

	w := c.Writer

	r := c.Request

	fileID := c.Param("fileID")

	var (
		session *models.Session
		err     error
		appErr  *types.AppError
		user    *types.JWTClaims
	)

	if sharedFile == nil {
		authHash := c.Query("hash")

		if authHash == "" {
			user, err = auth.VerifyUser(c, fs.db, fs.cache, fs.cnf.JWT.Secret)
			if err != nil {
				http.Error(w, "missing session or authash", http.StatusUnauthorized)
				return
			}
			userId, _ := strconv.ParseInt(user.Subject, 10, 64)
			session = &models.Session{UserId: userId, Session: user.TgSession}
		} else {
			session, err = auth.GetSessionByHash(fs.db, fs.cache, authHash)
			if err != nil {
				http.Error(w, "invalid hash", http.StatusBadRequest)
				return
			}
		}

	} else {

		session = &models.Session{UserId: sharedFile.UserID}
	}

	file := &schemas.FileOutFull{}

	key := fmt.Sprintf("files:%s", fileID)

	err = fs.cache.Get(key, file)

	if err != nil {
		file, appErr = fs.GetFileByID(fileID)
		if appErr != nil {
			http.Error(w, appErr.Error.Error(), http.StatusBadRequest)
			return
		}
		fs.cache.Set(key, file, 0)
	}

	c.Header("Accept-Ranges", "bytes")

	var start, end int64

	rangeHeader := r.Header.Get("Range")

	if file.Size == 0 {
		c.Header("Content-Type", file.MimeType)
		c.Header("Content-Length", "0")

		if rangeHeader != "" {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", file.Size))
			http.Error(w, "Requested Range Not Satisfiable", http.StatusRequestedRangeNotSatisfiable)
			return
		}

		c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": file.Name}))
		w.WriteHeader(http.StatusOK)
		return
	}

	if rangeHeader == "" {
		start = 0
		end = file.Size - 1
		w.WriteHeader(http.StatusOK)
	} else {
		ranges, err := http_range.Parse(rangeHeader, file.Size)
		if err == http_range.ErrNoOverlap {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", file.Size))
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
		c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, file.Size))

		w.WriteHeader(http.StatusPartialContent)
	}

	contentLength := end - start + 1

	mimeType := file.MimeType

	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	c.Header("Content-Type", mimeType)

	c.Header("Content-Length", strconv.FormatInt(contentLength, 10))
	c.Header("E-Tag", fmt.Sprintf("\"%s\"", md5.FromString(file.Id+strconv.FormatInt(file.Size, 10))))
	c.Header("Last-Modified", file.UpdatedAt.UTC().Format(http.TimeFormat))

	disposition := "inline"

	if download {
		disposition = "attachment"
	}

	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": file.Name}))

	tokens, err := getBotsToken(fs.db, fs.cache, session.UserId, file.ChannelID)

	if err != nil {
		fs.handleError(fmt.Errorf("failed to get bots: %w", err), w)
		return
	}

	var (
		lr           io.ReadCloser
		client       *telegram.Client
		multiThreads int
		token        string
	)

	multiThreads = fs.cnf.TG.Stream.MultiThreads

	if fs.cnf.TG.DisableStreamBots || len(tokens) == 0 {
		client, err = tgc.AuthClient(c, &fs.cnf.TG, session.Session)
		if err != nil {
			fs.handleError(err, w)
			return
		}
		multiThreads = 0

	} else {
		fs.botWorker.Set(tokens, file.ChannelID)

		token, _ = fs.botWorker.Next(file.ChannelID)

		middlewares := tgc.Middlewares(&fs.cnf.TG, 5)
		client, err = tgc.BotClient(c, fs.kv, &fs.cnf.TG, token, middlewares...)
		if err != nil {
			fs.handleError(err, w)
			return
		}
	}
	if download {
		multiThreads = 0
	}

	if r.Method != "HEAD" {
		handleStream := func() error {
			parts, err := getParts(c, client, fs.cache, file)
			if err != nil {
				fs.handleError(err, w)
				return nil
			}
			lr, err = reader.NewLinearReader(c, client.API(), fs.cache, file, parts, start, end, &fs.cnf.TG, multiThreads)

			if err != nil {
				fs.handleError(err, w)
				return nil
			}
			if lr == nil {
				fs.handleError(fmt.Errorf("failed to initialise reader"), w)
				return nil
			}
			_, err = io.CopyN(w, lr, contentLength)
			if err != nil {
				lr.Close()
			}
			return nil
		}
		tgc.RunWithAuth(c, client, token, func(ctx context.Context) error {
			return handleStream()
		})

	}
}

func (fs *FileService) handleError(err error, w http.ResponseWriter) {
	fs.logger.Error(err)
	http.Error(w, err.Error(), http.StatusInternalServerError)

}

func getOrder(fquery *schemas.FileQuery) clause.OrderByColumn {
	sortColumn := utils.CamelToSnake(fquery.Sort)

	return clause.OrderByColumn{Column: clause.Column{Name: sortColumn},
		Desc: fquery.Order == "desc"}
}
