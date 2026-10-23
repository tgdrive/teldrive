package api

import (
	"context"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/authn"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
)

func (h *Handler) GetUserPreferences(ctx context.Context) (gen.GetUserPreferencesRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Auth == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	row, err := h.Auth.Preferences(ctx, userID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return userPreferences(row), nil
}

func (h *Handler) UpdateUserPreferences(ctx context.Context, req *gen.UserPreferencesUpdate) (gen.UpdateUserPreferencesRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Auth == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	var update authn.PreferencesUpdate
	if uploads, ok := req.Uploads.Get(); ok {
		if value, ok := uploads.Encryption.Get(); ok {
			update.UploadEncryption = &value
		}
		if value, ok := uploads.ConflictPolicy.Get(); ok {
			policy := sqlcgen.NameConflictPolicy(value)
			update.UploadConflictPolicy = &policy
		}
		if value, ok := uploads.Concurrency.Get(); ok {
			update.UploadConcurrency = &value
		}
		if value, ok := uploads.PreferredPartSize.Get(); ok {
			update.UploadPartSize = &value
		}
	}
	if files, ok := req.Files.Get(); ok {
		if value, ok := files.Sort.Get(); ok {
			sort := string(value)
			update.FileSort = &sort
		}
		if value, ok := files.Order.Get(); ok {
			order := string(value)
			update.FileOrder = &order
		}
	}
	if appearance, ok := req.Appearance.Get(); ok {
		if value, ok := appearance.Theme.Get(); ok {
			theme := string(value)
			update.Theme = &theme
		}
		if value, ok := appearance.Locale.Get(); ok {
			locale := string(value)
			update.Locale = &locale
		}
		if value, ok := appearance.EmbeddedTracks.Get(); ok {
			update.EmbeddedTracks = &value
		}
		if value, ok := appearance.EmbeddedTracksOnShares.Get(); ok {
			update.EmbeddedTracksOnShares = &value
		}
	}
	row, err := h.Auth.UpdatePreferences(ctx, userID, update)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return userPreferences(row), nil
}

func userPreferences(row *sqlcgen.UserPreference) *gen.UserPreferences {
	result := &gen.UserPreferences{
		Uploads: gen.UploadPreferences{
			Encryption:        row.UploadEncryption,
			ConflictPolicy:    gen.NameConflictPolicy(row.UploadConflictPolicy),
			Concurrency:       row.UploadConcurrency,
			PreferredPartSize: row.UploadPartSize,
		},
		Files: gen.FileBrowserPreferences{
			Sort:  gen.FileBrowserSort(row.FileSort),
			Order: gen.SortOrder(row.FileOrder),
		},
		Appearance: gen.AppearancePreferences{
			Theme:                  gen.ThemePreference(row.Theme),
			EmbeddedTracks:         row.EmbeddedTracks,
			EmbeddedTracksOnShares: row.EmbeddedTracksOnShares,
		},
	}
	if row.Locale.Valid {
		result.Appearance.Locale = gen.NewOptInterfaceLocale(gen.InterfaceLocale(row.Locale.String))
	}
	if row.UpdatedAt.Valid {
		result.UpdatedAt = gen.NewOptDateTime(row.UpdatedAt.Time)
	}
	return result
}
