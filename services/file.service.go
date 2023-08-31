package services

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/divyam234/teldrive/cache"
	"github.com/divyam234/teldrive/models"
	"github.com/divyam234/teldrive/schemas"
	"github.com/divyam234/teldrive/utils"

	"github.com/divyam234/teldrive/types"

	"github.com/gin-gonic/gin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mitchellh/mapstructure"
	range_parser "github.com/quantumsheep/range-parser"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FileService struct {
	Db        *gorm.DB
	ChannelID int64
}

func getAuthUserId(c *gin.Context) int64 {
	val, _ := c.Get("jwtUser")
	jwtUser := val.(*types.JWTClaims)
	userId, _ := strconv.ParseInt(jwtUser.Subject, 10, 64)
	return userId
}

func (fs *FileService) CreateFile(c *gin.Context) (*schemas.FileOut, *types.AppError) {
	userId := getAuthUserId(c)
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
		fileIn.ChannelID = &fs.ChannelID
	}

	fileIn.UserID = userId
	fileIn.Starred = utils.BoolPointer(false)
	fileIn.Status = "active"

	fileDb := mapFileInToFile(fileIn)

	if err := fs.Db.Create(&fileDb).Error; err != nil {
		pgErr := err.(*pgconn.PgError)
		if pgErr.Code == "23505" {
			return nil, &types.AppError{Error: errors.New("file exists"), Code: http.StatusBadRequest}
		}
		return nil, &types.AppError{Error: errors.New("failed to create a file"), Code: http.StatusBadRequest}
	}

	res := mapFileToFileOut(fileDb)

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
		fileDb := mapFileInToFile(fileUpdate)
		if err := fs.Db.Model(&files).Clauses(clause.Returning{}).Where("id = ?", fileID).Updates(fileDb).Error; err != nil {
			return nil, &types.AppError{Error: errors.New("failed to update the file"), Code: http.StatusInternalServerError}
		}
	}

	if len(files) == 0 {
		return nil, &types.AppError{Error: errors.New("file not updated"), Code: http.StatusNotFound}
	}

	file := mapFileToFileOut(files[0])

	return &file, nil
}

