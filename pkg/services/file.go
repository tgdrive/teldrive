package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/WinterYukky/gorm-extra-clause-plugin/exclause"
	"github.com/divyam234/teldrive/internal/cache"
	"github.com/divyam234/teldrive/internal/category"
	"github.com/divyam234/teldrive/internal/config"
	"github.com/divyam234/teldrive/internal/database"
	"github.com/divyam234/teldrive/internal/http_range"
	"github.com/divyam234/teldrive/internal/md5"
	"github.com/divyam234/teldrive/internal/reader"
	"github.com/divyam234/teldrive/internal/tgc"
	"github.com/divyam234/teldrive/internal/utils"
	"github.com/divyam234/teldrive/pkg/logging"
	"github.com/divyam234/teldrive/pkg/mapper"
	"github.com/divyam234/teldrive/pkg/models"
	"github.com/divyam234/teldrive/pkg/schemas"
	"github.com/divyam234/teldrive/pkg/types"
	"github.com/gin-gonic/gin"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FileService struct {
	db     *gorm.DB
	cnf    *config.TGConfig
	worker *tgc.StreamWorker
}

func NewFileService(db *gorm.DB, cnf *config.Config, worker *tgc.StreamWorker) *FileService {
	return &FileService{db: db, cnf: &cnf.TG, worker: worker}
}

