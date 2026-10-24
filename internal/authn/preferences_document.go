package authn

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
)

type Preferences struct {
	Uploads    UploadPreferences     `json:"uploads"`
	Files      FilePreferences       `json:"files"`
	Appearance AppearancePreferences `json:"appearance"`
	Captions   CaptionPreferences    `json:"captions"`
	UpdatedAt  *time.Time            `json:"-"`
}

type UploadPreferences struct {
	Encryption        bool   `json:"encryption"`
	ConflictPolicy    string `json:"conflictPolicy"`
	Concurrency       int32  `json:"concurrency"`
	PreferredPartSize int64  `json:"preferredPartSize"`
}

type FilePreferences struct {
	Sort  string `json:"sort"`
	Order string `json:"order"`
}

type AppearancePreferences struct {
	Theme                  string  `json:"theme"`
	Locale                 *string `json:"locale,omitempty"`
	EmbeddedTracks         bool    `json:"embeddedTracks"`
	EmbeddedTracksOnShares bool    `json:"embeddedTracksOnShares"`
}

type CaptionPreferences struct {
	FontSize          string  `json:"fontSize"`
	FontScale         float64 `json:"fontScale"`
	FontFamily        string  `json:"fontFamily"`
	TextColor         string  `json:"textColor"`
	TextOpacity       float64 `json:"textOpacity"`
	BackgroundColor   string  `json:"backgroundColor"`
	BackgroundOpacity float64 `json:"backgroundOpacity"`
	EdgeStyle         string  `json:"edgeStyle"`
	EdgeColor         string  `json:"edgeColor"`
	Position          float64 `json:"position"`
	LineHeight        float64 `json:"lineHeight"`
	Delay             float64 `json:"delay"`
}

// Defaults are application-owned: adding a field never requires a DB migration.
func defaultPreferences() Preferences {
	return Preferences{
		Uploads:    UploadPreferences{ConflictPolicy: "rename", Concurrency: 3, PreferredPartSize: 512 << 20},
		Files:      FilePreferences{Sort: "name", Order: "asc"},
		Appearance: AppearancePreferences{Theme: "system"},
		Captions:   CaptionPreferences{FontSize: "medium", FontScale: 100, FontFamily: "sans", TextColor: "#ffffff", TextOpacity: 1, BackgroundColor: "#000000", BackgroundOpacity: .75, EdgeStyle: "shadow", EdgeColor: "#000000", LineHeight: 1.35},
	}
}

func decodePreferences(row *sqlcgen.UserPreference) (*Preferences, error) {
	result := defaultPreferences()
	if err := json.Unmarshal(row.Preferences, &result); err != nil {
		return nil, fmt.Errorf("decode user preferences: %w", err)
	}
	if row.UpdatedAt.Valid {
		result.UpdatedAt = &row.UpdatedAt.Time
	}
	return &result, nil
}
