package authn

import (
	"math"
	"regexp"
)

// CaptionPreferencesUpdate is a field-level patch. The service merges it under a
// PostgreSQL row lock, so independent browser updates cannot overwrite each other.
type CaptionPreferencesUpdate struct {
	FontSize          *string  `json:"fontSize,omitempty"`
	FontScale         *float64 `json:"fontScale,omitempty"`
	FontFamily        *string  `json:"fontFamily,omitempty"`
	TextColor         *string  `json:"textColor,omitempty"`
	TextOpacity       *float64 `json:"textOpacity,omitempty"`
	BackgroundColor   *string  `json:"backgroundColor,omitempty"`
	BackgroundOpacity *float64 `json:"backgroundOpacity,omitempty"`
	EdgeStyle         *string  `json:"edgeStyle,omitempty"`
	EdgeColor         *string  `json:"edgeColor,omitempty"`
	Position          *float64 `json:"position,omitempty"`
	LineHeight        *float64 `json:"lineHeight,omitempty"`
	Delay             *float64 `json:"delay,omitempty"`
}

var captionColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (u CaptionPreferencesUpdate) valid() bool {
	choice := func(value *string, choices ...string) bool {
		if value == nil {
			return true
		}
		for _, choice := range choices {
			if *value == choice {
				return true
			}
		}
		return false
	}
	color := func(value *string) bool { return value == nil || captionColor.MatchString(*value) }
	rangeOK := func(value *float64, min, max float64) bool {
		return value == nil || (!math.IsNaN(*value) && !math.IsInf(*value, 0) && *value >= min && *value <= max)
	}
	return choice(u.FontSize, "small", "medium", "large") && choice(u.FontFamily, "sans", "serif", "monospace") &&
		choice(u.EdgeStyle, "none", "shadow", "outline") && color(u.TextColor) && color(u.BackgroundColor) && color(u.EdgeColor) &&
		rangeOK(u.FontScale, 50, 200) && rangeOK(u.TextOpacity, 0, 1) && rangeOK(u.BackgroundOpacity, 0, 1) &&
		rangeOK(u.Position, -20, 30) && rangeOK(u.LineHeight, 1, 2) && rangeOK(u.Delay, -10, 10)
}