func (fs *FileService) CreateFile(c *gin.Context, userId int64, fileIn *schemas.FileIn) (*schemas.FileOut, *types.AppError) {

	var fileDB models.File

	fileIn.Path = strings.TrimSpace(fileIn.Path)

	if fileIn.Path != "" {
		pathId, err := fs.getPathId(fileIn.Path, userId)
		if err != nil || pathId == "" {
			return nil, &types.AppError{Error: err, Code: http.StatusNotFound}
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
		fileDB.Path = ""
		channelId := fileIn.ChannelID
		if fileIn.ChannelID == 0 {
			var err error
			channelId, err = GetDefaultChannel(c, fs.db, userId)
			if err != nil {
				return nil, &types.AppError{Error: err, Code: http.StatusNotFound}
			}
		}
		fileDB.ChannelID = &channelId
		fileDB.MimeType = fileIn.MimeType
		fileDB.Category = string(category.GetCategory(fileIn.Name))
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

	if err := fs.db.Create(&fileDB).Error; err != nil {
		if database.IsKeyConflictErr(err) {
			return nil, &types.AppError{Error: database.ErrKeyConflict, Code: http.StatusConflict}
		}
		return nil, &types.AppError{Error: err}
	}

	res := mapper.ToFileOut(fileDB)

	return res, nil
}

func (fs *FileService) UpdateFile(id string, userId int64, update *schemas.FileUpdate, cache *cache.Cache) (*schemas.FileOut, *types.AppError) {
	var (
		files []models.File
		chain *gorm.DB
	)
	if update.Type == "folder" && update.Name != "" {
		chain = fs.db.Raw("select * from teldrive.update_folder(?, ?, ?)", id, update.Name, userId).Scan(&files)
	} else {

		updateDb := models.File{
			Name:      update.Name,
			ParentID:  update.ParentID,
			UpdatedAt: update.UpdatedAt,
			Path:      update.Path,
			Size:      update.Size,
		}

		if update.Starred != nil {
			updateDb.Starred = *update.Starred
		}

		if len(update.Parts) > 0 {
			parts := models.Parts{}

			for _, part := range update.Parts {
				parts = append(parts, models.Part{
					ID:   part.ID,
					Salt: part.Salt,
				})

			}

			updateDb.Parts = &parts
		}
		chain = fs.db.Model(&files).Clauses(clause.Returning{}).Where("id = ?", id).Updates(updateDb)

		cache.Delete(fmt.Sprintf("files:%s", id))
	}

	if chain.Error != nil {
		return nil, &types.AppError{Error: chain.Error}
	}
	if chain.RowsAffected == 0 {
		return nil, &types.AppError{Error: database.ErrNotFound, Code: http.StatusNotFound}
	}

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

	var (
		pathId string
		err    error
	)

	if fquery.Path != "" {
		pathId, err = fs.getPathId(fquery.Path, userId)
		if err != nil {
			return nil, &types.AppError{Error: err, Code: http.StatusNotFound}
		}
	}

	query := fs.db.Limit(fquery.PerPage)
	setOrderFilter(query, fquery)

	if fquery.Op == "list" {
		filter := &models.File{UserID: userId, Status: "active"}
		query.Order("type DESC").Order(getOrder(fquery)).Where("parent_id = ?", pathId).
			Model(filter).Where(&filter)

	} else if fquery.Op == "find" {
		if !fquery.DeepSearch && pathId != "" && (fquery.Name != "" || fquery.Query != "") {
			query.Where("parent_id = ?", pathId)
			fquery.Path = ""
		} else if fquery.DeepSearch && pathId != "" && fquery.Query != "" {
			query = fs.db.Clauses(exclause.With{Recursive: true, CTEs: []exclause.CTE{{Name: "subdirs",
				Subquery: exclause.Subquery{DB: fs.db.Model(&models.File{Id: pathId}).Select("id", "parent_id").Clauses(exclause.NewUnion("ALL ?",
					fs.db.Table("teldrive.files as f").Select("f.id", "f.parent_id").
						Joins("inner join subdirs ON f.parent_id = subdirs.id")))}}}}).Where("files.id in (select id  from subdirs)")
			fquery.Path = ""
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
			query.Where("teldrive.get_tsquery(?) @@ teldrive.get_tsvector(name)", fquery.Query)
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

		filter := &models.File{UserID: userId, Status: "active"}
		filter.Name = fquery.Name
		filter.ParentID = fquery.ParentID
		filter.Path = fquery.Path
		filter.Type = fquery.Type
		if fquery.Starred != nil {
			filter.Starred = *fquery.Starred
		}

		query.Order("type DESC").Order(getOrder(fquery)).
			Model(&filter).Where(&filter)

		query.Limit(fquery.PerPage)
		setOrderFilter(query, fquery)
	}

	if fquery.Path == "" || fquery.DeepSearch {
		query.Select("*,(select path from teldrive.files as ff where ff.id = files.parent_id) as parent_path")
	}

	files := []schemas.FileOut{}

	query.Scan(&files)

	token := ""

	if len(files) == fquery.PerPage {
		lastItem := files[len(files)-1]
		token = utils.GetField(&lastItem, utils.CamelToPascalCase(fquery.Sort))
		token = base64.StdEncoding.EncodeToString([]byte(token))
	}

	res := &schemas.FileResponse{Files: files, NextPageToken: token}

	return res, nil
}

func (fs *FileService) getPathId(path string, userId int64) (string, error) {

	var file models.File

	if err := fs.db.Model(&models.File{}).Select("id").Where("path = ?", path).Where("user_id = ?", userId).
		First(&file).Error; database.IsRecordNotFoundErr(err) {
		return "", database.ErrNotFound

	}
	return file.Id, nil
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

	items := pgtype.Array[string]{
		Elements: payload.Files,
		Valid:    true,
		Dims:     []pgtype.ArrayDimension{{Length: int32(len(payload.Files)), LowerBound: 1}},
	}

	if err := fs.db.Exec("select * from teldrive.move_items(? , ? , ?)", items, payload.Destination, userId).Error; err != nil {
		return nil, &types.AppError{Error: err}
	}

	return &schemas.Message{Message: "files moved"}, nil
}

func (fs *FileService) DeleteFiles(userId int64, payload *schemas.FileOperation) (*schemas.Message, *types.AppError) {

	if err := fs.db.Exec("call teldrive.delete_files($1)", payload.Files).Error; err != nil {
		return nil, &types.AppError{Error: err}
	}

	return &schemas.Message{Message: "files deleted"}, nil
}

func (fs *FileService) DeleteFileParts(c *gin.Context, id string) (*schemas.Message, *types.AppError) {
	var file models.File
	if err := fs.db.Where("id = ?", id).First(&file).Error; err != nil {
		if database.IsRecordNotFoundErr(err) {
			return nil, &types.AppError{Error: database.ErrNotFound, Code: http.StatusNotFound}
		}
		return nil, &types.AppError{Error: err}
	}

	userId, session := GetUserAuth(c)

	ids := []int{}

	for _, part := range *file.Parts {
		ids = append(ids, int(part.ID))
	}

	err := DeleteTGMessages(c, fs.cnf, session, *file.ChannelID, userId, ids)

	if err != nil {
		return nil, &types.AppError{Error: err}
	}

	return &schemas.Message{Message: "file parts deleted"}, nil
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

	userId, session := GetUserAuth(c)

	client, _ := tgc.AuthClient(c, fs.cnf, session)

	var res []models.File

	if err := fs.db.Model(&models.File{}).Where("id = ?", payload.ID).Find(&res).Error; err != nil {
		return nil, &types.AppError{Error: err}
	}

	file := mapper.ToFileOutFull(res[0])

	newIds := models.Parts{}

	channelId, err := GetDefaultChannel(c, fs.db, userId)
	if err != nil {
		return nil, &types.AppError{Error: err}
	}

	err = tgc.RunWithAuth(c, client, "", func(ctx context.Context) error {
		user := strconv.FormatInt(userId, 10)
		messages, err := getTGMessages(c, client, file.Parts, file.ChannelID, user)

		if err != nil {
			return err
		}

		channel, err := GetChannelById(ctx, client, channelId, user)

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
			newIds = append(newIds, models.Part{ID: int64(msg.ID), Salt: file.Parts[i].Salt})

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
	dbFile.Parts = &newIds
	dbFile.UserID = userId
	dbFile.Starred = false
	dbFile.Status = "active"
	dbFile.ParentID = dest.Id
	dbFile.ChannelID = &channelId
	dbFile.Encrypted = file.Encrypted
	dbFile.Category = file.Category

	if err := fs.db.Create(&dbFile).Error; err != nil {
		return nil, &types.AppError{Error: err}
	}

	return mapper.ToFileOut(dbFile), nil
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
	cache := cache.FromContext(c)

	session, err := getSessionByHash(fs.db, cache, authHash)

	if err != nil {
		http.Error(w, "invalid hash", http.StatusBadRequest)
		return
	}

	file := &schemas.FileOutFull{}

	key := fmt.Sprintf("files:%s", fileID)

	err = cache.Get(key, file)

	var appErr *types.AppError

	if err != nil {
		file, appErr = fs.GetFileByID(fileID)
		if appErr != nil {
			http.Error(w, appErr.Error.Error(), http.StatusBadRequest)
			return
		}
		cache.Set(key, file, 0)
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
	c.Header("E-Tag", fmt.Sprintf("\"%s\"", md5.FromString(file.Id+strconv.FormatInt(file.Size, 10))))
	c.Header("Last-Modified", file.UpdatedAt.UTC().Format(http.TimeFormat))

	disposition := "inline"

	if c.Query("d") == "1" {
		disposition = "attachment"
	}

	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": file.Name}))

	tokens, err := getBotsToken(c, fs.db, session.UserId, file.ChannelID)

	logger := logging.FromContext(c)
	if err != nil {
		logger.Error("failed to get bots", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var (
		channelUser string
		lr          io.ReadCloser
	)

	var client *tgc.Client

	if fs.cnf.DisableStreamBots || len(tokens) == 0 {
		tgClient, _ := tgc.AuthClient(c, fs.cnf, session.Session)
		client, err = fs.worker.UserWorker(tgClient, session.UserId)
		if err != nil {
			logger.Error("file stream", zap.Error(err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		channelUser = strconv.FormatInt(session.UserId, 10)

		logger.Debugw("requesting file", "name", file.Name, "bot", channelUser, "user", channelUser, "start", start,
			"end", end, "fileSize", file.Size)

	} else {
		var index int

		limit := min(len(tokens), fs.cnf.BgBotsLimit)

		fs.worker.Set(tokens[:limit], file.ChannelID)

		client, index, err = fs.worker.Next(file.ChannelID)

		if err != nil {
			logger.Error("file stream", zap.Error(err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		channelUser = strings.Split(tokens[index], ":")[0]
		logger.Debugw("requesting file", "name", file.Name, "bot", channelUser, "botNo", index, "start", start,
			"end", end, "fileSize", file.Size)
	}

	if r.Method != "HEAD" {
		parts, err := getParts(c, client.Tg, file, channelUser)
		if err != nil {
			logger.Error("file stream", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if file.Encrypted {
			lr, err = reader.NewDecryptedReader(c, client.Tg, parts, start, end, fs.cnf.Uploads.EncryptionKey)
		} else {
			lr, err = reader.NewLinearReader(c, client.Tg, parts, start, end)
		}

		if err != nil {
			logger.Error("file stream", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if lr == nil {
			http.Error(w, "failed to initialise reader", http.StatusInternalServerError)
			return
		}

		io.CopyN(w, lr, contentLength)
	}
}
func setOrderFilter(query *gorm.DB, fquery *schemas.FileQuery) *gorm.DB {
	if fquery.NextPageToken != "" {
		sortColumn := utils.CamelToSnake(fquery.Sort)

		tokenValue, err := base64.StdEncoding.DecodeString(fquery.NextPageToken)
		if err == nil {
			if fquery.Order == "asc" {
				return query.Where(fmt.Sprintf("%s > ?", sortColumn), string(tokenValue))
			} else {
				return query.Where(fmt.Sprintf("%s < ?", sortColumn), string(tokenValue))
			}
		}
	}
	return query
}

func getOrder(fquery *schemas.FileQuery) clause.OrderByColumn {
	sortColumn := utils.CamelToSnake(fquery.Sort)

	return clause.OrderByColumn{Column: clause.Column{Name: sortColumn},
		Desc: fquery.Order == "desc"}
}
