package api

import (
	"context"
	"github.com/google/uuid"
	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

func (h *Handler) BulkSpamFiles(ctx context.Context, req *gen.FileBulkTrashRequest, params gen.BulkSpamFilesParams) (gen.BulkSpamFilesRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Catalog == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	ids := make([]uuid.UUID, 0, len(req.FileIds))
	for _, id := range req.FileIds {
		ids = append(ids, googleUUID(id))
	}
	files, err := h.Catalog.BulkSpam(ctx, userID, ids)
	if err != nil {
		return nil, mapServiceError(err)
	}
	items, err := fileEntries(files)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.FileBulkResult{Items: items}, nil
}

func (h *Handler) MarkIncomingSpam(ctx context.Context, params gen.MarkIncomingSpamParams) (gen.MarkIncomingSpamRes, error) {
	actor, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Shares == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	if err := h.Shares.MarkIncomingSpam(ctx, actor, googleUUID(params.FileId)); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.MarkIncomingSpamNoContent{}, nil
}
func (h *Handler) RestoreIncomingSpam(ctx context.Context, params gen.RestoreIncomingSpamParams) (gen.RestoreIncomingSpamRes, error) {
	actor, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Shares == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	if err := h.Shares.RestoreIncomingSpam(ctx, actor, googleUUID(params.FileId), false); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.RestoreIncomingSpamNoContent{}, nil
}
func (h *Handler) DismissIncomingSpam(ctx context.Context, params gen.DismissIncomingSpamParams) (gen.DismissIncomingSpamRes, error) {
	actor, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Shares == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	if err := h.Shares.RestoreIncomingSpam(ctx, actor, googleUUID(params.FileId), true); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.DismissIncomingSpamNoContent{}, nil
}
func (h *Handler) ListIncomingSpam(ctx context.Context) (gen.ListIncomingSpamRes, error) {
	actor, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Shares == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	rows, err := h.Shares.ListIncomingSpam(ctx, actor)
	if err != nil {
		return nil, mapServiceError(err)
	}
	out := make(gen.ListIncomingSpamOKApplicationJSON, 0, len(rows))
	for _, row := range rows {
		id, ok := dbtypes.GoogleUUID(row.ID)
		if !ok {
			continue
		}
		file, err := h.Catalog.Get(ctx, row.UserID, id)
		if err != nil {
			continue
		}
		entry, err := fileEntry(file)
		if err != nil {
			return nil, err
		}
		entry.Status = gen.FileStatusSpam
		entry.UpdatedAt = row.ReportedAt.Time
		out = append(out, entry)
	}
	return &out, nil
}
