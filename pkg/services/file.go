package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/category"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/internal/database/types"
	"github.com/tgdrive/teldrive/internal/events"
	"github.com/tgdrive/teldrive/internal/hash"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/dto"
	"github.com/tgdrive/teldrive/pkg/mapper"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrorStreamAbandoned = errors.New("stream abandoned")
	defaultContentType   = "application/octet-stream"
)

func isUUID(str string) bool {
	_, err := uuid.Parse(str)
	return err == nil
}

func uploadTreeHash(uploads []jetmodel.Uploads) *string {
	var allBlockHashes []byte
	for _, upload := range uploads {
		if upload.BlockHashes != nil {
			allBlockHashes = append(allBlockHashes, (*upload.BlockHashes)...)
		}
	}

	if len(allBlockHashes) == 0 {
		return nil
	}

	treeHash := hash.SumToHex(hash.ComputeTreeHash(allBlockHashes))
	return &treeHash
}

func (a *apiService) FilesCategoryStats(ctx context.Context) ([]api.CategoryStats, error) {
	userId := auth.User(ctx)
	stats, err := a.repo.Files.CategoryStats(ctx, userId)
	if err != nil {
		return nil, &apiError{err: err}
	}

	return utils.Map(stats, func(item repositories.CategoryStats) api.CategoryStats {
		return api.CategoryStats{Category: api.Category(item.Category), TotalFiles: item.TotalFiles, TotalSize: item.TotalSize}
	}), nil
}

func (a *apiService) FilesCopy(ctx context.Context, req *api.FileCopy, params api.FilesCopyParams) (*api.File, error) {
	userId := auth.User(ctx)

	client, err := a.telegram.AuthClient(ctx, auth.JWTUser(ctx).TgSession, 5)
	if err != nil {
		return nil, &apiError{err: err}
	}

	file, err := a.repo.Files.GetByIDAndUser(ctx, uuid.UUID(params.ID), userId)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, &apiError{err: errors.New("file not found"), code: 404}
		}
		return nil, &apiError{err: err}
	}

	var sourceParts, newIds []api.Part

	for _, part := range file.Parts.Data {
		sourceParts = append(sourceParts, api.Part{ID: part.ID, Salt: api.NewOptString(part.Salt)})
	}

	channelId, err := a.channelManager.CurrentChannel(ctx, userId)
	if err != nil {
		return nil, &apiError{err: err}
	}

	err = a.telegram.RunWithAuth(ctx, client, "", func(ctx context.Context) error {
		copied, err := a.telegram.CopyFileParts(ctx, client, *file.ChannelID, channelId, sourceParts)
		if err != nil {
			return err
		}
		newIds = copied
		return nil
	})

	if err != nil {
		return nil, &apiError{err: err}
	}

	if len(newIds) != len(sourceParts) {
		return nil, &apiError{err: errors.New("failed to copy all file parts")}
	}

	var parentId string
	if !isUUID(req.Destination) {
		resolvedID, err := a.repo.Files.CreateDirectories(ctx, userId, req.Destination)
		if err != nil {
			return nil, &apiError{err: err}
		}
		if resolvedID == nil {
			return nil, &apiError{err: errors.New("destination path not found"), code: 404}
		}
		parentId = resolvedID.String()
	} else {
		parentId = req.Destination
	}

	parentUUID, err := uuid.Parse(parentId)
	if err != nil {
		return nil, &apiError{err: err, code: 400}
	}

	now := time.Now().UTC()
	updatedAt := now
	if req.UpdatedAt.IsSet() && !req.UpdatedAt.Value.IsZero() {
		updatedAt = req.UpdatedAt.Value
	}

	var dbParts types.Parts
	for _, part := range newIds {
		dbParts = append(dbParts, types.Part{ID: part.ID, Salt: part.Salt.Value})
	}
	newFile := &jetmodel.Files{
		ID:        uuid.New(),
		Name:      req.NewName.Or(file.Name),
		Type:      file.Type,
		MimeType:  file.MimeType,
		Size:      file.Size,
		UserID:    userId,
		Status:    utils.Ptr("active"),
		ChannelID: &channelId,
		Parts:     utils.Ptr(types.NewJSONB(dbParts)),
		Encrypted: file.Encrypted,
		Category:  file.Category,
		ParentID:  &parentUUID,
		Hash:      file.Hash,
		CreatedAt: now,
		UpdatedAt: updatedAt,
	}

	if err := a.repo.Files.Create(ctx, newFile); err != nil {
		return nil, &apiError{err: err}
	}

	a.events.Record(events.OpCopy, userId, &dto.Source{
		ID:       newFile.ID.String(),
		Type:     newFile.Type,
		Name:     newFile.Name,
		ParentID: parentId,
	})
	return mapper.ToJetFileOut(*newFile), nil
}

