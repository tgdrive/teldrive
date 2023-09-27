package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/divyam234/teldrive/database"
	"github.com/divyam234/teldrive/mapper"
	"github.com/divyam234/teldrive/models"
	"github.com/divyam234/teldrive/schemas"
	"github.com/divyam234/teldrive/utils"
	"github.com/divyam234/teldrive/utils/kv"
	"github.com/divyam234/teldrive/utils/md5"
	"github.com/divyam234/teldrive/utils/reader"
	"github.com/divyam234/teldrive/utils/tgc"
	"github.com/gotd/td/telegram"

	"github.com/divyam234/teldrive/types"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mitchellh/mapstructure"
	range_parser "github.com/quantumsheep/range-parser"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FileService struct {
	Db *gorm.DB
}

func (fs *FileService) CreateFile(c *gin.Context) (*schemas.FileOut, *types.AppError) {
	userId, _ := getUserAuth(c)
	var fileIn schemas.FileIn
	if err := c.ShouldBindJSON(&fileIn); err != nil {
		return nil, &types.AppError{Error: errors.New("invalid request payload"), Code: http.StatusBadRequest}
	}

	fileIn.Path = strings.TrimSpace(fileIn.Path)

	if fileIn.Path != "" {
		var parent models.File
		if err := fs.Db.Where("type = ? AND path = ?", "folder", fileIn.Path).First(&parent).Error; err != nil {
			return nil, &types.AppError{Error: errors.New("parent directory not found"), Code: http.StatusNotFound}
		}
		fileIn.ParentID = parent.ID
	}

	if fileIn.Type == "folder" {
		fileIn.MimeType = "drive/folder"
		var fullPath string
		if fileIn.Path == "/" {
			fullPath = "/" + fileIn.Name
		} else {
			fullPath = fileIn.Path + "/" + fileIn.Name
		}
		fileIn.Path = fullPath
		fileIn.Depth = utils.IntPointer(len(strings.Split(fileIn.Path, "/")) - 1)
	} else if fileIn.Type == "file" {
		fileIn.Path = ""

		channelId, err := GetDefaultChannel(c, userId)

		if err != nil {
			return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
		}

		fileIn.ChannelID = &channelId
	}

	fileIn.UserID = userId
	fileIn.Starred = utils.BoolPointer(false)
	fileIn.Status = "active"

	fileDb := mapper.MapFileInToFile(fileIn)

	if err := fs.Db.Create(&fileDb).Error; err != nil {
		pgErr := err.(*pgconn.PgError)
		if pgErr.Code == "23505" {
			return nil, &types.AppError{Error: errors.New("file exists"), Code: http.StatusBadRequest}
		}
		return nil, &types.AppError{Error: errors.New("failed to create a file"), Code: http.StatusBadRequest}

	}

	res := mapper.MapFileToFileOut(fileDb)

	return &res, nil
}

func (fs *FileService) UpdateFile(c *gin.Context) (*schemas.FileOut, *types.AppError) {

	fileID := c.Param("fileID")

	var fileUpdate schemas.FileIn

	var files []models.File

	if err := c.ShouldBindJSON(&fileUpdate); err != nil {
		return nil, &types.AppError{Error: errors.New("invalid request payload"), Code: http.StatusBadRequest}
	}

	if fileUpdate.Type == "folder" && fileUpdate.Name != "" {
		if err := fs.Db.Raw("select * from teldrive.update_folder(?, ?)", fileID, fileUpdate.Name).Scan(&files).Error; err != nil {
			return nil, &types.AppError{Error: errors.New("failed to update the file"), Code: http.StatusInternalServerError}
		}
	} else {
		fileDb := mapper.MapFileInToFile(fileUpdate)
		if err := fs.Db.Model(&files).Clauses(clause.Returning{}).Where("id = ?", fileID).Updates(fileDb).Error; err != nil {
			return nil, &types.AppError{Error: errors.New("failed to update the file"), Code: http.StatusInternalServerError}
		}
	}

	if len(files) == 0 {
		return nil, &types.AppError{Error: errors.New("file not updated"), Code: http.StatusNotFound}
	}

	file := mapper.MapFileToFileOut(files[0])

	key := kv.Key("files", fileID)
	database.KV.Delete(key)

	return &file, nil

}

func (fs *FileService) GetFileByID(c *gin.Context) (*schemas.FileOutFull, error) {

	fileID := c.Param("fileID")

	var file []models.File

	fs.Db.Model(&models.File{}).Where("id = ?", fileID).Find(&file)

	if len(file) == 0 {
		return nil, errors.New("file not found")
	}

	return mapper.MapFileToFileOutFull(file[0]), nil
}

