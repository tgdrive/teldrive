package services

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
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
	"github.com/tgdrive/teldrive/internal/http_range"
	"github.com/tgdrive/teldrive/internal/md5"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/constants"
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

	parentID, err := a.resolveParentID(ctx, fileIn, userId)
	if err != nil {
		return nil, err
	}

	fileDB := jetmodel.Files{ID: uuid.New(), UserID: userId, Encrypted: fileIn.Encrypted.Value}
	fileDB.Status = utils.Ptr(constants.FileStatusActive.String())
	fileDB.ParentID = parentID

	var uploadId string
	switch fileIn.Type {
	case api.FileTypeFolder:
		fileDB.MimeType = "drive/folder"
	case api.FileTypeFile:
		var err error
		uploadId, _, err = a.prepareFileData(ctx, fileIn, &fileDB, userId)
		if err != nil {
			return nil, &apiError{err: err}
		}
	}

	fileDB.Name = fileIn.Name
	fileDB.Type = string(fileIn.Type)
	fileDB.CreatedAt = time.Now().UTC()
	if fileIn.UpdatedAt.IsSet() && !fileIn.UpdatedAt.Value.IsZero() {
		fileDB.UpdatedAt = fileIn.UpdatedAt.Value
	} else {
		fileDB.UpdatedAt = time.Now().UTC()
	}

	if err := a.persistAndCleanup(ctx, fileDB, uploadId, userId); err != nil {
		return nil, &apiError{err: err}
	}

	return mapper.ToJetFileOut(fileDB), nil
}

// resolveParentID resolves the parent ID from either a path or direct input.
func (a *apiService) resolveParentID(ctx context.Context, fileIn *api.File, userId int64) (*uuid.UUID, error) {
	if fileIn.Path.Value == "" && !fileIn.ParentId.IsSet() {
		return nil, &apiError{err: errors.New("parent id or path is required"), code: 409}
	}

	if fileIn.Path.Value != "" {
		path := strings.ReplaceAll(fileIn.Path.Value, "//", "/")
		resolvedID, err := a.repo.Files.ResolvePathID(ctx, path, userId)
		if err != nil {
			return nil, &apiError{err: err, code: 404}
		}
		return resolvedID, nil
	}

	if fileIn.ParentId.IsSet() {
		parentUUID := uuid.UUID(fileIn.ParentId.Value)
		return &parentUUID, nil
	}

	return nil, nil
}

