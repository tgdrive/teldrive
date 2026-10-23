package authn

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

const preferencePartSizeStep = 16 << 20

// PreferencesUpdate lists the preference fields to change; nil fields keep
// their stored value.
type PreferencesUpdate struct {
	UploadEncryption     *bool
	UploadConflictPolicy *sqlcgen.NameConflictPolicy
	UploadConcurrency    *int32
	UploadPartSize       *int64
	FileSort             *string
	FileOrder            *string
	Theme                *string
	Locale               *string
	// EmbeddedTracks lets the player read the audio tracks and subtitles stored inside files.
	EmbeddedTracks *bool
	// EmbeddedTracksOnShares does the same on the user's public shares; the update keeps it off
	// while EmbeddedTracks is off.
	EmbeddedTracksOnShares *bool
}

func (u PreferencesUpdate) empty() bool {
	return u == PreferencesUpdate{}
}

// Preferences returns the user's web UI preferences, creating the default row
// on first access so every user always has a complete set of values.
func (s *Service) Preferences(ctx context.Context, userID int64) (*sqlcgen.UserPreference, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	if err := s.queries.EnsureUserPreferences(ctx, userID); err != nil {
		return nil, fmt.Errorf("ensure user preferences: %w", err)
	}
	row, err := s.queries.GetUserPreferences(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user preferences: %w", err)
	}
	return row, nil
}

// UpdatePreferences merges the set fields of update into the stored preferences.
func (s *Service) UpdatePreferences(ctx context.Context, userID int64, update PreferencesUpdate) (*sqlcgen.UserPreference, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	if update.UploadPartSize != nil && *update.UploadPartSize%preferencePartSizeStep != 0 {
		return nil, ErrInvalidInput
	}
	if update.empty() {
		return s.Preferences(ctx, userID)
	}
	if err := s.queries.EnsureUserPreferences(ctx, userID); err != nil {
		return nil, fmt.Errorf("ensure user preferences: %w", err)
	}
	params := sqlcgen.UpdateUserPreferencesParams{
		UploadConcurrency: dbtypes.OptionalInt4(update.UploadConcurrency),
		UploadPartSize:    dbtypes.OptionalInt8(update.UploadPartSize),
		FileSort:          dbtypes.OptionalText(update.FileSort),
		FileOrder:         dbtypes.OptionalText(update.FileOrder),
		Theme:             dbtypes.OptionalText(update.Theme),
		Locale:            dbtypes.OptionalText(update.Locale),
		UserID:            userID,
	}
	if update.UploadEncryption != nil {
		params.UploadEncryption = pgtype.Bool{Bool: *update.UploadEncryption, Valid: true}
	}
	if update.EmbeddedTracks != nil {
		params.EmbeddedTracks = pgtype.Bool{Bool: *update.EmbeddedTracks, Valid: true}
	}
	if update.EmbeddedTracksOnShares != nil {
		params.EmbeddedTracksOnShares = pgtype.Bool{Bool: *update.EmbeddedTracksOnShares, Valid: true}
	}
	if update.UploadConflictPolicy != nil {
		params.UploadConflictPolicy = sqlcgen.NullNameConflictPolicy{NameConflictPolicy: *update.UploadConflictPolicy, Valid: true}
	}
	row, err := s.queries.UpdateUserPreferences(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("update user preferences: %w", err)
	}
	return row, nil
}