func (fs *FileService) ShareFile(c *gin.Context, session *types.Session) (*string, *types.AppError) {
	fileID := c.Param("fileID")
	var payload schemas.FileShare

	if err := c.ShouldBindJSON(&payload); err != nil {
		return nil, &types.AppError{Error: errors.New("invalida request payload"), Code: http.StatusBadRequest}
	}
	query := fs.Db.Model(&models.File{}).Select("id").Where("id = ?", fileID)
	var results []schemas.FileOut
	query.Find(&results)
	if len(results) == 0 {
		return nil, &types.AppError{Error: errors.New("this file does not exist"), Code: http.StatusInternalServerError}
	}

	tx := fs.Db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var fileUpdate models.File
	fileUpdate.Visibility = payload.Visibility

	if err := fs.Db.Model(&fileUpdate).Clauses(clause.Returning{}).Where("id = ?", fileID).Updates(fileUpdate).Error; err != nil {
		return nil, &types.AppError{Error: errors.New("failed to update the file"), Code: http.StatusInternalServerError}
	}

	log.Println(payload.Usernames, "mira")

	err := fs.Db.Transaction(func(tx *gorm.DB) error {
		var sharedFileCount int64
		if err := tx.Model(&models.SharedFile{}).Where("file_id = ?", fileID).Count(&sharedFileCount).Error; err != nil {
			return err
		}

		if sharedFileCount == 0 {
			if len(payload.Usernames) != 0 {
				// Crear nuevos registros para los usuarios compartidos
				var sharedFiles []models.SharedFile
				for _, frontendUser := range payload.Usernames {
					sharedFiles = append(sharedFiles, models.SharedFile{
						FileID:             fileID,
						SharedWithUsername: frontendUser,
					})
				}
				if err := tx.Create(&sharedFiles).Error; err != nil {
					return err
				}
			}
		} else {
			var existingSharedUsers []string
			err := tx.Model(&models.SharedFile{}).Select("shared_with_username").Where("file_id = ?", fileID).Pluck("shared_with_username", &existingSharedUsers).Error
			if err != nil {
				return err
			}

			var newSharedUsers []string
			var removedSharedUsers []string

			// Identificar nuevos usuarios y usuarios eliminados
			for _, newUsername := range payload.Usernames {
				if !utils.Contains(existingSharedUsers, newUsername) {
					newSharedUsers = append(newSharedUsers, newUsername)
				}
			}
			for _, existingUsername := range existingSharedUsers {
				if !utils.Contains(payload.Usernames, existingUsername) {
					removedSharedUsers = append(removedSharedUsers, existingUsername)
				}
			}
			log.Println("new: ", newSharedUsers, "removed: ", removedSharedUsers)
			// Agregar nuevos usuarios a la lista compartida
			if len(newSharedUsers) > 0 {
				// Construir el array de usernames en la consulta SQL
				usernamesArray := "{" + strings.Join(newSharedUsers, ",") + "}"
				err = tx.Exec("SELECT teldrive.add_shared_users(?, ?::text[])", fileID, usernamesArray).Error
				if err != nil {
					return err
				}
			}

			// Eliminar usuarios que ya no están en la lista compartida
			if len(removedSharedUsers) > 0 {
				// Construir el array de usernames en la consulta SQL
				usernamesArray := "{" + strings.Join(removedSharedUsers, ",") + "}"
				err = tx.Exec("SELECT teldrive.remove_shared_users(?, ?::text[])", fileID, usernamesArray).Error
				if err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		log.Println(err)
		return nil, &types.AppError{Error: errors.New("failed to update shared usernames"), Code: http.StatusInternalServerError}
	}

	tx.Commit()

	return &fileUpdate.Visibility, nil
}

func (fs *FileService) GetFileByID(c *gin.Context) (*schemas.FileOutFull, error) {

	fileID := c.Param("fileID")

	var file []models.File

	fs.Db.Model(&models.File{}).Where("id = ?", fileID).Find(&file)

	if len(file) == 0 {
		return nil, errors.New("file not found")
	}

	return mapFileToFileOutFull(file[0]), nil
}

func (fs *FileService) ListFiles(c *gin.Context) (*schemas.FileResponse, *types.AppError) {

	userId := getAuthUserId(c)

	var pagingParams schemas.PaginationQuery
	pagingParams.PerPage = 200
	if err := c.ShouldBindQuery(&pagingParams); err != nil {
		return nil, &types.AppError{Error: errors.New(""), Code: http.StatusBadRequest}
	}

	var sortingParams schemas.SortingQuery
	sortingParams.Order = "asc"
	sortingParams.Sort = "name"
	if err := c.ShouldBindQuery(&sortingParams); err != nil {
		return nil, &types.AppError{Error: errors.New(""), Code: http.StatusBadRequest}
	}

	var fileQuery schemas.FileQuery
	fileQuery.Op = "list"
	fileQuery.Status = "active"
	fileQuery.UserID = userId
	if err := c.ShouldBindQuery(&fileQuery); err != nil {
		return nil, &types.AppError{Error: errors.New(""), Code: http.StatusBadRequest}
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

		setOrderFilter(query, &pagingParams, &sortingParams)

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

	userId := getAuthUserId(c)
	if err := fs.Db.Raw("select * from teldrive.create_directories(?, ?)", userId, payload.Path).Scan(&files).Error; err != nil {
		return nil, &types.AppError{Error: errors.New("failed to create directories"), Code: http.StatusInternalServerError}
	}

	file := mapFileToFileOut(files[0])

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

	var err error

	res, err := cache.CachedFunction(fs.GetFileByID, fmt.Sprintf("files:%s", fileID))(c)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	file := res.(*schemas.FileOutFull)

	w.Header().Set("Accept-Ranges", "bytes")

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
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, file.Size))
		w.WriteHeader(http.StatusPartialContent)
	}

	contentLength := end - start + 1

	w.Header().Set("Content-Type", file.MimeType)

	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))

	disposition := "inline"

	if c.Query("d") == "1" {
		disposition = "attachment"
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=\"%s\"", disposition, file.Name))

	client, idx := utils.GetDownloadClient(c)

	defer func() {
		utils.GetClientWorkload().Dec(idx)
	}()

	ir, iw := io.Pipe()
	parts, err := fs.getParts(c, client, file)
	if err != nil {
		return
	}
	parts = rangedParts(parts, int64(start), int64(end))

	if r.Method != "HEAD" {
		go func() {
			defer iw.Close()
			for _, part := range parts {
				streamFilePart(c, client, iw, &part, part.Start, part.End, 1024*1024)
			}
		}()
		io.CopyN(w, ir, contentLength)
	}

}

