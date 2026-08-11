package catalog

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const maxNameRunes = 255

var ErrInvalidName = errors.New("invalid file name")

// NormalizeName returns the canonical display name and the exact-case NFC
// value used by PostgreSQL uniqueness constraints and ordering.
func NormalizeName(raw string) (display string, normalized string, err error) {
	display = norm.NFC.String(strings.TrimSpace(raw))
	if display == "" || display == "." || display == ".." {
		return "", "", ErrInvalidName
	}
	if !utf8.ValidString(display) || utf8.RuneCountInString(display) > maxNameRunes {
		return "", "", ErrInvalidName
	}
	for _, r := range display {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return "", "", ErrInvalidName
		}
	}
	return display, display, nil
}
