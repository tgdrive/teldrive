package api

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/shares"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

func (h *Handler) HealthLive(ctx context.Context) (*gen.HealthStatus, error) {
	if h.Health == nil {
		return nil, problem(503, "service_unavailable", "health service is unavailable", ErrOperationUnavailable)
	}
	status := h.Health.Live()
	return &gen.HealthStatus{Status: gen.HealthStatusStatus(status.State), Version: status.Version}, nil
}

func (h *Handler) HealthReady(ctx context.Context) (gen.HealthReadyRes, error) {
	if h.Health == nil {
		return nil, problem(503, "service_unavailable", "health service is unavailable", ErrOperationUnavailable)
	}
	status, err := h.Health.Ready(ctx)
	if err != nil {
		return nil, problem(503, "not_ready", "service is not ready", err)
	}
	return &gen.HealthStatus{Status: gen.HealthStatusStatus(status.State), Version: status.Version}, nil
}

func (h *Handler) CreateFolder(ctx context.Context, req *gen.FolderCreateRequest, params gen.CreateFolderParams) (gen.CreateFolderRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Catalog == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	if policy, ok := req.ConflictPolicy.Get(); ok && string(policy) != "fail" {
		return nil, mapServiceError(uploads.ErrUnsupportedConflictPolicy)
	}
	var modTime time.Time
	if value, ok := req.ModTime.Get(); ok {
		modTime = value
	}
	parentID := optionalGoogleUUID(req.ParentId)
	ownerID := userID
	if parentID != nil {
		access, err := h.resolveAuthenticatedFileAccess(ctx, *parentID, true)
		if err != nil {
			return nil, mapServiceError(err)
		}
		ownerID = access.OwnerID
	}
	file, err := h.Catalog.CreateFolder(ctx, catalog.CreateFolderInput{
		UserID: ownerID, ParentID: parentID, Name: req.Name, ModTime: modTime,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	entry, err := fileEntry(file)
	if err != nil {
		return nil, mapServiceError(err)
	}
	fileID := googleUUID(entry.ID)
	return &gen.CopyFileCreatedHeaders{
		Etag: generationETag(file.Generation), Location: gen.URI(url.URL{Path: "/files/" + fileID.String()}),
		Response: entry,
	}, nil
}

func (h *Handler) GetFile(ctx context.Context, params gen.GetFileParams) (gen.GetFileRes, error) {
	if h.Catalog == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	access, err := h.resolveAuthenticatedFileAccess(ctx, googleUUID(params.FileId), false)
	if err != nil {
		return nil, mapServiceError(err)
	}
	file, err := h.Catalog.Get(ctx, access.OwnerID, googleUUID(params.FileId))
	if err != nil {
		return nil, mapServiceError(err)
	}
	entry, err := fileEntry(file)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.FileEntryHeaders{Etag: generationETag(file.Generation), Response: entry}, nil
}

func (h *Handler) ListFiles(ctx context.Context, params gen.ListFilesParams) (gen.ListFilesRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Catalog == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	var cursor fileCursor
	if err := decodeCursor(params.Cursor, &cursor); err != nil {
		return nil, mapServiceError(uploads.ErrInvalidInput)
	}
	limit := params.Limit.Or(100)
	sortBy, order, searchType := "name", "asc", "text"
	if value, ok := params.Sort.Get(); ok {
		sortBy = string(value)
	}
	if value, ok := params.Order.Get(); ok {
		order = string(value)
	}
	if value, ok := params.SearchType.Get(); ok {
		searchType = string(value)
	}
	if cursor.Sort != "" && (cursor.Sort != sortBy || cursor.Order != order) {
		return nil, mapServiceError(uploads.ErrInvalidInput)
	}
	parentID := optionalGoogleUUID(params.ParentId)
	ownerID := userID
	if parentID != nil {
		access, err := h.resolveAuthenticatedFileAccess(ctx, *parentID, false)
		if err != nil {
			return nil, mapServiceError(err)
		}
		ownerID = access.OwnerID
	}
	input := catalog.ListInput{
		UserID: ownerID, ParentID: parentID, Path: params.Path.Or(""),
		Search: params.Search.Or(""), SearchType: searchType,
		Sort: sortBy, Order: order, Limit: limit,
	}
	if value, ok := params.Kind.Get(); ok {
		kind := sqlcgen.FileKind(value)
		input.Kind = &kind
	}
	if value, ok := params.Status.Get(); ok {
		input.Status = sqlcgen.FileStatus(value)
	}
	for _, category := range params.Category {
		input.Categories = append(input.Categories, string(category))
	}
	if value, ok := params.UpdatedAfter.Get(); ok {
		input.UpdatedAfter = &value
	}
	if value, ok := params.UpdatedBefore.Get(); ok {
		input.UpdatedBefore = &value
	}
	if cursor.ID != uuid.Nil {
		input.AfterID = &cursor.ID
		input.AfterName = cursor.Name
		input.AfterValue = cursor.Value
	}
	files, err := h.Catalog.List(ctx, input)
	if err != nil {
		return nil, mapServiceError(err)
	}
	items := make([]gen.FileEntry, 0, len(files))
	for _, file := range files {
		entry, err := fileEntry(file)
		if err != nil {
			return nil, mapServiceError(err)
		}
		items = append(items, entry)
	}
	response := gen.ListFilesOK{Items: items}
	if len(files) == int(limit) && len(files) > 0 {
		last := files[len(files)-1]
		lastID, _ := dbtypes.GoogleUUID(last.ID)
		response.NextCursor = encodeCursor(fileCursor{
			Name: last.NormalizedName, Sort: sortBy, Order: order,
			Value: catalog.FileCursorValue(last, sortBy), ID: lastID,
		})
	}
	return &response, nil
}

func (h *Handler) UpdateFile(ctx context.Context, req *gen.FileUpdateRequest, params gen.UpdateFileParams) (gen.UpdateFileRes, error) {
	if h.Catalog == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	generation, err := parseGenerationETag(params.IfMatch)
	if err != nil {
		return nil, mapServiceError(uploads.ErrInvalidInput)
	}
	var name *string
	if value, ok := req.Name.Get(); ok {
		name = &value
	}
	var modTime *time.Time
	if value, ok := req.ModTime.Get(); ok {
		modTime = &value
	}
	access, err := h.resolveAuthenticatedFileAccess(ctx, googleUUID(params.FileId), true)
	if err != nil {
		return nil, mapServiceError(err)
	}
	file, err := h.Catalog.Update(ctx, catalog.UpdateInput{
		UserID: access.OwnerID, FileID: googleUUID(params.FileId), ExpectedGeneration: generation, Name: name, ModTime: modTime,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	entry, err := fileEntry(file)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.FileEntryHeaders{Etag: generationETag(file.Generation), Response: entry}, nil
}

func (h *Handler) MoveFile(ctx context.Context, req *gen.FileMoveRequest, params gen.MoveFileParams) (gen.MoveFileRes, error) {
	if h.Catalog == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	policy := "fail"
	if value, ok := req.ConflictPolicy.Get(); ok {
		policy = string(value)
	}
	generation, err := parseGenerationETag(params.IfMatch)
	if err != nil {
		return nil, mapServiceError(uploads.ErrInvalidInput)
	}
	sourceAccess, err := h.resolveAuthenticatedFileAccess(ctx, googleUUID(params.FileId), true)
	if err != nil {
		return nil, mapServiceError(err)
	}
	parentID := optionalGoogleUUID(req.ParentId)
	if parentID == nil {
		if !sourceAccess.Owned {
			return nil, mapServiceError(shares.ErrForbidden)
		}
	} else {
		destinationAccess, err := h.resolveAuthenticatedFileAccess(ctx, *parentID, true)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if destinationAccess.OwnerID != sourceAccess.OwnerID {
			return nil, mapServiceError(shares.ErrForbidden)
		}
	}
	file, err := h.Catalog.MoveWithPolicy(ctx, sourceAccess.OwnerID, googleUUID(params.FileId), parentID, generation, policy)
	if err != nil {
		return nil, mapServiceError(err)
	}
	entry, err := fileEntry(file)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.FileEntryHeaders{Etag: generationETag(file.Generation), Response: entry}, nil
}

func (h *Handler) TrashFile(ctx context.Context, params gen.TrashFileParams) (gen.TrashFileRes, error) {
	if h.Catalog == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	fileID := googleUUID(params.FileId)
	access, err := h.resolveAuthenticatedFileAccess(ctx, fileID, true)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if !access.Owned && access.RootFileID == fileID {
		return nil, mapServiceError(shares.ErrForbidden)
	}
	if _, err := h.Catalog.Trash(ctx, access.OwnerID, fileID); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.TrashFileNoContent{}, nil
}

func (h *Handler) RestoreFile(ctx context.Context, params gen.RestoreFileParams) (gen.RestoreFileRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Catalog == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	file, err := h.Catalog.Restore(ctx, userID, googleUUID(params.FileId))
	if err != nil {
		return nil, mapServiceError(err)
	}
	entry, err := fileEntry(file)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.FileEntryHeaders{Etag: generationETag(file.Generation), Response: entry}, nil
}

func (h *Handler) CreateUpload(ctx context.Context, req *gen.UploadCreateRequest, params gen.CreateUploadParams) (gen.CreateUploadRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Uploads == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	parentID := optionalGoogleUUID(req.ParentId)
	ownerID := userID
	if parentID != nil {
		access, err := h.resolveAuthenticatedFileAccess(ctx, *parentID, true)
		if err != nil {
			return nil, mapServiceError(err)
		}
		ownerID = access.OwnerID
	}
	session, err := h.createUploadForOwner(ctx, ownerID, parentID, req)
	if err != nil {
		return nil, err
	}
	response, err := uploadSession(session)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &response, nil
}

func (h *Handler) GetUpload(ctx context.Context, params gen.GetUploadParams) (gen.GetUploadRes, error) {
	if h.Uploads == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	ownerID, err := h.resolveAuthenticatedUploadOwner(ctx, googleUUID(params.UploadId), false)
	if err != nil {
		return nil, mapServiceError(err)
	}
	session, err := h.Uploads.Get(ctx, ownerID, googleUUID(params.UploadId))
	if err != nil {
		return nil, mapServiceError(err)
	}
	response, err := uploadSession(session)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &response, nil
}

func (h *Handler) ListUploads(ctx context.Context, params gen.ListUploadsParams) (gen.ListUploadsRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Uploads == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	var cursor uploadCursor
	if err := decodeCursor(params.Cursor, &cursor); err != nil {
		return nil, mapServiceError(uploads.ErrInvalidInput)
	}
	input := uploads.ListInput{UserID: userID, Limit: params.Limit.Or(100)}
	if value, ok := params.State.Get(); ok {
		state := sqlcgen.UploadState(value)
		input.State = &state
	}
	if cursor.ID != uuid.Nil {
		input.AfterID, input.AfterCreatedAt = &cursor.ID, &cursor.CreatedAt
	}
	sessions, err := h.Uploads.List(ctx, input)
	if err != nil {
		return nil, mapServiceError(err)
	}
	items := make([]gen.UploadSession, 0, len(sessions))
	for _, session := range sessions {
		item, err := uploadSession(session)
		if err != nil {
			return nil, mapServiceError(err)
		}
		items = append(items, item)
	}
	response := gen.ListUploadsOK{Items: items}
	if len(sessions) == int(input.Limit) && len(sessions) > 0 {
		last := sessions[len(sessions)-1]
		lastID, _ := dbtypes.GoogleUUID(last.ID)
		response.NextCursor = encodeCursor(uploadCursor{CreatedAt: last.CreatedAt.Time, ID: lastID})
	}
	return &response, nil
}

func (h *Handler) ListUploadParts(ctx context.Context, params gen.ListUploadPartsParams) (gen.ListUploadPartsRes, error) {
	if h.Uploads == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	var cursor partCursor
	if err := decodeCursor(params.Cursor, &cursor); err != nil {
		return nil, mapServiceError(uploads.ErrInvalidInput)
	}
	ownerID, err := h.resolveAuthenticatedUploadOwner(ctx, googleUUID(params.UploadId), false)
	if err != nil {
		return nil, mapServiceError(err)
	}
	input := uploads.ListPartsInput{UserID: ownerID, UploadID: googleUUID(params.UploadId), Limit: params.Limit.Or(100)}
	if cursor.PartNo > 0 {
		input.AfterPartNo = &cursor.PartNo
	}
	parts, err := h.Uploads.ListParts(ctx, input)
	if err != nil {
		return nil, mapServiceError(err)
	}
	items := make([]gen.UploadPart, 0, len(parts))
	for _, part := range parts {
		item, err := uploadPart(part)
		if err != nil {
			return nil, mapServiceError(err)
		}
		items = append(items, item)
	}
	response := gen.ListUploadPartsOK{Items: items}
	if len(parts) == int(input.Limit) && len(parts) > 0 {
		response.NextCursor = encodeCursor(partCursor{PartNo: parts[len(parts)-1].PartNo})
	}
	return &response, nil
}

func (h *Handler) PutUploadPart(ctx context.Context, req gen.PutUploadPartReq, params gen.PutUploadPartParams) (gen.PutUploadPartRes, error) {
	if h.UploadPipeline == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	var checksum *string
	if value, ok := params.XPartChecksum.Get(); ok {
		text := string(value)
		checksum = &text
	}
	ownerID, err := h.resolveAuthenticatedUploadOwner(ctx, googleUUID(params.UploadId), true)
	if err != nil {
		return nil, mapServiceError(err)
	}
	result, err := h.UploadPipeline.UploadPart(ctx, transfer.UploadPartRequest{
		UserID: ownerID, UploadID: googleUUID(params.UploadId), PartNo: params.PartNo,
		PlainSize: params.ContentLength, Checksum: checksum, Body: req.Data,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	part, err := uploadPart(result.Part)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if result.Existing {
		response := gen.PutUploadPartOK(part)
		return &response, nil
	}
	response := gen.PutUploadPartCreated(part)
	return &response, nil
}

func (h *Handler) CompleteUpload(ctx context.Context, params gen.CompleteUploadParams) (gen.CompleteUploadRes, error) {
	if h.Uploads == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	ownerID, err := h.resolveAuthenticatedUploadOwner(ctx, googleUUID(params.UploadId), true)
	if err != nil {
		return nil, mapServiceError(err)
	}
	file, err := h.Uploads.Complete(ctx, ownerID, googleUUID(params.UploadId))
	if err != nil {
		return nil, mapServiceError(err)
	}
	entry, err := fileEntry(file)
	if err != nil {
		return nil, mapServiceError(err)
	}
	fileID := googleUUID(entry.ID)
	headers := gen.CopyFileCreatedHeaders{
		Etag: generationETag(file.Generation), Location: gen.URI(url.URL{Path: "/files/" + fileID.String()}),
		Response: entry,
	}
	response := gen.CompleteUploadCreated(headers)
	return &response, nil
}

func (h *Handler) AbortUpload(ctx context.Context, params gen.AbortUploadParams) (gen.AbortUploadRes, error) {
	if h.Uploads == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	ownerID, err := h.resolveAuthenticatedUploadOwner(ctx, googleUUID(params.UploadId), true)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if _, err := h.Uploads.Abort(ctx, ownerID, googleUUID(params.UploadId)); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.AbortUploadNoContent{}, nil
}

func (h *Handler) HeadFile(ctx context.Context, params gen.HeadFileParams) (gen.HeadFileRes, error) {
	if h.Catalog == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	access, err := h.resolveAuthenticatedFileAccess(ctx, googleUUID(params.FileId), false)
	if err != nil {
		return nil, mapServiceError(err)
	}
	file, err := h.Catalog.Get(ctx, access.OwnerID, googleUUID(params.FileId))
	if err != nil {
		return nil, mapServiceError(err)
	}
	if file.Kind != sqlcgen.FileKindFile || !file.Size.Valid {
		return nil, mapServiceError(transfer.ErrInvalidDownload)
	}
	return &gen.HeadFileOK{
		AcceptRanges: gen.HeadFileOKAcceptRanges("bytes"), ContentDisposition: contentDisposition(file.Name, false),
		ContentLength: file.Size.Int64, Etag: contentETag(file), LastModified: file.ModTime.Time,
	}, nil
}

func (h *Handler) HeadFileLegacy(ctx context.Context, params gen.HeadFileLegacyParams) (gen.HeadFileLegacyRes, error) {
	if h.Catalog == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	fileID := googleUUID(params.FileId)
	access, err := h.resolveAuthenticatedFileAccess(ctx, fileID, false)
	if err != nil {
		return nil, mapServiceError(err)
	}
	file, err := h.Catalog.Get(ctx, access.OwnerID, fileID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if file.Kind != sqlcgen.FileKindFile || !file.Size.Valid {
		return nil, mapServiceError(catalog.ErrNotFound)
	}
	return &gen.HeadFileLegacyOK{
		AcceptRanges: gen.HeadFileLegacyOKAcceptRanges("bytes"), ContentDisposition: contentDisposition(file.Name, false),
		ContentLength: file.Size.Int64, Etag: contentETag(file), LastModified: file.ModTime.Time,
	}, nil
}

func contentETag(file *sqlcgen.File) gen.ETag {
	if file.HashValue.Valid && strings.TrimSpace(file.HashValue.String) != "" {
		return gen.ETag(`"` + file.HashValue.String + `"`)
	}
	return generationETag(file.Generation)
}
