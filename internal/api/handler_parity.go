package api

import (
	"context"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/shares"
)

func (h *Handler) BulkMoveFiles(ctx context.Context, req *gen.FileBulkMoveRequest, params gen.BulkMoveFilesParams) (gen.BulkMoveFilesRes, error) {
	if h.Catalog == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	fileIDs := make([]uuid.UUID, 0, len(req.FileIds))
	for _, value := range req.FileIds {
		fileIDs = append(fileIDs, googleUUID(value))
	}
	ownerID, _, err := h.resolveAuthenticatedFileAccessMany(ctx, fileIDs, true)
	if err != nil {
		return nil, mapServiceError(err)
	}
	parentID := optionalGoogleUUID(req.ParentId)
	if parentID == nil {
		actorID, err := UserIDFromContext(ctx)
		if err != nil || ownerID != actorID {
			return nil, mapServiceError(shares.ErrForbidden)
		}
	} else {
		destinationAccess, err := h.resolveAuthenticatedFileAccess(ctx, *parentID, true)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if destinationAccess.OwnerID != ownerID {
			return nil, mapServiceError(shares.ErrForbidden)
		}
	}
	policy := "fail"
	if value, ok := req.ConflictPolicy.Get(); ok {
		policy = string(value)
	}
	files, err := h.Catalog.BulkMove(ctx, ownerID, fileIDs, parentID, policy)
	if err != nil {
		return nil, mapServiceError(err)
	}
	items, err := fileEntries(files)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.FileBulkResult{Items: items}, nil
}

func (h *Handler) BulkTrashFiles(ctx context.Context, req *gen.FileBulkTrashRequest, params gen.BulkTrashFilesParams) (gen.BulkTrashFilesRes, error) {
	if h.Catalog == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	fileIDs := make([]uuid.UUID, 0, len(req.FileIds))
	for _, value := range req.FileIds {
		fileIDs = append(fileIDs, googleUUID(value))
	}
	ownerID, accesses, err := h.resolveAuthenticatedFileAccessMany(ctx, fileIDs, true)
	if err != nil {
		return nil, mapServiceError(err)
	}
	for index, access := range accesses {
		if !access.Owned && access.RootFileID == fileIDs[index] {
			return nil, mapServiceError(shares.ErrForbidden)
		}
	}
	files, err := h.Catalog.BulkTrash(ctx, ownerID, fileIDs)
	if err != nil {
		return nil, mapServiceError(err)
	}
	items, err := fileEntries(files)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.FileBulkResult{Items: items}, nil
}

func (h *Handler) resolveAuthenticatedFileAccessMany(ctx context.Context, fileIDs []uuid.UUID, requireEdit bool) (int64, []*shares.Access, error) {
	actorID, err := UserIDFromContext(ctx)
	if err != nil {
		return 0, nil, err
	}
	if len(fileIDs) == 0 {
		return 0, nil, shares.ErrInvalidInput
	}
	var accesses []*shares.Access
	if h.Shares != nil {
		accesses, err = h.Shares.ResolveAccessMany(ctx, actorID, fileIDs, requireEdit)
		if err != nil {
			return 0, nil, err
		}
	} else {
		if h.Catalog == nil {
			return 0, nil, ErrOperationUnavailable
		}
		accesses = make([]*shares.Access, 0, len(fileIDs))
		for _, fileID := range fileIDs {
			if _, err := h.Catalog.Get(ctx, actorID, fileID); err != nil {
				return 0, nil, err
			}
			accesses = append(accesses, &shares.Access{
				OwnerID: actorID, RootFileID: fileID, Permission: sqlcgen.SharePermissionEdit, Owned: true,
			})
		}
	}
	ownerID := accesses[0].OwnerID
	for _, access := range accesses[1:] {
		if access.OwnerID != ownerID {
			return 0, nil, shares.ErrForbidden
		}
	}
	return ownerID, accesses, nil
}

