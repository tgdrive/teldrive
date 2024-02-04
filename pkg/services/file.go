package services

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/divyam234/teldrive/config"
	"github.com/divyam234/teldrive/internal/cache"
	"github.com/divyam234/teldrive/internal/http_range"
	"github.com/divyam234/teldrive/internal/md5"
	"github.com/divyam234/teldrive/internal/reader"
	"github.com/divyam234/teldrive/internal/tgc"
	"github.com/divyam234/teldrive/internal/utils"
	"github.com/divyam234/teldrive/pkg/mapper"
	"github.com/divyam234/teldrive/pkg/models"
	"github.com/divyam234/teldrive/pkg/schemas"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/divyam234/teldrive/pkg/types"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mitchellh/mapstructure"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	updateFileContext    = "file update"
	getFileByIDContext   = "getting file by ID"
	listFilesContext     = "listing files"
	getPathIDContext     = "getting path ID"
	makeDirectoryContext = "making directory"
	copyFileContext      = "copying file"
	moveFilesContext     = "moving files"
	deleteFilesContext   = "deleting files"
	moveDirectoryContext = "moving directory"
	bindJSONContext      = "binding JSON"
	bindQueryContext     = "binding query"
)

type FileService struct {
	Db     *gorm.DB
	log    *zap.Logger
	worker *tgc.StreamWorker
}

func NewFileService(db *gorm.DB, logger *zap.Logger) *FileService {
	return &FileService{Db: db, log: logger.Named("files"),
		worker: &tgc.StreamWorker{}}
}

func (fs *FileService) logAndReturn(context string, err error, errCode int) *types.AppError {
	fs.log.Error(context, zap.Error(err))
	return &types.AppError{Error: err, Code: errCode}
}

func (fs *FileService) CreateFile(c *gin.Context) (*schemas.FileOut, *types.AppError) {
	userId, _ := getUserAuth(c)
	var fileIn schemas.CreateFile
	if err := c.ShouldBindJSON(&fileIn); err != nil {
		return nil, fs.logAndReturn(bindJSONContext, err, http.StatusBadRequest)
	}

	var fileDB models.File

	fileIn.Path = strings.TrimSpace(fileIn.Path)

	if fileIn.Path != "" {

		pathId, err := fs.getPathId(fileIn.Path, userId)
		if err != nil {
			return nil, fs.logAndReturn(listFilesContext, err, http.StatusNotFound)
		}
		fileDB.ParentID = pathId
	}

	if fileIn.Type == "folder" {
		fileDB.MimeType = "drive/folder"
		var fullPath string
		if fileIn.Path == "/" {
			fullPath = "/" + fileIn.Name
		} else {
			fullPath = fileIn.Path + "/" + fileIn.Name
		}
		fileDB.Path = fullPath
		fileDB.Depth = utils.IntPointer(len(strings.Split(fileIn.Path, "/")) - 1)
	} else if fileIn.Type == "file" {
		var err error
		fileDB.Path = ""
		channelId := fileIn.ChannelID
		if fileIn.ChannelID == 0 {
			channelId, err = GetDefaultChannel(c, userId)
			if err != nil {
				return nil, fs.logAndReturn("default channel", err, http.StatusInternalServerError)
			}
		}
		fileDB.ChannelID = utils.Int64Pointer(channelId)
		fileDB.MimeType = fileIn.MimeType
		parts := models.Parts{}
		for _, part := range fileIn.Parts {
			parts = append(parts, models.Part{
				ID:   part.ID,
				Salt: part.Salt,
			})

		}
		fileDB.Parts = &parts
		fileDB.Starred = false
		fileDB.Size = &fileIn.Size
	}
	fileDB.Name = fileIn.Name
	fileDB.Type = fileIn.Type
	fileDB.UserID = userId
	fileDB.Status = "active"
	fileDB.Encrypted = fileIn.Encrypted

	if err := fs.Db.Create(&fileDB).Error; err != nil {
		pgErr := err.(*pgconn.PgError)
		if pgErr.Code == "23505" {
			return nil, fs.logAndReturn("file exists", err, http.StatusInternalServerError)
		}
		return nil, fs.logAndReturn("file create", err, http.StatusInternalServerError)
	}

	res := mapper.ToFileOut(fileDB)

	return &res, nil
}

