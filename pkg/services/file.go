package services

import (
	"context"
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
	"github.com/tgdrive/teldrive/internal/events"
	"github.com/tgdrive/teldrive/internal/hash"
	"github.com/tgdrive/teldrive/internal/http_range"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/md5"
	"github.com/tgdrive/teldrive/internal/reader"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/mapper"
	"github.com/tgdrive/teldrive/pkg/models"
	"github.com/tgdrive/teldrive/pkg/types"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrorStreamAbandoned = errors.New("stream abandoned")
	defaultContentType   = "application/octet-stream"
)

func isUUID(str string) bool {
	_, err := uuid.Parse(str)
	return err == nil
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

	client, _ := tgc.AuthClient(ctx, &a.cnf.TG, auth.GetJWTUser(ctx).TgSession, a.newMiddlewares(ctx, 5)...)

	var res []models.File

	if err := a.db.Model(&models.File{}).Where("id = ?", params.ID).Find(&res).Error; err != nil {
		return nil, &apiError{err: err}
	}
	if len(res) == 0 {
		return nil, &apiError{err: errors.New("file not found"), code: 404}
	}

	file := res[0]

	newIds := []api.Part{}

	channelId, err := a.channelManager.CurrentChannel(ctx, userId)
	if err != nil {
		return nil, &apiError{err: err}
	}

	err = tgc.RunWithAuth(ctx, client, "", func(ctx context.Context) error {

		ids := utils.Map(*file.Parts, func(part api.Part) int { return part.ID })
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

			id, _ := client.RandInt64()
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
			if (*file.Parts)[i].Salt.Value != "" {
				p.Salt = (*file.Parts)[i].Salt
			}
			newIds = append(newIds, p)

		}
		return nil
	})

	if err != nil {
		return nil, &apiError{err: err}
	}

	if len(newIds) != len(*file.Parts) {
		return nil, &apiError{err: errors.New("failed to copy all file parts")}
	}

	var parentId string
	if !isUUID(req.Destination) {
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
	dbFile.Type = file.Type
	dbFile.MimeType = file.MimeType
	if len(newIds) > 0 {
		dbFile.Parts = utils.Ptr(datatypes.NewJSONSlice(newIds))
	}
	dbFile.UserId = userId
	dbFile.Status = "active"
	dbFile.ParentId = utils.Ptr(parentId)
	dbFile.ChannelId = &channelId
	dbFile.Encrypted = file.Encrypted
	dbFile.Category = file.Category
	dbFile.Hash = file.Hash // Preserve hash during copy (content is identical)
	if req.UpdatedAt.IsSet() && !req.UpdatedAt.Value.IsZero() {
		dbFile.UpdatedAt = utils.Ptr(req.UpdatedAt.Value)
	} else {
		dbFile.UpdatedAt = utils.Ptr(time.Now().UTC())
	}

	if err := a.db.Create(&dbFile).Error; err != nil {
		return nil, &apiError{err: err}
	}

	a.events.Record(events.OpCopy, userId, &models.Source{
		ID:       dbFile.ID,
		Type:     dbFile.Type,
		Name:     dbFile.Name,
		ParentID: parentId,
	})
	return mapper.ToFileOut(dbFile), nil
}