func (fs *FileService) ListFiles(c *gin.Context) (*schemas.FileResponse, *types.AppError) {

	userId, _ := getUserAuth(c)

	var pagingParams schemas.PaginationQuery
	pagingParams.PerPage = 200
	if err := c.ShouldBindQuery(&pagingParams); err != nil {
		return nil, &types.AppError{Error: errors.New("invalid params"), Code: http.StatusBadRequest}
	}

	var sortingParams schemas.SortingQuery
	sortingParams.Order = "asc"
	sortingParams.Sort = "name"
	if err := c.ShouldBindQuery(&sortingParams); err != nil {
		return nil, &types.AppError{Error: errors.New("invalid params"), Code: http.StatusBadRequest}
	}

	var fileQuery schemas.FileQuery
	fileQuery.Op = "list"
	fileQuery.Status = "active"
	fileQuery.UserID = userId
	if err := c.ShouldBindQuery(&fileQuery); err != nil {
		return nil, &types.AppError{Error: errors.New("invalid params"), Code: http.StatusBadRequest}
	}

	query := fs.Db.Model(&models.File{}).Limit(pagingParams.PerPage).
		Where(map[string]interface{}{"user_id": userId, "status": "active"})

	if fileQuery.Op == "list" {

		if pathExists, message := fs.CheckIfPathExists(&fileQuery.Path); !pathExists {
			return nil, &types.AppError{Error: errors.New(message), Code: http.StatusNotFound}
		}

		setOrderFilter(query, &pagingParams, &sortingParams)

		query.Order("type DESC").Order(getOrder(sortingParams)).
			Where("parent_id in (?)", fs.Db.Model(&models.File{}).Select("id").Where("path = ?", fileQuery.Path))

	} else if fileQuery.Op == "find" {

		filterQuery := map[string]interface{}{}

		err := mapstructure.Decode(fileQuery, &filterQuery)

		if err != nil {
			return nil, &types.AppError{Error: err, Code: http.StatusBadRequest}
		}

		delete(filterQuery, "op")

		if filterQuery["updated_at"] == nil {
			delete(filterQuery, "updated_at")
		}

		if filterQuery["path"] != nil && filterQuery["name"] != nil {
			query.Where("parent_id in (?)", fs.Db.Model(&models.File{}).Select("id").Where("path = ?", filterQuery["path"]))
			delete(filterQuery, "path")
		}

		query.Order("type DESC").Order(getOrder(sortingParams)).Where(filterQuery)

	} else if fileQuery.Op == "search" {

		query.Where("teldrive.get_tsquery(?) @@ teldrive.get_tsvector(name)", fileQuery.Search)

		setOrderFilter(query, &pagingParams, &sortingParams)
		query.Order(getOrder(sortingParams))

	}

	var results []schemas.FileOut

	query.Find(&results)

	token := ""

	if len(results) == pagingParams.PerPage {
		lastItem := results[len(results)-1]
		token = utils.GetField(&lastItem, utils.CamelToPascalCase(sortingParams.Sort))
		token = base64.StdEncoding.EncodeToString([]byte(token))
	}

	res := &schemas.FileResponse{Results: results, NextPageToken: token}

	return res, nil
}

func (fs *FileService) CheckIfPathExists(path *string) (bool, string) {
	query := fs.Db.Model(&models.File{}).Select("id").Where("path = ?", path)
	var results []schemas.FileOut
	query.Find(&results)
	if len(results) == 0 {
		return false, "This directory doesn't exist."
	}
	return true, ""
}

func (fs *FileService) MakeDirectory(c *gin.Context) (*schemas.FileOut, *types.AppError) {

	var payload schemas.MkDir

	var files []models.File

	if err := c.ShouldBindJSON(&payload); err != nil {
		return nil, &types.AppError{Error: errors.New("invalid request payload"), Code: http.StatusBadRequest}
	}

	userId, _ := getUserAuth(c)
	if err := fs.Db.Raw("select * from teldrive.create_directories(?, ?)", userId, payload.Path).Scan(&files).Error; err != nil {
		return nil, &types.AppError{Error: errors.New("failed to create directories"), Code: http.StatusInternalServerError}
	}

	file := mapper.MapFileToFileOut(files[0])

	return &file, nil

}

func (fs *FileService) MoveFiles(c *gin.Context) (*schemas.Message, *types.AppError) {

	var payload schemas.FileOperation

	if err := c.ShouldBindJSON(&payload); err != nil {
		return nil, &types.AppError{Error: errors.New("invalid request payload"), Code: http.StatusBadRequest}
	}

	var destination models.File

	if pathExists, message := fs.CheckIfPathExists(&payload.Destination); !pathExists {
		return nil, &types.AppError{Error: errors.New(message), Code: http.StatusBadRequest}
	}

	if err := fs.Db.Model(&models.File{}).Select("id").Where("path = ?", payload.Destination).First(&destination).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, &types.AppError{Error: errors.New("destination not found"), Code: http.StatusNotFound}

	}

	if err := fs.Db.Model(&models.File{}).Where("id IN ?", payload.Files).UpdateColumn("parent_id", destination.ID).Error; err != nil {
		return nil, &types.AppError{Error: errors.New("move failed"), Code: http.StatusInternalServerError}
	}

	return &schemas.Message{Status: true, Message: "files moved"}, nil
}