func (fs *FileService) UpdateFile(c *gin.Context) (*schemas.FileOut, *types.AppError) {

	fileID := c.Param("fileID")

	userId, _ := getUserAuth(c)

	var fileUpdate schemas.UpdateFile

	var files []models.File

	if err := c.ShouldBindJSON(&fileUpdate); err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusBadRequest}
	}

	if fileUpdate.Type == "folder" && fileUpdate.Name != "" {
		if err := fs.Db.Raw("select * from teldrive.update_folder(?, ?, ?)", fileID, fileUpdate.Name, userId).Scan(&files).Error; err != nil {
			return nil, fs.logAndReturn(updateFileContext, err, http.StatusInternalServerError)
		}
	} else {
		if err := fs.Db.Model(&files).Clauses(clause.Returning{}).Where("id = ?", fileID).Updates(fileUpdate).Error; err != nil {
			return nil, fs.logAndReturn(updateFileContext, err, http.StatusInternalServerError)
		}
	}

	if len(files) == 0 {
		return nil, fs.logAndReturn(updateFileContext, errors.New("update failed"), http.StatusInternalServerError)
	}

	file := mapper.ToFileOut(files[0])

	key := fmt.Sprintf("files:%s", fileID)

	cache.GetCache().Delete(key)

	return &file, nil

}

func (fs *FileService) GetFileByID(c *gin.Context) (*schemas.FileOutFull, *types.AppError) {
	fileID := c.Param("fileID")

	var file []models.File

	fs.Db.Model(&models.File{}).Where("id = ?", fileID).Find(&file)

	if len(file) == 0 {
		err := errors.New("file not found")
		return nil, fs.logAndReturn(getFileByIDContext, err, http.StatusNotFound)
	}

	return mapper.ToFileOutFull(file[0]), nil
}

func (fs *FileService) ListFiles(c *gin.Context) (*schemas.FileResponse, *types.AppError) {
	userId, _ := getUserAuth(c)

	var (
		pagingParams  schemas.PaginationQuery
		sortingParams schemas.SortingQuery
		fileQuery     schemas.FileQuery
	)
	pagingParams.PerPage = 200
	sortingParams.Order = "asc"
	sortingParams.Sort = "name"
	fileQuery.Op = "list"
	fileQuery.Status = "active"
	fileQuery.UserID = userId

	if err := c.ShouldBindQuery(&pagingParams); err != nil {
		return nil, fs.logAndReturn(listFilesContext, err, http.StatusBadRequest)
	}

	if err := c.ShouldBindQuery(&sortingParams); err != nil {
		return nil, fs.logAndReturn(listFilesContext, err, http.StatusBadRequest)
	}

	if err := c.ShouldBindQuery(&fileQuery); err != nil {
		return nil, fs.logAndReturn(listFilesContext, err, http.StatusBadRequest)
	}

	var (
		pathId string
		err    error
	)
	if fileQuery.Path != "" {
		pathId, err = fs.getPathId(fileQuery.Path, userId)
		if err != nil {
			return nil, fs.logAndReturn(listFilesContext, err, http.StatusNotFound)
		}
	}

	query := fs.Db.Model(&models.File{}).Limit(pagingParams.PerPage).
		Where(map[string]interface{}{"user_id": userId, "status": "active"})

	if fileQuery.Op == "list" {
		setOrderFilter(query, &pagingParams, &sortingParams)

		query.Order("type DESC").Order(getOrder(sortingParams)).
			Where("parent_id = ?", pathId)

	} else if fileQuery.Op == "find" {

		filterQuery := map[string]interface{}{}

		err := mapstructure.Decode(fileQuery, &filterQuery)

		if err != nil {
			return nil, fs.logAndReturn(listFilesContext, err, http.StatusBadRequest)
		}

		delete(filterQuery, "op")

		if filterQuery["updated_at"] == nil {
			delete(filterQuery, "updated_at")
		}

		if filterQuery["path"] != nil && filterQuery["name"] != nil {
			query.Where("parent_id = ?", pathId)
			delete(filterQuery, "path")
		}

		setOrderFilter(query, &pagingParams, &sortingParams)

		query.Order("type DESC").Order(getOrder(sortingParams)).Where(filterQuery)

	} else if fileQuery.Op == "search" {

		query.Where("teldrive.get_tsquery(?) @@ teldrive.get_tsvector(name)", fileQuery.Search)

		setOrderFilter(query, &pagingParams, &sortingParams)
		query.Order(getOrder(sortingParams))
	}

	if fileQuery.Path == "" {
		query.Select("*,(select path from teldrive.files as f where f.id = files.parent_id) as parent_path")
	}

	results := []schemas.FileOut{}

	query.Scan(&results)

	token := ""

	if len(results) == pagingParams.PerPage {
		lastItem := results[len(results)-1]
		token = utils.GetField(&lastItem, utils.CamelToPascalCase(sortingParams.Sort))
		token = base64.StdEncoding.EncodeToString([]byte(token))
	}

	res := &schemas.FileResponse{Results: results, NextPageToken: token}

	return res, nil
}

func (fs *FileService) getPathId(path string, userId int64) (string, error) {

	var file models.File

	if err := fs.Db.Model(&models.File{}).Select("id").Where("path = ?", path).Where("user_id = ?", userId).
		First(&file).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return "", errors.New("path not found")

	}
	return file.ID, nil
}