func (fs *FileService) getParts(ctx context.Context, tgClient *telegram.Client, file *schemas.FileOutFull) ([]types.Part, error) {

	ids := []tg.InputMessageID{}

	for _, part := range *file.Parts {
		ids = append(ids, tg.InputMessageID{ID: int(part.ID)})
	}

	s := make([]tg.InputMessageClass, len(ids))

	for i := range ids {
		s[i] = &ids[i]
	}

	api := tgClient.API()

	res, err := cache.CachedFunction(utils.GetChannelById, fmt.Sprintf("channels:%d", fs.ChannelID))(ctx, api, fs.ChannelID)

	if err != nil {
		return nil, err
	}

	channel := res.(*tg.Channel)

	messageRequest := tg.ChannelsGetMessagesRequest{Channel: &tg.InputChannel{ChannelID: fs.ChannelID, AccessHash: channel.AccessHash},
		ID: s}

	res, err = cache.CachedFunction(api.ChannelsGetMessages, fmt.Sprintf("messages:%s", file.ID))(ctx, &messageRequest)

	if err != nil {
		return nil, err
	}

	messages := res.(*tg.MessagesChannelMessages)

	parts := []types.Part{}

	for _, message := range messages.Messages {
		item := message.(*tg.Message)
		media := item.Media.(*tg.MessageMediaDocument)
		document := media.Document.(*tg.Document)
		location := document.AsInputDocumentFileLocation()
		parts = append(parts, types.Part{Location: location, Start: 0, End: document.Size - 1, Size: document.Size})
	}
	return parts, nil
}

func mapFileToFileOut(file models.File) schemas.FileOut {
	return schemas.FileOut{
		ID:        file.ID,
		Name:      file.Name,
		Type:      file.Type,
		MimeType:  file.MimeType,
		Path:      file.Path,
		Size:      file.Size,
		Starred:   file.Starred,
		ParentID:  file.ParentID,
		UpdatedAt: file.UpdatedAt,
	}
}

func mapFileInToFile(file schemas.FileIn) models.File {
	return models.File{
		Name:      file.Name,
		Type:      file.Type,
		MimeType:  file.MimeType,
		Path:      file.Path,
		Size:      file.Size,
		Starred:   file.Starred,
		Depth:     file.Depth,
		UserID:    file.UserID,
		ParentID:  file.ParentID,
		Parts:     file.Parts,
		ChannelID: file.ChannelID,
		Status:    file.Status,
	}
}

func mapFileToFileOutFull(file models.File) *schemas.FileOutFull {
	return &schemas.FileOutFull{
		FileOut: mapFileToFileOut(file),
		Parts:   file.Parts, ChannelID: file.ChannelID,
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

func chunk(ctx context.Context, tgClient *telegram.Client, part *types.Part, offset int64, limit int64) ([]byte, error) {

	req := &tg.UploadGetFileRequest{
		Offset:   offset,
		Limit:    int(limit),
		Location: part.Location,
	}

	r, err := tgClient.API().UploadGetFile(ctx, req)

	if err != nil {
		return nil, err
	}

	switch result := r.(type) {
	case *tg.UploadFile:
		return result.Bytes, nil
	default:
		return nil, fmt.Errorf("unexpected type %T", r)
	}
}

func totalParts(start, end, chunkSize int64) int {

	totalBytes := end - start + 1
	parts := totalBytes / chunkSize

	if totalBytes%chunkSize != 0 {
		parts++
	}

	return int(parts)
}

func streamFilePart(ctx context.Context, tgClient *telegram.Client, writer *io.PipeWriter, part *types.Part, start, end, chunkSize int64) error {

	offset := start - (start % chunkSize)
	firstPartCut := start - offset
	lastPartCut := (end % chunkSize) + 1

	partCount := totalParts(start, end, chunkSize)

	currentPart := 1

	for {
		r, _ := chunk(ctx, tgClient, part, offset, chunkSize)

		if len(r) == 0 {
			break
		} else if partCount == 1 {
			r = r[firstPartCut:lastPartCut]

		} else if currentPart == 1 {
			r = r[firstPartCut:]

		} else if currentPart == partCount {
			r = r[:lastPartCut]

		}

		writer.Write(r)

		currentPart++

		offset += chunkSize

		if currentPart > partCount {
			break
		}

	}

	return nil
}

func rangedParts(parts []types.Part, start, end int64) []types.Part {

	chunkSize := parts[0].Size

	startPartNumber := utils.Max(int64(math.Ceil(float64(start)/float64(chunkSize)))-1, 0)

	endPartNumber := int64(math.Ceil(float64(end) / float64(chunkSize)))

	partsToDownload := parts[startPartNumber:endPartNumber]
	partsToDownload[0].Start = start % chunkSize
	partsToDownload[len(partsToDownload)-1].End = end % chunkSize

	return partsToDownload
}
