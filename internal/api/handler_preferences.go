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
	if captions, ok := req.Captions.Get(); ok {
		patch := &authn.CaptionPreferencesUpdate{}
		if value, ok := captions.FontSize.Get(); ok {
			text := string(value)
			patch.FontSize = &text
		}
		if value, ok := captions.FontScale.Get(); ok {
			patch.FontScale = &value
		}
		if value, ok := captions.FontFamily.Get(); ok {
			text := string(value)
			patch.FontFamily = &text
		}
		if value, ok := captions.TextColor.Get(); ok {
			patch.TextColor = &value
		}
		if value, ok := captions.TextOpacity.Get(); ok {
			patch.TextOpacity = &value
		}
		if value, ok := captions.BackgroundColor.Get(); ok {
			patch.BackgroundColor = &value
		}
		if value, ok := captions.BackgroundOpacity.Get(); ok {
			patch.BackgroundOpacity = &value
		}
		if value, ok := captions.EdgeStyle.Get(); ok {
			text := string(value)
			patch.EdgeStyle = &text
		}
		if value, ok := captions.EdgeColor.Get(); ok {
			patch.EdgeColor = &value
		}
		if value, ok := captions.Position.Get(); ok {
			patch.Position = &value
		}
		if value, ok := captions.LineHeight.Get(); ok {
			patch.LineHeight = &value
		}
		if value, ok := captions.Delay.Get(); ok {
			patch.Delay = &value
		}
		update.Captions = patch
	}
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

func userPreferences(row *authn.Preferences) *gen.UserPreferences {
	result := &gen.UserPreferences{
		Uploads: gen.UploadPreferences{
			Encryption:        row.Uploads.Encryption,
			ConflictPolicy:    gen.NameConflictPolicy(row.Uploads.ConflictPolicy),
			Concurrency:       row.Uploads.Concurrency,
			PreferredPartSize: row.Uploads.PreferredPartSize,
		},
		Files: gen.FileBrowserPreferences{
			Sort:  gen.FileBrowserSort(row.Files.Sort),
			Order: gen.SortOrder(row.Files.Order),
		},
		Appearance: gen.AppearancePreferences{
			Theme:                  gen.ThemePreference(row.Appearance.Theme),
			EmbeddedTracks:         row.Appearance.EmbeddedTracks,
			EmbeddedTracksOnShares: row.Appearance.EmbeddedTracksOnShares,
		},
		Captions: gen.CaptionPreferences{
			FontSize: gen.CaptionPreferencesFontSize(row.Captions.FontSize), FontScale: row.Captions.FontScale,
			FontFamily: gen.CaptionPreferencesFontFamily(row.Captions.FontFamily), TextColor: row.Captions.TextColor,
			TextOpacity: row.Captions.TextOpacity, BackgroundColor: row.Captions.BackgroundColor,
			BackgroundOpacity: row.Captions.BackgroundOpacity, EdgeStyle: gen.CaptionPreferencesEdgeStyle(row.Captions.EdgeStyle),
			EdgeColor: row.Captions.EdgeColor, Position: row.Captions.Position, LineHeight: row.Captions.LineHeight, Delay: row.Captions.Delay,
		},
	}
	if row.Appearance.Locale != nil {
		result.Appearance.Locale = gen.NewOptInterfaceLocale(gen.InterfaceLocale(*row.Appearance.Locale))
	}
	if row.UpdatedAt != nil {
		result.UpdatedAt = gen.NewOptDateTime(*row.UpdatedAt)
	}
	return result
}