func (fs *FileService) MakeDirectory(c *gin.Context) (*schemas.FileOut, *types.AppError) {
	var payload schemas.MkDir
	var files []models.File

	if err := c.ShouldBindJSON(&payload); err != nil {
		return nil, fs.logAndReturn(makeDirectoryContext, err, http.StatusBadRequest)
	}

	userId, _ := getUserAuth(c)
	if err := fs.Db.Raw("select * from teldrive.create_directories(?, ?)", userId, payload.Path).
		Scan(&files).Error; err != nil {
		return nil, fs.logAndReturn(makeDirectoryContext, err, http.StatusInternalServerError)
	}

	file := mapper.ToFileOut(files[0])

	return &file, nil
}

func (fs *FileService) CopyFile(c *gin.Context) (*schemas.FileOut, *types.AppError) {
	var payload schemas.Copy

	if err := c.ShouldBindJSON(&payload); err != nil {
		return nil, fs.logAndReturn(copyFileContext, err, http.StatusBadRequest)
	}

	userId, session := getUserAuth(c)

	client, _ := tgc.UserLogin(c, session)

	var res []models.File

	fs.Db.Model(&models.File{}).Where("id = ?", payload.ID).Find(&res)

	file := mapper.ToFileOutFull(res[0])

	newIds := models.Parts{}

	err := tgc.RunWithAuth(c, fs.log, client, "", func(ctx context.Context) error {
		user := strconv.FormatInt(userId, 10)
		messages, err := getTGMessages(c, client, file.Parts, file.ChannelID, user)
		if err != nil {
			return err
		}

		channel, err := GetChannelById(c, client, file.ChannelID, user)
		if err != nil {
			fs.log.Error("channel", zap.Error(err))
			return err
		}
		for _, message := range messages.Messages {
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
			newIds = append(newIds, models.Part{ID: int64(msg.ID)})

		}
		return nil
	})

	if err != nil {
		return nil, fs.logAndReturn(copyFileContext, err, http.StatusBadRequest)
	}

	var destRes []models.File

	if err := fs.Db.Raw("select * from teldrive.create_directories(?, ?)", userId, payload.Destination).Scan(&destRes).Error; err != nil {
		return nil, fs.logAndReturn(copyFileContext, err, http.StatusInternalServerError)
	}

	dest := destRes[0]

	dbFile := models.File{}

	dbFile.Name = payload.Name
	dbFile.Size = &file.Size
	dbFile.Type = file.Type
	dbFile.MimeType = file.MimeType
	dbFile.Parts = &newIds
	dbFile.UserID = userId
	dbFile.Starred = false
	dbFile.Status = "active"
	dbFile.ParentID = dest.ID
	dbFile.ChannelID = &file.ChannelID
	dbFile.Encrypted = file.Encrypted

	if err := fs.Db.Create(&dbFile).Error; err != nil {
		return nil, fs.logAndReturn(copyFileContext, err, http.StatusInternalServerError)
	}

	out := mapper.ToFileOut(dbFile)

	return &out, nil
}

func (fs *FileService) MoveFiles(c *gin.Context) (*schemas.Message, *types.AppError) {
	var payload schemas.FileOperation

	if err := c.ShouldBindJSON(&payload); err != nil {
		return nil, fs.logAndReturn(moveFilesContext, err, http.StatusBadRequest)
	}
	userId, _ := getUserAuth(c)

	items := pgtype.Array[string]{
		Elements: payload.Files,
		Valid:    true,
		Dims:     []pgtype.ArrayDimension{{Length: 1, LowerBound: 1}},
	}

	if err := fs.Db.Exec("select * from teldrive.move_items(? , ? , ?)", items, payload.Destination, userId).Error; err != nil {
		return nil, fs.logAndReturn(moveDirectoryContext, err, http.StatusInternalServerError)
	}

	return &schemas.Message{Message: "files moved"}, nil
}

func (fs *FileService) DeleteFiles(c *gin.Context) (*schemas.Message, *types.AppError) {
	var payload schemas.FileOperation

	if err := c.ShouldBindJSON(&payload); err != nil {
		return nil, fs.logAndReturn(deleteFilesContext, err, http.StatusBadRequest)
	}

	if err := fs.Db.Exec("call teldrive.delete_files($1)", payload.Files).Error; err != nil {
		return nil, fs.logAndReturn(deleteFilesContext, err, http.StatusInternalServerError)
	}

	return &schemas.Message{Message: "files deleted"}, nil
}