// prepareFileData prepares file-specific data (FileTypeFile only) including
// channel resolution, parts handling, uploads, and hash computation.
func (a *apiService) prepareFileData(ctx context.Context, fileIn *api.File, fileDB *jetmodel.Files, userId int64) (uploadId string, uploads []jetmodel.Uploads, err error) {
	if fileIn.ChannelId.Value == 0 {
		resolvedChannelID, err := a.channelManager.CurrentChannel(ctx, userId)
		if err != nil {
			return "", nil, err
		}
		fileDB.ChannelID = &resolvedChannelID
	} else {
		fileDB.ChannelID = &fileIn.ChannelId.Value
	}
	fileDB.MimeType = fileIn.MimeType.Value
	fileDB.Category = utils.Ptr(string(category.GetCategory(fileIn.Name)))

	var parts []api.Part
	if len(fileIn.Parts) > 0 {
		parts = fileIn.Parts
	} else if fileIn.UploadId.Value != "" {
		uploadId = fileIn.UploadId.Value
		fetchedUploads, err := a.repo.Uploads.GetByUploadID(ctx, uploadId)
		if err != nil {
			return "", nil, err
		}
		uploads = fetchedUploads
		for _, upload := range uploads {
			if upload.PartID == 0 {
				return "", nil, errors.New("invalid part: part_id cannot be zero")
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
	return uploadId, uploads, nil
}

// persistAndCleanup handles transaction persistence, cache invalidation, and event recording.
func (a *apiService) persistAndCleanup(ctx context.Context, fileDB jetmodel.Files, uploadId string, userId int64) error {
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
		return err
	}

	fileIDStr := fileDB.ID.String()
	keys := []string{cache.KeyFile(fileIDStr)}
	if fileDB.Parts != nil {
		keys = append(keys, cache.KeyFileMessages(fileIDStr))
		a.cache.DeletePattern(ctx, cache.KeyFileLocationPattern(fileIDStr))
	}
	a.cache.Delete(ctx, keys...)

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
	return nil
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

	deleted, err := a.repo.Files.DeleteBulkReturning(ctx, []uuid.UUID{fileID}, userId, "pending_deletion")
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

func (a *apiService) FilesDelete(ctx context.Context, req *api.FileDelete) error {
	userId := auth.User(ctx)
	if len(req.Ids) == 0 {
		return &apiError{err: errors.New("ids should not be empty"), code: 409}
	}
	ids := make([]uuid.UUID, 0, len(req.Ids))
	for _, id := range req.Ids {
		ids = append(ids, uuid.UUID(id))
	}

	deleted, err := a.repo.Files.DeleteBulkReturning(ctx, ids, userId, "pending_deletion")
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

func (a *apiService) FilesStreamHead(ctx context.Context, params api.FilesStreamHeadParams) (api.FilesStreamHeadRes, error) {
	userID := auth.User(ctx)
	fileID := uuid.UUID(params.ID)

	file, err := cache.Fetch(ctx, a.cache, cache.KeyFile(fileID.String()), 0, func() (*jetmodel.Files, error) {
		return a.repo.Files.GetByIDAndUser(ctx, fileID, userID)
	})
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, &apiError{err: errors.New("file not found"), code: 404}
		}
		return nil, &apiError{err: err}
	}

	contentLength := int64(0)
	if file.Size != nil {
		contentLength = *file.Size
	}
	etag := fmt.Sprintf("\"%s\"", md5.FromString(fileID.String()+strconv.FormatInt(contentLength, 10)))

	disposition := "inline"
	if v, ok := params.Download.Get(); ok && v == api.FilesStreamHeadDownload1 {
		disposition = "attachment"
	}
	contentDisposition := mime.FormatMediaType(disposition, map[string]string{"filename": file.Name})
	lastModified := file.UpdatedAt.UTC()

	if rawRange, ok := params.Range.Get(); ok && rawRange != "" && contentLength > 0 {
		ranges, err := http_range.Parse(rawRange, contentLength)
		if err == http_range.ErrNoOverlap {
			return nil, &apiError{err: err, code: http.StatusRequestedRangeNotSatisfiable}
		}
		if err != nil {
			return nil, &apiError{err: err, code: http.StatusBadRequest}
		}
		if len(ranges) > 1 {
			return nil, &apiError{err: fmt.Errorf("multiple ranges are not supported"), code: http.StatusRequestedRangeNotSatisfiable}
		}

		start := ranges[0].Start
		end := ranges[0].End
		return &api.FilesStreamHeadPartialContent{
			AcceptRanges:       api.FilesStreamHeadPartialContentAcceptRangesBytes,
			ContentDisposition: contentDisposition,
			ContentLength:      strconv.FormatInt(end-start+1, 10),
			ContentRange:       api.NewOptString(fmt.Sprintf("bytes %d-%d/%d", start, end, contentLength)),
			Etag:               etag,
			LastModified:       lastModified,
		}, nil
	}

	return &api.FilesStreamHeadOK{
		AcceptRanges:       api.FilesStreamHeadOKAcceptRangesBytes,
		ContentDisposition: contentDisposition,
		ContentLength:      strconv.FormatInt(contentLength, 10),
		Etag:               etag,
		LastModified:       lastModified,
	}, nil
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

	update, uploadId, err := a.buildFileUpdate(ctx, req)
	if err != nil {
		return nil, &apiError{err: err}
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

	a.invalidateFileCache(ctx, fileUUID.String(), len(req.Parts) > 0)

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

func (a *apiService) buildFileUpdate(ctx context.Context, req *api.FileUpdate) (repositories.FileUpdate, string, error) {
	update := repositories.FileUpdate{}
	uploadId := ""
	var uploads []jetmodel.Uploads
	var err error

	if req.UploadId.IsSet() && req.UploadId.Value != "" {
		uploadId = req.UploadId.Value
		if uploads, err = a.repo.Uploads.GetByUploadID(ctx, uploadId); err != nil {
			return repositories.FileUpdate{}, "", err
		}
		totalSize, parts := a.buildPartsFromUploads(uploads)
		req.Parts = parts
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

	update = a.applyContentUpdate(req, update, uploads)

	return update, uploadId, nil
}

func (a *apiService) buildPartsFromUploads(uploads []jetmodel.Uploads) (int64, []api.Part) {
	var totalSize int64
	var parts []api.Part
	for _, u := range uploads {
		part := api.Part{ID: int(u.PartID)}
		if u.Salt != nil {
			part.Salt = api.NewOptString(*u.Salt)
		}
		parts = append(parts, part)
		totalSize += u.Size
	}
	return totalSize, parts
}

func (a *apiService) applyContentUpdate(req *api.FileUpdate, update repositories.FileUpdate, uploads []jetmodel.Uploads) repositories.FileUpdate {
	isContentUpdate := false
	uploadId := ""
	if req.UploadId.IsSet() {
		uploadId = req.UploadId.Value
	}

	if req.Size.IsSet() && req.Size.Value == 0 {
		update.Size = &req.Size.Value
		isContentUpdate = true
	} else if req.Size.IsSet() && req.Size.Value != 0 && len(req.Parts) > 0 {
		update.Parts = mapper.ToDBPartsJSONB(req.Parts)
		update.Size = &req.Size.Value
		isContentUpdate = true
	}

	if req.Encrypted.IsSet() {
		update.Encrypted = &req.Encrypted.Value
		isContentUpdate = true
	}

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

	return update
}

func (a *apiService) invalidateFileCache(ctx context.Context, fileID string, hasParts bool) {
	keys := []string{cache.KeyFile(fileID)}
	if hasParts {
		keys = append(keys, cache.KeyFileMessages(fileID))
		a.cache.DeletePattern(ctx, cache.KeyFileLocationPattern(fileID))
	}
	a.cache.Delete(ctx, keys...)
}