func (a *apiService) FilesCreate(ctx context.Context, fileIn *api.File) (*api.File, error) {
	userId := auth.User(ctx)

	var (
		parentID  *uuid.UUID
		path      string
		channelId int64
		uploadId  string
		uploads   []jetmodel.Uploads
	)

	if fileIn.Path.Value == "" && !fileIn.ParentId.IsSet() {
		return nil, &apiError{err: errors.New("parent id or path is required"), code: 409}
	}

	if fileIn.Path.Value != "" {
		path = strings.ReplaceAll(fileIn.Path.Value, "//", "/")

	}

	if path != "" && !fileIn.ParentId.IsSet() {
		resolvedID, err := a.repo.Files.ResolvePathID(ctx, path, userId)
		if err != nil {
			return nil, &apiError{err: err, code: 404}
		}
		parentID = resolvedID

	} else if fileIn.ParentId.IsSet() {
		parentUUID := uuid.UUID(fileIn.ParentId.Value)
		parentID = &parentUUID
	}

	fileDB := jetmodel.Files{ID: uuid.New(), UserID: userId, Encrypted: fileIn.Encrypted.Value}
	status := "active"
	fileDB.Status = &status
	fileDB.ParentID = parentID

	switch fileIn.Type {
	case api.FileTypeFolder:
		fileDB.MimeType = "drive/folder"
	case api.FileTypeFile:
		if fileIn.ChannelId.Value == 0 {
			resolvedChannelID, err := a.channelManager.CurrentChannel(ctx, userId)
			if err != nil {
				return nil, &apiError{err: err}
			}
			channelId = resolvedChannelID
		} else {
			channelId = fileIn.ChannelId.Value
		}
		fileDB.ChannelID = &channelId
		fileDB.MimeType = fileIn.MimeType.Value
		fileDB.Category = utils.Ptr(string(category.GetCategory(fileIn.Name)))

		var parts []api.Part
		if len(fileIn.Parts) > 0 {
			parts = fileIn.Parts
		} else if fileIn.UploadId.Value != "" {
			uploadId = fileIn.UploadId.Value
			fetchedUploads, err := a.repo.Uploads.GetByUploadID(ctx, uploadId)
			if err != nil {
				return nil, &apiError{err: err}
			}
			uploads = fetchedUploads
			for _, upload := range uploads {
				if upload.PartID == 0 {
					return nil, &apiError{err: errors.New("invalid part: part_id cannot be zero"), code: 400}
				}
			}

			for _, upload := range uploads {
				parts = append(parts, api.Part{
					ID: int(upload.PartID),
				})
				if upload.Salt != nil {
					parts[len(parts)-1].Salt = api.NewOptString(*upload.Salt)
				}
			}
		}

		if len(parts) > 0 {
			fileDB.Parts = mapper.ToDBPartsJSONB(parts)
		}

		// Compute BLAKE3 tree hash from block hashes if uploadId is provided
		if uploadId != "" && len(uploads) > 0 {
			fileDB.Hash = uploadTreeHash(uploads)
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
	fileDB.CreatedAt = time.Now().UTC()
	if fileIn.UpdatedAt.IsSet() && !fileIn.UpdatedAt.Value.IsZero() {
		fileDB.UpdatedAt = fileIn.UpdatedAt.Value
	} else {
		fileDB.UpdatedAt = time.Now().UTC()
	}

	if err := a.repo.WithTx(ctx, func(txCtx context.Context) error {
		if err := a.repo.Files.UpsertActive(txCtx, &fileDB); err != nil {
			return err
		}
		if uploadId != "" {
			if err := a.repo.Uploads.Delete(txCtx, uploadId); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return nil, &apiError{err: err}
	}

	parentIDStr := ""
	if fileDB.ParentID != nil {
		parentIDStr = fileDB.ParentID.String()
	}

	a.events.Record(events.OpCreate, userId, &dto.Source{
		ID:       fileDB.ID.String(),
		Type:     fileDB.Type,
		Name:     fileDB.Name,
		ParentID: parentIDStr,
	})
	return mapper.ToJetFileOut(fileDB), nil
}

func (a *apiService) FilesCreateShare(ctx context.Context, req *api.FileShareCreate, params api.FilesCreateShareParams) error {
	userId := auth.User(ctx)

	var fileShare jetmodel.FileShares

	if req.Password.Value != "" {
		bytes, err := bcrypt.GenerateFromPassword([]byte(req.Password.Value), bcrypt.MinCost)
		if err != nil {
			return &apiError{err: err}
		}
		fileShare.Password = utils.Ptr(string(bytes))
	}

	fileShare.ID = uuid.New()
	fileShare.FileID = uuid.UUID(params.ID)
	if req.ExpiresAt.IsSet() {
		fileShare.ExpiresAt = utils.Ptr(req.ExpiresAt.Value)
	}
	fileShare.UserID = userId

	if err := a.repo.Shares.Create(ctx, &fileShare); err != nil {
		return &apiError{err: err}
	}

	return nil
}

func (a *apiService) FilesDeleteById(ctx context.Context, params api.FilesDeleteByIdParams) error {

	userId := auth.User(ctx)

	req := &api.FileDelete{Ids: []api.UUID{params.ID}}

	var fileDB struct {
		ID       string
		Type     string
		Name     string
		ParentID *string
	}

	fileID := uuid.UUID(req.Ids[0])

	deleted, err := a.repo.Files.DeleteBulkReturning(ctx, []uuid.UUID{fileID}, userId, "trashed")
	if err != nil {
		return &apiError{err: err}
	}
	if len(deleted) == 0 {
		return &apiError{err: repositories.ErrNotFound}
	}
	firstFile := deleted[0]
	fileDB.ID = firstFile.ID.String()
	fileDB.Type = firstFile.Type
	fileDB.Name = firstFile.Name
	if firstFile.ParentID != nil {
		pid := firstFile.ParentID.String()
		fileDB.ParentID = &pid
	}

	keys := []string{}
	for _, id := range req.Ids {
		idStr := uuid.UUID(id).String()
		keys = append(keys, cache.KeyFile(idStr), cache.KeyFileMessages(idStr))
	}
	if len(keys) > 0 {
		a.cache.Delete(ctx, keys...)
	}

	var parentID string
	if fileDB.ParentID != nil {
		parentID = *fileDB.ParentID
	}

	a.events.Record(events.OpDelete, userId, &dto.Source{
		ID:       fileDB.ID,
		Type:     fileDB.Type,
		Name:     fileDB.Name,
		ParentID: parentID,
	})

	return nil
}

func (a *apiService) FilesDelete(ctx context.Context, req *api.FileDelete, params api.FilesDeleteParams) error {
	userId := auth.User(ctx)
	if len(req.Ids) == 0 {
		return &apiError{err: errors.New("ids should not be empty"), code: 409}
	}
	ids := make([]uuid.UUID, 0, len(req.Ids))
	for _, id := range req.Ids {
		ids = append(ids, uuid.UUID(id))
	}

	targetStatus := "trashed"
	if params.Force.Value {
		targetStatus = "purge_pending"
	}

	deleted, err := a.repo.Files.DeleteBulkReturning(ctx, ids, userId, targetStatus)
	if err != nil {
		return &apiError{err: err}
	}

	keys := make([]string, 0, len(deleted)*2)
	for _, item := range deleted {
		idStr := item.ID.String()
		keys = append(keys, cache.KeyFile(idStr), cache.KeyFileMessages(idStr))
	}
	if len(keys) > 0 {
		a.cache.Delete(ctx, keys...)
	}

	return nil
}

func (a *apiService) FilesDeleteShare(ctx context.Context, params api.FilesDeleteShareParams) error {
	if err := a.repo.Shares.Delete(ctx, uuid.UUID(params.ShareId)); err != nil {
		return &apiError{err: err}
	}
	a.cache.Delete(ctx, cache.KeyShare(uuid.UUID(params.ShareId).String()))
	return nil
}

func (a *apiService) FilesEditShare(ctx context.Context, req *api.FileShareCreate, params api.FilesEditShareParams) error {
	update := repositories.ShareUpdate{}

	if req.Password.Value != "" {
		bytes, err := bcrypt.GenerateFromPassword([]byte(req.Password.Value), bcrypt.MinCost)
		if err != nil {
			return &apiError{err: err}
		}
		update.Password = utils.Ptr(string(bytes))
	}
	if req.ExpiresAt.IsSet() {
		update.ExpiresAt = utils.Ptr(req.ExpiresAt.Value)
	}

	if err := a.repo.Shares.Update(ctx, uuid.UUID(params.ID), update); err != nil {
		return &apiError{err: err}
	}

	return nil
}

func (a *apiService) FilesGetById(ctx context.Context, params api.FilesGetByIdParams) (*api.File, error) {

	file, err := a.repo.Files.GetByID(ctx, uuid.UUID(params.ID))
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, &apiError{err: errors.New("file not found"), code: 404}
		}
		return nil, &apiError{err: err}
	}

	path, err := a.repo.Files.GetFullPath(ctx, uuid.UUID(params.ID))
	if err != nil {
		return nil, &apiError{err: err}
	}

	res := mapper.ToJetFileOut(*file)
	res.Path = api.NewOptString(path)

	return res, nil
}

func (a *apiService) FilesList(ctx context.Context, params api.FilesListParams) (*api.FileList, error) {
	userId := auth.User(ctx)

	qParams := repositories.FileQueryParams{
		UserID:    userId,
		Operation: string(params.Operation.Value),
		Status:    string(params.Status.Value),
		ParentID: func() string {
			if !params.ParentId.IsSet() {
				return ""
			}
			return uuid.UUID(params.ParentId.Value).String()
		}(),
		Path:       params.Path.Value,
		Name:       params.Name.Value,
		Type:       string(params.Type.Value),
		Category:   utils.Map(params.Category, func(c api.Category) string { return string(c) }),
		Query:      params.Query.Value,
		SearchType: string(params.SearchType.Value),
		DeepSearch: params.DeepSearch.Value,
		UpdatedAt:  params.UpdatedAt.Value,
		Shared:     params.Shared.Value,
		Sort:       string(params.Sort.Value),
		Order:      string(params.Order.Value),
		Cursor:     params.Cursor.Value,
		Limit:      params.Limit.Value,
	}

	res, err := a.repo.Files.List(ctx, qParams)
	if err != nil {
		return nil, &apiError{err: err}
	}

	files := utils.Map(res, func(item jetmodel.Files) api.File {
		return *mapper.ToJetFileOut(item)
	})

	var nextCursor api.OptString
	if len(res) > 0 && len(res) == qParams.Limit {
		last := res[len(res)-1]
		var cursorVal string
		switch strings.ToLower(string(params.Sort.Value)) {
		case "name":
			cursorVal = last.Name
		case "size":
			cursorVal = strconv.FormatInt(*last.Size, 10)
		case "id":
			cursorVal = last.ID.String()
		default: // updated_at
			cursorVal = last.UpdatedAt.Format(time.RFC3339Nano)
		}
		nextCursor.SetTo(fmt.Sprintf("%s:%s", cursorVal, last.ID.String()))
	}

	return &api.FileList{
		Items: files,
		Meta:  api.Meta{NextCursor: nextCursor},
	}, nil
}

func (a *apiService) FilesChildren(ctx context.Context, params api.FilesChildrenParams) (*api.FileList, error) {
	listParams := api.FilesListParams{
		Name:       params.Name,
		Query:      params.Query,
		SearchType: params.SearchType,
		Type:       params.Type,
		Path:       params.Path,
		Operation:  params.Operation,
		Status:     params.Status,
		DeepSearch: params.DeepSearch,
		Shared:     params.Shared,
		ParentId:   api.NewOptUUID(params.ID),
		Category:   params.Category,
		UpdatedAt:  params.UpdatedAt,
		Sort:       params.Sort,
		Order:      params.Order,
		Limit:      params.Limit,
		Cursor:     params.Cursor,
	}

	return a.FilesList(ctx, listParams)
}

func (a *apiService) FilesRestore(ctx context.Context, params api.FilesRestoreParams) error {
	file, err := a.repo.Files.GetByID(ctx, uuid.UUID(params.ID))
	if err != nil {
		return &apiError{err: err}
	}
	if file.Status == nil || *file.Status != "trashed" {
		return &apiError{err: errors.New("only trashed files can be restored"), code: 409}
	}
	status := "active"
	if _, err := a.repo.Files.UpdateReturning(ctx, uuid.UUID(params.ID), repositories.FileUpdate{Status: &status}); err != nil {
		return &apiError{err: err}
	}

	return nil
}

func (a *apiService) FilesStreamHead(ctx context.Context, params api.FilesStreamHeadParams) (api.FilesStreamHeadRes, error) {
	_ = ctx
	_ = params
	return &api.FilesStreamHeadOKHeaders{}, nil
}

func (a *apiService) FilesMkdir(ctx context.Context, path string) error {
	userId := auth.User(ctx)

	if _, err := a.repo.Files.CreateDirectories(ctx, userId, path); err != nil {
		return &apiError{err: err}
	}
	return nil
}

func (a *apiService) FilesMove(ctx context.Context, req *api.FileMove) error {
	userId := auth.User(ctx)

	var destParentID *uuid.UUID

	if !isUUID(req.DestinationParent) {
		r, err := a.repo.Files.ResolvePathID(ctx, req.DestinationParent, userId)
		if err != nil {
			return &apiError{err: err}
		}
		destParentID = r

	} else {
		parsedParentID, err := uuid.Parse(req.DestinationParent)
		if err != nil {
			return &apiError{err: err, code: 400}
		}
		destParentID = &parsedParentID
	}

	if len(req.Ids) == 0 {
		return nil
	}

	srcID := uuid.UUID(req.Ids[0])

	ids := make([]uuid.UUID, 0, len(req.Ids))
	for _, id := range req.Ids {
		ids = append(ids, uuid.UUID(id))
	}

	var srcFile *jetmodel.Files
	err := a.repo.WithTx(ctx, func(txCtx context.Context) error {
		fetched, err := a.repo.Files.GetByIDAndUser(txCtx, srcID, userId)
		if err != nil {
			return err
		}
		srcFile = fetched

		if len(req.Ids) == 1 && req.DestinationName.Value != "" {
			existing, err := a.repo.Files.GetActiveByNameAndParent(txCtx, userId, req.DestinationName.Value, destParentID)
			if err == nil && existing.ID != srcFile.ID {
				if err := a.repo.Files.Delete(txCtx, []uuid.UUID{existing.ID}); err != nil {
					return err
				}
			}

			moved, err := a.repo.Files.MoveSingleReturning(txCtx, srcID, userId, destParentID, &req.DestinationName.Value)
			if err != nil {
				return err
			}
			srcFile = moved
			return nil
		}

		moved, err := a.repo.Files.MoveBulkReturning(txCtx, ids, userId, destParentID)
		if err != nil {
			return err
		}
		for i := range moved {
			if moved[i].ID == srcID {
				srcFile = &moved[i]
				break
			}
		}

		return nil
	})
	if err != nil {
		return &apiError{err: err}
	}

	parentID := ""
	if srcFile.ParentID != nil {
		parentID = srcFile.ParentID.String()
	}

	destParentIDStr := ""
	if destParentID != nil {
		destParentIDStr = destParentID.String()
	}

	a.events.Record(events.OpMove, userId, &dto.Source{
		ID:           srcFile.ID.String(),
		Type:         srcFile.Type,
		Name:         srcFile.Name,
		ParentID:     parentID,
		DestParentID: destParentIDStr,
	})

	return nil

}

func (a *apiService) FilesListShares(ctx context.Context, params api.FilesListSharesParams) ([]api.FileShare, error) {
	fileID := uuid.UUID(params.ID)

	result, err := a.repo.Shares.GetByFileID(ctx, fileID)
	if err != nil {
		return nil, &apiError{err: err}
	}

	res := make([]api.FileShare, 0, len(result))
	for _, item := range result {
		share := api.FileShare{ID: api.UUID(item.ID)}
		if item.Password != nil {
			share.Protected = true
		}
		if item.ExpiresAt != nil {
			share.ExpiresAt = api.NewOptDateTime(*item.ExpiresAt)
		}
		res = append(res, share)
	}

	return res, nil
}

func (a *apiService) FilesUpdate(ctx context.Context, req *api.FileUpdate, params api.FilesUpdateParams) (*api.File, error) {

	userId := auth.User(ctx)
	var err error

	update := repositories.FileUpdate{}
	isContentUpdate := false
	uploadId := ""
	var uploads []jetmodel.Uploads

	if req.UploadId.IsSet() && req.UploadId.Value != "" {
		uploadId = req.UploadId.Value
		if uploads, err = a.repo.Uploads.GetByUploadID(ctx, uploadId); err != nil {
			return nil, &apiError{err: err}
		}
		var totalSize int64
		for _, u := range uploads {
			req.Parts = append(req.Parts, api.Part{ID: int(u.PartID)})
			if u.Salt != nil {
				req.Parts[len(req.Parts)-1].Salt = api.NewOptString(*u.Salt)
			}
			totalSize += u.Size
		}
		if req.Size.Value == 0 {
			req.Size.SetTo(totalSize)
		}
	}

	if req.Name.IsSet() && req.Name.Value != "" {
		update.Name = &req.Name.Value
	}

	if req.ParentId.IsSet() {
		parentUUID := uuid.UUID(req.ParentId.Value)
		update.ParentID = &parentUUID
	}

	if req.ChannelId.IsSet() && req.ChannelId.Value != 0 {
		update.ChannelID = &req.ChannelId.Value
	}

	if req.Size.IsSet() && req.Size.Value != 0 && len(req.Parts) > 0 {
		update.Parts = mapper.ToDBPartsJSONB(req.Parts)
		update.Size = &req.Size.Value
		isContentUpdate = true
	}
	if req.Size.IsSet() && req.Size.Value == 0 {
		update.Size = &req.Size.Value
		isContentUpdate = true
	}

	if req.Encrypted.IsSet() {
		update.Encrypted = &req.Encrypted.Value
		isContentUpdate = true
	}

	// Update UpdatedAt if content changed OR if explicitly set (e.g., SetModTime)
	if isContentUpdate || req.UpdatedAt.IsSet() {
		if req.UpdatedAt.IsSet() && !req.UpdatedAt.Value.IsZero() {
			update.UpdatedAt = &req.UpdatedAt.Value
		} else {
			now := time.Now().UTC()
			update.UpdatedAt = &now
		}
	}
	if uploadId != "" && len(uploads) > 0 {
		update.Hash = uploadTreeHash(uploads)
	}

	fileUUID := uuid.UUID(params.ID)

	var file *jetmodel.Files
	if err := a.repo.WithTx(ctx, func(txCtx context.Context) error {
		updated, err := a.repo.Files.UpdateReturning(txCtx, fileUUID, update)
		if err != nil {
			return err
		}
		file = updated
		if uploadId != "" {
			if err := a.repo.Uploads.Delete(txCtx, uploadId); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return nil, &apiError{err: err}
	}

	fileIDStr := fileUUID.String()
	keys := []string{cache.KeyFile(fileIDStr)}
	if len(req.Parts) > 0 {
		keys = append(keys, cache.KeyFileMessages(fileIDStr))
		a.cache.DeletePattern(ctx, cache.KeyFileLocationPattern(fileIDStr))
	}
	a.cache.Delete(ctx, keys...)

	var parentID string
	if file.ParentID != nil {
		parentID = file.ParentID.String()
	}

	a.events.Record(events.OpUpdate, userId, &dto.Source{
		ID:       file.ID.String(),
		Type:     file.Type,
		Name:     file.Name,
		ParentID: parentID,
	})
	return mapper.ToJetFileOut(*file), nil
}