func (fs *FileService) MoveDirectory(c *gin.Context) (*schemas.Message, *types.AppError) {
	var payload schemas.DirMove

	if err := c.ShouldBindJSON(&payload); err != nil {
		return nil, fs.logAndReturn(moveDirectoryContext, err, http.StatusBadRequest)
	}

	userId, _ := getUserAuth(c)

	if err := fs.Db.Exec("select * from teldrive.move_directory(? , ? , ?)", payload.Source, payload.Destination, userId).Error; err != nil {
		return nil, fs.logAndReturn(moveDirectoryContext, err, http.StatusInternalServerError)
	}

	return &schemas.Message{Message: "directory moved"}, nil
}

func (fs *FileService) GetFileStream(c *gin.Context) {
	w := c.Writer
	r := c.Request

	fileID := c.Param("fileID")

	authHash := c.Query("hash")

	if authHash == "" {
		http.Error(w, "missing hash param", http.StatusBadRequest)
		return
	}

	session, err := getSessionByHash(authHash)

	if err != nil {
		http.Error(w, "invalid hash", http.StatusBadRequest)
		return
	}

	file := &schemas.FileOutFull{}

	key := fmt.Sprintf("files:%s", fileID)

	err = cache.GetCache().Get(key, file)
	var appErr *types.AppError

	if err != nil {
		file, appErr = fs.GetFileByID(c)
		if appErr != nil {
			http.Error(w, appErr.Error.Error(), http.StatusBadRequest)
			return
		}
		cache.GetCache().Set(key, file, 0)
	}

	c.Header("Accept-Ranges", "bytes")

	var start, end int64

	rangeHeader := r.Header.Get("Range")

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
	c.Header("E-Tag", fmt.Sprintf("\"%s\"", md5.FromString(file.ID+strconv.FormatInt(file.Size, 10))))
	c.Header("Last-Modified", file.UpdatedAt.UTC().Format(http.TimeFormat))

	disposition := "inline"

	if c.Query("d") == "1" {
		disposition = "attachment"
	}

	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": file.Name}))

	tokens, err := getBotsToken(c, session.UserId, file.ChannelID)

	if err != nil {
		fs.log.Error("failed to get bots", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var (
		channelUser string
		lr          io.ReadCloser
	)

	var client *tgc.Client

	if config.GetConfig().DisableStreamBots || len(tokens) == 0 {
		tgClient, _ := tgc.UserLogin(c, session.Session)
		client, err = fs.worker.UserWorker(tgClient, session.UserId)
		if err != nil {
			fs.log.Error("file stream", zap.Error(err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		channelUser = strconv.FormatInt(session.UserId, 10)

		fs.log.Debug("requesting file", zap.String("name", file.Name),
			zap.String("user", channelUser), zap.Int64("start", start),
			zap.Int64("end", end), zap.Int64("fileSize", file.Size))

	} else {
		var index int
		limit := min(len(tokens), config.GetConfig().BgBotsLimit)

		fs.worker.Set(tokens[:limit], file.ChannelID)

		client, index, err = fs.worker.Next(file.ChannelID)

		if err != nil {
			fs.log.Error("file stream", zap.Error(err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		channelUser = strings.Split(tokens[index], ":")[0]
		fs.log.Debug("requesting file", zap.String("name", file.Name),
			zap.String("bot", channelUser), zap.Int("botNo", index), zap.Int64("start", start),
			zap.Int64("end", end), zap.Int64("fileSize", file.Size))
	}

	if r.Method != "HEAD" {
		parts, err := getParts(c, client.Tg, file, channelUser)
		if err != nil {
			fs.log.Error("file stream", zap.Error(err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		parts = rangedParts(parts, start, end)

		if file.Encrypted {
			lr, _ = reader.NewDecryptedReader(c, client.Tg, parts, contentLength)
		} else {
			lr, _ = reader.NewLinearReader(c, client.Tg, parts, contentLength)
		}

		io.CopyN(w, lr, contentLength)
	}
}

func setOrderFilter(query *gorm.DB, pagingParams *schemas.PaginationQuery, sortingParams *schemas.SortingQuery) *gorm.DB {
	if pagingParams.NextPageToken != "" {
		sortColumn := utils.CamelToSnake(sortingParams.Sort)

		tokenValue, err := base64.StdEncoding.DecodeString(pagingParams.NextPageToken)
		if err == nil {
			if sortingParams.Order == "asc" {
				return query.Where(fmt.Sprintf("%s > ?", sortColumn), string(tokenValue))
			} else {
				return query.Where(fmt.Sprintf("%s < ?", sortColumn), string(tokenValue))
			}
		}
	}
	return query
}

func getOrder(sortingParams schemas.SortingQuery) clause.OrderByColumn {
	sortColumn := utils.CamelToSnake(sortingParams.Sort)

	return clause.OrderByColumn{Column: clause.Column{Name: sortColumn},
		Desc: sortingParams.Order == "desc"}
}
