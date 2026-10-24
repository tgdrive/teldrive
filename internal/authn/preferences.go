package authn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
)

const preferencePartSizeStep = 16 << 20

// PreferencesUpdate lists the preference fields to change; nil fields keep
// their stored value.
type PreferencesUpdate struct {
	Captions             *CaptionPreferencesUpdate
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
func (s *Service) Preferences(ctx context.Context, userID int64) (*Preferences, error) {
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
	return decodePreferences(row)
}

// UpdatePreferences merges the set fields of update into the stored preferences.
func (s *Service) UpdatePreferences(ctx context.Context, userID int64, update PreferencesUpdate) (*Preferences, error) {
	if userID <= 0 || !update.valid() {
		return nil, ErrInvalidInput
	}
	if update.empty() {
		return s.Preferences(ctx, userID)
	}
	if err := s.queries.EnsureUserPreferences(ctx, userID); err != nil {
		return nil, fmt.Errorf("ensure user preferences: %w", err)
	}
	// Lock before reading and merging. Concurrent field-level patches from different
	// browsers serialize on this row rather than replacing one another's values.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := s.queries.WithTx(tx)
	row, err := queries.GetUserPreferencesForUpdate(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("lock user preferences: %w", err)
	}
	patch := update.documentPatch()
	data, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("encode preferences patch: %w", err)
	}
	merged, err := mergePreferencesJSON(row.Preferences, data)
	if err != nil {
		return nil, fmt.Errorf("merge preferences: %w", err)
	}
	row.Preferences = merged
	resolved, err := decodePreferences(row)
	if err != nil {
		return nil, err
	}
	// Public shares cannot enable embedded tracks while the account option is off.
	if resolved.Appearance.EmbeddedTracksOnShares && !resolved.Appearance.EmbeddedTracks {
		merged, err = mergePreferencesJSON(merged, []byte(`{"appearance":{"embeddedTracksOnShares":false}}`))
		if err != nil {
			return nil, err
		}
	}
	row, err = queries.UpdateUserPreferences(ctx, sqlcgen.UpdateUserPreferencesParams{UserID: userID, Preferences: merged})
	if err != nil {
		return nil, fmt.Errorf("update user preferences: %w", err)
	}
	result, err := decodePreferences(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

// Merge objects recursively, preserving unrecognized/future keys as well as exact
// JSON number representations. Non-object values are replaced by the patch.
func mergePreferencesJSON(current, patch []byte) ([]byte, error) {
	var base, changes map[string]json.RawMessage
	if err := json.Unmarshal(current, &base); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(patch, &changes); err != nil {
		return nil, err
	}
	if base == nil {
		base = make(map[string]json.RawMessage)
	}
	for key, value := range changes {
		if bytes.HasPrefix(bytes.TrimSpace(base[key]), []byte("{")) && bytes.HasPrefix(bytes.TrimSpace(value), []byte("{")) {
			merged, err := mergePreferencesJSON(base[key], value)
			if err != nil {
				return nil, err
			}
			base[key] = merged
		} else {
			base[key] = value
		}
	}
	return json.Marshal(base)
}

func putPreference[T any](group map[string]any, key string, value *T) {
	if value != nil {
		group[key] = *value
	}
}

func (u PreferencesUpdate) documentPatch() map[string]any {
	uploads, files, appearance := map[string]any{}, map[string]any{}, map[string]any{}
	putPreference(uploads, "encryption", u.UploadEncryption)
	putPreference(uploads, "conflictPolicy", u.UploadConflictPolicy)
	putPreference(uploads, "concurrency", u.UploadConcurrency)
	putPreference(uploads, "preferredPartSize", u.UploadPartSize)
	putPreference(files, "sort", u.FileSort)
	putPreference(files, "order", u.FileOrder)
	putPreference(appearance, "theme", u.Theme)
	putPreference(appearance, "locale", u.Locale)
	putPreference(appearance, "embeddedTracks", u.EmbeddedTracks)
	putPreference(appearance, "embeddedTracksOnShares", u.EmbeddedTracksOnShares)
	patch := map[string]any{}
	for key, group := range map[string]map[string]any{"uploads": uploads, "files": files, "appearance": appearance} {
		if len(group) > 0 {
			patch[key] = group
		}
	}
	if u.Captions != nil {
		patch["captions"] = u.Captions
	}
	return patch
}

func (u PreferencesUpdate) valid() bool {
	choice := func(value *string, options ...string) bool {
		if value == nil {
			return true
		}
		for _, option := range options {
			if *value == option {
				return true
			}
		}
		return false
	}
	return (u.UploadConcurrency == nil || (*u.UploadConcurrency >= 1 && *u.UploadConcurrency <= 12)) &&
		(u.UploadPartSize == nil || (*u.UploadPartSize >= preferencePartSizeStep && *u.UploadPartSize <= 2<<30 && *u.UploadPartSize%preferencePartSizeStep == 0)) &&
		(u.UploadConflictPolicy == nil || *u.UploadConflictPolicy == "fail" || *u.UploadConflictPolicy == "rename" || *u.UploadConflictPolicy == "replace") &&
		choice(u.FileSort, "name", "updatedAt", "size") && choice(u.FileOrder, "asc", "desc") &&
		choice(u.Theme, "system", "light", "dark") && choice(u.Locale, "en", "es", "pt-BR", "ru", "fr", "de", "zh-CN") &&
		(u.Captions == nil || u.Captions.valid())
}
