package api

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

func (h *Handler) GetFileViewState(ctx context.Context, params gen.GetFileViewStateParams) (gen.GetFileViewStateRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if _, accessErr := h.resolveAuthenticatedFileAccess(ctx, googleUUID(params.FileId), false); accessErr != nil {
		return nil, mapServiceError(accessErr)
	}
	state, err := h.Catalog.GetViewState(ctx, userID, googleUUID(params.FileId))
	if err != nil {
		if err == catalog.ErrNotFound {
			return &gen.GetFileViewStateNoContent{}, nil
		}
		return nil, mapServiceError(err)
	}
	return fileViewState(state)
}

func (h *Handler) PutFileViewState(ctx context.Context, req *gen.FileViewStateUpdate, params gen.PutFileViewStateParams) (gen.PutFileViewStateRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if _, accessErr := h.resolveAuthenticatedFileAccess(ctx, googleUUID(params.FileId), false); accessErr != nil {
		return nil, mapServiceError(accessErr)
	}
	position, err := json.Marshal(req.Position.Or(gen.FileViewStateUpdatePosition{}))
	if err != nil {
		return nil, mapServiceError(err)
	}
	preferences, err := json.Marshal(req.Preferences.Or(gen.FileViewStateUpdatePreferences{}))
	if err != nil {
		return nil, mapServiceError(err)
	}
	bookmarks, err := json.Marshal(req.Bookmarks)
	if err != nil {
		return nil, mapServiceError(err)
	}
	state, err := h.Catalog.UpsertViewState(ctx, userID, googleUUID(params.FileId), string(req.Kind), position, preferences, bookmarks)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return fileViewState(state)
}

func (h *Handler) DeleteFileViewState(ctx context.Context, params gen.DeleteFileViewStateParams) (gen.DeleteFileViewStateRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if _, accessErr := h.resolveAuthenticatedFileAccess(ctx, googleUUID(params.FileId), false); accessErr != nil {
		return nil, mapServiceError(accessErr)
	}
	if err := h.Catalog.DeleteViewState(ctx, userID, googleUUID(params.FileId)); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.DeleteFileViewStateNoContent{}, nil
}

func fileViewState(row *sqlcgen.FileViewState) (*gen.FileViewState, error) {
	fileID, ok := dbtypes.GoogleUUID(row.FileID)
	if !ok {
		return nil, errors.New("invalid file view state ID")
	}
	result := &gen.FileViewState{FileId: apiUUID(fileID), Kind: gen.ViewerKind(row.ViewerKind), UpdatedAt: row.UpdatedAt.Time}
	if err := json.Unmarshal(row.Position, &result.Position); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(row.Preferences, &result.Preferences); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(row.Bookmarks, &result.Bookmarks); err != nil {
		return nil, err
	}
	return result, nil
}