func (fs *FileService) DeleteFiles(c *gin.Context) (*schemas.Message, *types.AppError) {

	var payload schemas.FileOperation

	if err := c.ShouldBindJSON(&payload); err != nil {
		return nil, &types.AppError{Error: errors.New("invalid request payload"), Code: http.StatusBadRequest}
	}

	if err := fs.Db.Exec("call teldrive.delete_files($1)", payload.Files).Error; err != nil {
		return nil, &types.AppError{Error: errors.New("failed to delete files"), Code: http.StatusInternalServerError}
	}

	return &schemas.Message{Status: true, Message: "files deleted"}, nil
}

func (fs *FileService) GetFileStream(c *gin.Context) {

	w := c.Writer
	r := c.Request

	fileID := c.Param("fileID")

	authHash := c.Query("hash")

	if authHash == "" {
		http.Error(w, "misssing hash", http.StatusBadRequest)
		return
	}

	data, err := database.KV.Get(kv.Key("sessions", authHash))

	if err != nil {
		http.Error(w, "hash missing relogin", http.StatusBadRequest)
		return
	}

	jwtUser := &types.JWTClaims{}

	err = json.Unmarshal(data, jwtUser)

	if err != nil {
		http.Error(w, "invalid hash", http.StatusBadRequest)
		return
	}

	file := &schemas.FileOutFull{}

	key := kv.Key("files", fileID)

	err = kv.GetValue(database.KV, key, file)
	if err != nil {
		file, err = fs.GetFileByID(c)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		kv.SetValue(database.KV, key, file)
	}

	c.Header("Accept-Ranges", "bytes")

	var start, end int64

	rangeHeader := r.Header.Get("Range")

	if rangeHeader == "" {
		start = 0
		end = file.Size - 1
		w.WriteHeader(http.StatusOK)
	} else {
		ranges, err := range_parser.Parse(file.Size, r.Header.Get("Range"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
	c.Header("E-Tag", md5.FromString(file.ID+strconv.FormatInt(file.Size, 10)))
	c.Header("Last-Modified", file.UpdatedAt.UTC().Format(http.TimeFormat))

	disposition := "inline"

	if c.Query("d") == "1" {
		disposition = "attachment"
	}

	c.Header("Content-Disposition", fmt.Sprintf("%s; filename=\"%s\"", disposition, file.Name))

	userID, _ := strconv.ParseInt(jwtUser.Subject, 10, 64)

	tokens, err := GetBotsToken(c, userID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	config := utils.GetConfig()

	var token string

	var channelUser string

	if config.LazyStreamBots || len(tokens) == 0 {
		var client *telegram.Client
		if len(tokens) == 0 {
			client, _ = tgc.UserLogin(jwtUser.TgSession)
			channelUser = jwtUser.Subject
		} else {
			tgc.Workers.Set(tokens)
			token = tgc.Workers.Next()
			client, _ = tgc.BotLogin(token)
			channelUser = strings.Split(token, ":")[0]
		}
		if r.Method != "HEAD" {
			tgc.RunWithAuth(c, client, token, func(ctx context.Context) error {
				parts, err := getParts(c, client, file, channelUser)
				if err != nil {
					return err
				}
				parts = rangedParts(parts, start, end)
				r, _ := reader.NewLinearReader(c, client, parts)
				io.CopyN(w, r, contentLength)
				return nil
			})
		}

	} else {
		limit := utils.Min(len(tokens), config.BgBotsLimit)

		tgc.StreamWorkers.Set(tokens[:limit])

		client, err := tgc.StreamWorkers.Next()

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		channelUser = strings.Split(token, ":")[0]

		if r.Method != "HEAD" {

			parts, err := getParts(c, client.Tg, file, channelUser)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			parts = rangedParts(parts, start, end)
			r, _ := reader.NewLinearReader(c, client.Tg, parts)
			io.CopyN(w, r, contentLength)
		}
	}

}

func setOrderFilter(query *gorm.DB, pagingParams *schemas.PaginationQuery, sortingParams *schemas.SortingQuery) *gorm.DB {
	if pagingParams.NextPageToken != "" {
		sortColumn := sortingParams.Sort
		if sortColumn == "name" {
			sortColumn = "name collate numeric"
		} else {
			sortColumn = utils.CamelToSnake(sortingParams.Sort)
		}

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

func getOrder(sortingParams schemas.SortingQuery) string {
	sortColumn := utils.CamelToSnake(sortingParams.Sort)
	if sortingParams.Sort == "name" {
		sortColumn = "name collate numeric"
	}

	return fmt.Sprintf("%s %s", sortColumn, strings.ToUpper(sortingParams.Order))
}