func (h *Handler) GetFileCategoryStatistics(ctx context.Context) (gen.GetFileCategoryStatisticsRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Catalog == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	rows, err := h.Catalog.CategoryStatistics(ctx, userID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	items := make([]gen.FileCategoryStatistics, 0, len(rows))
	for _, row := range rows {
		items = append(items, gen.FileCategoryStatistics{
			Category: gen.FileCategory(row.Category), TotalFiles: row.TotalFiles, TotalSize: row.TotalSize,
		})
	}
	response := gen.GetFileCategoryStatisticsOKApplicationJSON(items)
	return &response, nil
}

func (h *Handler) GetDriveStatistics(ctx context.Context) (gen.GetDriveStatisticsRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Catalog == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	stats, err := h.Catalog.DriveStatistics(ctx, userID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.DriveStatistics{
		TotalFiles: stats.TotalFiles, TotalFolders: stats.TotalFolders, TotalBytes: stats.TotalBytes,
		TrashedFiles: stats.TrashedFiles, ActiveShares: stats.ActiveShares, OpenUploads: stats.OpenUploads,
	}, nil
}

func (h *Handler) UpdateShare(ctx context.Context, req *gen.ShareUpdateRequest, params gen.UpdateShareParams) (gen.UpdateShareRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Shares == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	input := shares.UpdateInput{
		OwnerID: userID, ShareID: googleUUID(params.ShareId),
		ClearPassword: req.ClearPassword.Or(false), ClearExpiresAt: req.ClearExpiresAt.Or(false),
		ClearMaxDownloads: req.ClearMaxDownloads.Or(false),
	}
	if value, ok := req.Password.Get(); ok {
		input.Password = &value
	}
	if value, ok := req.ExpiresAt.Get(); ok {
		input.ExpiresAt = &value
	}
	if value, ok := req.MaxDownloads.Get(); ok {
		input.MaxDownloads = &value
	}
	if value, ok := req.Permission.Get(); ok {
		permission := sqlcgen.SharePermission(value)
		input.Permission = &permission
	}
	row, err := h.Shares.Update(ctx, input)
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := shareSummary(row)
	return &response, nil
}

func (h *Handler) ListPublicShareFiles(ctx context.Context, params gen.ListPublicShareFilesParams) (gen.ListPublicShareFilesRes, error) {
	if h.Shares == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	var cursor fileCursor
	if err := decodeCursor(params.Cursor, &cursor); err != nil {
		return nil, mapServiceError(shares.ErrInvalidInput)
	}
	limit := params.Limit.Or(100)
	input := shares.PublicListInput{
		Token: params.Token, Password: params.XSharePassword.Or(""), Path: params.Path.Or(""),
		Search: params.Search.Or(""), Limit: limit,
	}
	if cursor.ID != uuid.Nil {
		input.AfterID, input.AfterName = &cursor.ID, cursor.Name
	}
	files, err := h.Shares.ListPublicFiles(ctx, input)
	if err != nil {
		return nil, mapServiceError(err)
	}
	items, err := fileEntries(files)
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := gen.ListPublicShareFilesOK{Items: items}
	if len(files) == int(limit) && len(files) > 0 {
		last := files[len(files)-1]
		lastID, _ := dbtypes.GoogleUUID(last.ID)
		response.NextCursor = encodeCursor(fileCursor{Name: last.Name, Sort: "name", Order: "asc", Value: last.Name, ID: lastID})
	}
	return &response, nil
}

func (h *Handler) GetUploadStatistics(ctx context.Context, params gen.GetUploadStatisticsParams) (gen.GetUploadStatisticsRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Uploads == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	rows, err := h.Uploads.Statistics(ctx, userID, params.Days.Or(30))
	if err != nil {
		return nil, mapServiceError(err)
	}
	items := make([]gen.UploadDailyStatistics, 0, len(rows))
	for _, row := range rows {
		items = append(items, gen.UploadDailyStatistics{
			Date: row.Date, UploadedBytes: row.UploadedBytes, CompletedFiles: row.CompletedFiles,
		})
	}
	response := gen.GetUploadStatisticsOKApplicationJSON(items)
	return &response, nil
}
func fileEntries(files []*sqlcgen.File) ([]gen.FileEntry, error) {
	items := make([]gen.FileEntry, 0, len(files))
	for _, file := range files {
		entry, err := fileEntry(file)
		if err != nil {
			return nil, err
		}
		items = append(items, entry)
	}
	return items, nil
}