func (a *apiService) FilesCreate(ctx context.Context, fileIn *api.File) (*api.File, error) {
	userId := auth.GetUser(ctx)

	var (
		fileDB    models.File
		parentID  *string
		err       error
		path      string
		channelId int64
		uploadId  string
		uploads   []models.Upload
	)

	if fileIn.Path.Value == "" && fileIn.ParentId.Value == "" {
		return nil, &apiError{err: errors.New("parent id or path is required"), code: 409}
	}

	if fileIn.Path.Value != "" {
		path = strings.ReplaceAll(fileIn.Path.Value, "//", "/")

	}

	if path != "" && fileIn.ParentId.Value == "" {
		parentID, err = resolvePathID(a.db, path, userId)
		if err != nil {
			return nil, &apiError{err: err, code: 404}
		}
		fileDB.ParentId = parentID

	} else if fileIn.ParentId.Value != "" {
		fileDB.ParentId = utils.Ptr(fileIn.ParentId.Value)
	}

	switch fileIn.Type {
	case api.FileTypeFolder:
		fileDB.MimeType = "drive/folder"
		fileDB.Parts = nil
	case api.FileTypeFile:
		if fileIn.ChannelId.Value == 0 {
			channelId, err = a.channelManager.CurrentChannel(ctx, userId)
			if err != nil {
				return nil, &apiError{err: err}
			}
		} else {
			channelId = fileIn.ChannelId.Value
		}
		fileDB.ChannelId = &channelId
		fileDB.MimeType = fileIn.MimeType.Value
		fileDB.Category = utils.Ptr(string(category.GetCategory(fileIn.Name)))

		// Handle parts - either from direct input or fetch by uploadId
		var parts []api.Part
		if len(fileIn.Parts) > 0 {
			parts = fileIn.Parts
		} else if fileIn.UploadId.Value != "" {
			uploadId = fileIn.UploadId.Value
			// Fetch parts from uploads table
			if err := a.db.Where("upload_id = ?", uploadId).Order("part_no").Find(&uploads).Error; err != nil {
				return nil, &apiError{err: err}
			}

			// Validate parts: sum of sizes must equal file size and no partId should be 0
			for _, upload := range uploads {
				if upload.PartId == 0 {
					return nil, &apiError{err: errors.New("invalid part: part_id cannot be zero"), code: 400}
				}
			}
			
			// Convert uploads to parts
			for _, upload := range uploads {
				parts = append(parts, api.Part{
					ID:   upload.PartId,
					Salt: api.NewOptString(upload.Salt),
				})
			}
		}

		if len(parts) > 0 {
			fileDB.Parts = utils.Ptr(datatypes.NewJSONSlice(mapParts(parts)))
		}

		// Compute BLAKE3 tree hash from block hashes if uploadId is provided
		if uploadId != "" && len(uploads) > 0 {
			var allBlockHashes []byte
			for _, upload := range uploads {
				allBlockHashes = append(allBlockHashes, upload.BlockHashes...)
			}

			if len(allBlockHashes) > 0 {
				treeHashBytes := hash.ComputeTreeHash(allBlockHashes)
				treeHash := hash.SumToHex(treeHashBytes)
				fileDB.Hash = &treeHash
			}
		} else if fileIn.Size.Value == 0 {
			// For zero-length files, compute hash of empty data
			treeHashBytes := hash.ComputeTreeHash([]byte{})
			treeHash := hash.SumToHex(treeHashBytes)
			fileDB.Hash = &treeHash
		}

		fileDB.Size = utils.Ptr(fileIn.Size.Value)
	}
	fileDB.Name = fileIn.Name
	fileDB.Type = string(fileIn.Type)
	fileDB.UserId = userId
	fileDB.Status = "active"
	fileDB.Encrypted = utils.Ptr(fileIn.Encrypted.Value)
	if fileIn.UpdatedAt.IsSet() && !fileIn.UpdatedAt.Value.IsZero() {
		fileDB.UpdatedAt = utils.Ptr(fileIn.UpdatedAt.Value)
	} else {
		fileDB.UpdatedAt = utils.Ptr(time.Now().UTC())
	}

	// Use transaction to ensure file creation and upload cleanup are atomic
	err = a.db.Transaction(func(tx *gorm.DB) error {
		//For some reason, gorm conflict clauses are not working with partial index so using raw query
		if err := tx.Raw(`
			INSERT INTO teldrive.files (
				name, parent_id, user_id, mime_type, category, parts,
				size, type, encrypted, updated_at, channel_id, status, hash
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (name, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), user_id)
			WHERE status = 'active'
			DO UPDATE SET
				mime_type = EXCLUDED.mime_type,
				category = EXCLUDED.category,
				parts = EXCLUDED.parts,
				size = EXCLUDED.size,
				type = EXCLUDED.type,
				encrypted = EXCLUDED.encrypted,
				updated_at = EXCLUDED.updated_at,
				channel_id = EXCLUDED.channel_id,
				status = EXCLUDED.status,
				hash = EXCLUDED.hash
			RETURNING *
		`,
			fileDB.Name, fileDB.ParentId, fileDB.UserId, fileDB.MimeType,
			fileDB.Category, fileDB.Parts, fileDB.Size, fileDB.Type,
			fileDB.Encrypted, fileDB.UpdatedAt, fileDB.ChannelId, fileDB.Status,
			fileDB.Hash,
		).Scan(&fileDB).Error; err != nil {
			return err
		}

		// Delete uploads after successful file creation
		if uploadId != "" {
			if err := tx.Where("upload_id = ?", uploadId).Delete(&models.Upload{}).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, &apiError{err: err}
	}

	if fileDB.ParentId != nil {
		parentID = fileDB.ParentId
	}

	a.events.Record(events.OpCreate, userId, &models.Source{
		ID:       fileDB.ID,
		Type:     fileDB.Type,
		Name:     fileDB.Name,
		ParentID: *parentID,
	})
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

func (a *apiService) deleteFilesBulk(db *gorm.DB, fileIds []string, userId int64) error {
	query := `
	WITH RECURSIVE target_folders AS (
		SELECT id FROM teldrive.files WHERE id IN (?) AND user_id = ?
		UNION ALL
		SELECT f.id FROM teldrive.files f JOIN target_folders tf ON f.parent_id = tf.id
	),
	mark_deleted AS (
		UPDATE teldrive.files SET status = 'pending_deletion'
		WHERE (parent_id IN (SELECT id FROM target_folders) OR id IN (?))
		AND type = 'file'
	)
	DELETE FROM teldrive.files WHERE id IN (SELECT id FROM target_folders) AND type = 'folder';
	`
	return db.Exec(query, fileIds, userId, fileIds).Error
}

func (a *apiService) getFullPath(db *gorm.DB, fileID string) (string, error) {
	var path string
	query := `
	WITH RECURSIVE path_tree AS (
		SELECT id, parent_id, name, 0 as lvl FROM teldrive.files WHERE id = ?
		UNION ALL
		SELECT f.id, f.parent_id, f.name, pt.lvl + 1
		FROM teldrive.files f JOIN path_tree pt ON f.id = pt.parent_id
	)
	SELECT string_agg(name, '/' ORDER BY lvl DESC) FROM path_tree;
	`
	err := db.Raw(query, fileID).Scan(&path).Error
	if path != "" {
		path = "/" + path
	}
	return strings.TrimPrefix(path, "/root"), err
}

func (a *apiService) FilesDelete(ctx context.Context, req *api.FileDelete) error {
	userId := auth.GetUser(ctx)

	if len(req.Ids) == 0 {
		return &apiError{err: errors.New("ids should not be empty"), code: 409}
	}

	var fileDB models.File

	if err := a.db.Model(&models.File{}).Where("id = ?", req.Ids[0]).Where("user_id = ?", userId).
		First(&fileDB).Error; err != nil {
		return &apiError{err: err}
	}

	if err := a.deleteFilesBulk(a.db, req.Ids, userId); err != nil {
		return &apiError{err: err}
	}

	keys := []string{}
	for _, id := range req.Ids {
		keys = append(keys, cache.KeyFile(id), cache.KeyFileMessages(id))
	}
	if len(keys) > 0 {
		a.cache.Delete(ctx, keys...)
	}

	var parentID string
	if fileDB.ParentId != nil {
		parentID = *fileDB.ParentId
	}

	a.events.Record(events.OpDelete, userId, &models.Source{
		ID:       fileDB.ID,
		Type:     fileDB.Type,
		Name:     fileDB.Name,
		ParentID: parentID,
	})

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
		a.cache.Delete(ctx, cache.KeyShare(deletedShare.ID))
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
	var file models.File
	if err := a.db.Model(&models.File{}).Where("id = ?", params.ID).First(&file).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, &apiError{err: errors.New("file not found"), code: 404}
		}
		return nil, &apiError{err: err}
	}

	path, err := a.getFullPath(a.db, params.ID)
	if err != nil {
		return nil, &apiError{err: err}
	}

	res := mapper.ToFileOut(file)
	res.Path = api.NewOptString(path)
	if file.ChannelId != nil {
		res.ChannelId = api.NewOptInt64(*file.ChannelId)
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

	var destParentID *string

	if !isUUID(req.DestinationParent) {
		r, err := resolvePathID(a.db, req.DestinationParent, userId)
		if err != nil {
			return &apiError{err: err}
		}
		destParentID = r

	} else {
		destParentID = &req.DestinationParent
	}

	err := a.db.Transaction(func(tx *gorm.DB) error {
		var srcFile models.File
		if err := tx.Where("id = ? AND user_id = ?", req.Ids[0], userId).First(&srcFile).Error; err != nil {
			return err
		}
		if len(req.Ids) == 1 && req.DestinationName.Value != "" {
			var existing models.File
			query := tx.Where("name = ? AND user_id = ? AND status = 'active'",
				req.DestinationName.Value, userId)
			if destParentID == nil {
				query = query.Where("parent_id IS NULL")
			} else {
				query = query.Where("parent_id = ?", *destParentID)
			}

			if err := query.First(&existing).Error; err == nil {
				if srcFile.Type == "folder" && existing.Type == "folder" {
					if err := tx.Model(&models.File{}).
						Where("parent_id = ? AND status = 'active'", existing.ID).
						Where("name NOT IN (?)",
							tx.Model(&models.File{}).
								Select("name").
								Where("parent_id = ? AND status = 'active'", srcFile.ID),
						).
						Update("parent_id", srcFile.ID).Error; err != nil {
						return err
					}
				}
				if err := a.deleteFilesBulk(tx, []string{existing.ID}, userId); err != nil {
					return err
				}
			}
			return tx.Model(&models.File{}).
				Where("id = ? AND user_id = ?", req.Ids[0], userId).
				Updates(map[string]any{
					"parent_id": destParentID,
					"name":      req.DestinationName.Value,
				}).Error
		}
		items := pgtype.Array[string]{
			Elements: req.Ids,
			Valid:    true,
			Dims:     []pgtype.ArrayDimension{{Length: int32(len(req.Ids)), LowerBound: 1}},
		}
		if err := a.db.Model(&models.File{}).Where("id = any(?)", items).Where("user_id = ?", userId).
			Update("parent_id", destParentID).Error; err != nil {
			return err
		}

		var parentID string
		if srcFile.ParentId != nil {
			parentID = *srcFile.ParentId
		}

		var destParentIDStr string
		if destParentID != nil {
			destParentIDStr = *destParentID
		}

		a.events.Record(events.OpMove, userId, &models.Source{
			ID:           destParentIDStr,
			Type:         srcFile.Type,
			Name:         srcFile.Name,
			ParentID:     parentID,
			DestParentID: destParentIDStr,
		})
		return nil

	})
	if err != nil {
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

func (a *apiService) FilesUpdate(ctx context.Context, req *api.FileUpdate, params api.FilesUpdateParams) (*api.File, error) {

	userId := auth.GetUser(ctx)

	updateDb := models.File{}
	isContentUpdate := false
	uploadId := ""
	var uploads []models.Upload

	if req.UploadId.IsSet() && req.UploadId.Value != "" {
		uploadId = req.UploadId.Value
		if err := a.db.Where("upload_id = ?", uploadId).Order("part_no").Find(&uploads).Error; err != nil {
			return nil, &apiError{err: err}
		}
		var totalSize int64
		for _, u := range uploads {
			req.Parts = append(req.Parts, api.Part{
				ID:   u.PartId,
				Salt: api.NewOptString(u.Salt),
			})
			totalSize += u.Size
		}
		if req.Size.Value == 0 {
			req.Size.SetTo(totalSize)
		}
	}

	if req.Name.IsSet() && req.Name.Value != "" {
		updateDb.Name = req.Name.Value
	}

	if req.ParentId.IsSet() && req.ParentId.Value != "" {
		updateDb.ParentId = utils.Ptr(req.ParentId.Value)
	}

	if req.ChannelId.IsSet() && req.ChannelId.Value != 0 {
		updateDb.ChannelId = utils.Ptr(req.ChannelId.Value)
	}

	if req.Size.IsSet() && req.Size.Value != 0 && len(req.Parts) > 0 {
		updateDb.Parts = utils.Ptr(datatypes.NewJSONSlice(mapParts(req.Parts)))
		updateDb.Size = utils.Ptr(req.Size.Value)
		isContentUpdate = true
	}
	if req.Size.IsSet() && req.Size.Value == 0 {
		updateDb.Size = utils.Ptr(req.Size.Value)
		isContentUpdate = true
	}

	if req.Encrypted.IsSet() {
		updateDb.Encrypted = utils.Ptr(req.Encrypted.Value)
		isContentUpdate = true
	}

	// Update UpdatedAt if content changed OR if explicitly set (e.g., SetModTime)
	if isContentUpdate || req.UpdatedAt.IsSet() {
		if req.UpdatedAt.IsSet() && !req.UpdatedAt.Value.IsZero() {
			updateDb.UpdatedAt = utils.Ptr(req.UpdatedAt.Value)
		} else {
			updateDb.UpdatedAt = utils.Ptr(time.Now().UTC())
		}
	}

	// Use transaction for atomic update
	var file models.File
	err := a.db.Transaction(func(tx *gorm.DB) error {
		// Compute BLAKE3 tree hash if uploadId provided
		if uploadId != "" && len(uploads) > 0 {
			var allBlockHashes []byte
			for _, upload := range uploads {
				allBlockHashes = append(allBlockHashes, upload.BlockHashes...)
			}

			if len(allBlockHashes) > 0 {
				treeHashBytes := hash.ComputeTreeHash(allBlockHashes)
				treeHash := hash.SumToHex(treeHashBytes)
				updateDb.Hash = &treeHash
			}
		}

		// Build update query - explicitly select UpdatedAt if it's the only change
		query := tx.Model(&models.File{}).Where("id = ?", params.ID)
		if req.UpdatedAt.IsSet() && !isContentUpdate {
			// Force update of updated_at field even when only metadata changes
			query = query.Select("updated_at")
		}
		if err := query.Updates(updateDb).Error; err != nil {
			return err
		}

		// Delete uploads after successful update
		if uploadId != "" {
			if err := tx.Where("upload_id = ?", uploadId).Delete(&models.Upload{}).Error; err != nil {
				return err
			}
		}

		return tx.Where("id = ?", params.ID).First(&file).Error
	})

	if err != nil {
		return nil, &apiError{err: err}
	}

	keys := []string{cache.KeyFile(params.ID)}
	if len(req.Parts) > 0 {
		keys = append(keys, cache.KeyFileMessages(params.ID))
		a.cache.DeletePattern(ctx, cache.KeyFileLocationPattern(params.ID))
	}
	a.cache.Delete(ctx, keys...)

	var parentID string
	if file.ParentId != nil {
		parentID = *file.ParentId
	}

	a.events.Record(events.OpUpdate, userId, &models.Source{
		ID:       file.ID,
		Type:     file.Type,
		Name:     file.Name,
		ParentID: parentID,
	})
	return mapper.ToFileOut(file), nil
}

func (e *extendedService) FilesStream(w http.ResponseWriter, r *http.Request, fileId string, userId int64) {
	ctx := r.Context()
	logger := logging.Component("FILE").With(
		zap.String("file_id", fileId),
		zap.Int64("user_id", userId),
	)
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
			user, err = auth.VerifyUser(ctx, e.api.db, e.api.cache, e.api.cnf.JWT.Secret, cookie.Value)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
			}
			userId, _ := strconv.ParseInt(user.Subject, 10, 64)
			session = &models.Session{UserId: userId, Session: user.TgSession}
		} else {
			session, err = auth.GetSessionByHash(ctx, e.api.db, e.api.cache, authHash)
			if err != nil {
				http.Error(w, "invalid hash", http.StatusBadRequest)
				return
			}
		}
	} else {
		session = &models.Session{UserId: userId}
	}

	file, err := cache.Fetch(ctx, e.api.cache, cache.Key("files", fileId), 0, func() (*models.File, error) {
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
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", md5.FromString(fileId+strconv.FormatInt(*file.Size, 10))))
	w.Header().Set("Last-Modified", file.UpdatedAt.UTC().Format(http.TimeFormat))

	disposition := "inline"

	download := r.URL.Query().Get("download") == "1"

	if download {
		disposition = "attachment"
	}

	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": file.Name}))

	w.WriteHeader(status)

	if r.Method == http.MethodHead {
		return
	}

	tokens, err := e.api.channelManager.BotTokens(ctx, session.UserId)

	if err != nil {
		logger.Error("stream.bots_fetch_failed", zap.Error(err))
		http.Error(w, "failed to get bots", http.StatusInternalServerError)
		return
	}

	// Limit the number of bots used for streaming if configured
	if limit := e.api.cnf.TG.Stream.BotsLimit; limit > 0 && len(tokens) > limit {
		tokens = tokens[:limit]
	}

	var (
		lr     io.ReadCloser
		client *telegram.Client
		token  string
	)

	if len(tokens) == 0 {
		client, err = tgc.AuthClient(ctx, &e.api.cnf.TG, session.Session, e.api.newMiddlewares(ctx, 5)...)
		if err != nil {
			logger.Error("stream.auth_client_failed", zap.Error(err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	} else {
		token, _, err = e.api.botSelector.Next(ctx, tgc.BotOpStream, session.UserId, tokens)
		if err != nil {
			logger.Error("stream.bot_selection_failed", zap.Error(err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		client, err = tgc.BotClient(ctx, e.api.db, e.api.cache, &e.api.cnf.TG, token, e.api.newMiddlewares(ctx, 5)...)
		if err != nil {
			logger.Error("stream.bot_client_failed", zap.Error(err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	botID := strconv.FormatInt(session.UserId, 10)
	if token != "" {
		parts := strings.Split(token, ":")
		if len(parts) > 0 {
			botID = parts[0]
		}
	}

	if r.Method != http.MethodHead {
		handleStream := func() error {
			parts, err := getParts(ctx, client, e.api.cache, file)
			if err != nil {
				logger.Error("stream.parts_fetch_failed", zap.Error(err))
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return nil
			}

			lr, err = reader.NewReader(ctx,
				client.API(),
				e.api.cache,
				file,
				parts,
				start,
				end,
				&e.api.cnf.TG,
				botID,
			)

			if err != nil {
				logger.Error("stream.reader_create_failed", zap.Error(err))
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return nil
			}
			if lr == nil {
				logger.Error("stream.reader_nil")
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

func (a *apiService) FilesStream(ctx context.Context, params api.FilesStreamParams) (api.FilesStreamRes, error) {
	return nil, nil
}

func (a *apiService) SharesStream(ctx context.Context, params api.SharesStreamParams) (api.SharesStreamRes, error) {
	return nil, nil
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
