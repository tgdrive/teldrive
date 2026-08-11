package catalog

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		display    string
		normalized string
		wantErr    bool
	}{
		{name: "trims and preserves case", input: "  Report.TXT  ", display: "Report.TXT", normalized: "Report.TXT"},
		{name: "normalizes unicode", input: "Cafe\u0301", display: "Café", normalized: "Café"},
		{name: "rejects empty", input: "   ", wantErr: true},
		{name: "rejects dot", input: ".", wantErr: true},
		{name: "rejects dot dot", input: "..", wantErr: true},
		{name: "rejects slash", input: "a/b", wantErr: true},
		{name: "rejects backslash", input: `a\b`, wantErr: true},
		{name: "rejects control", input: "a\x00b", wantErr: true},
		{name: "rejects too long", input: strings.Repeat("a", maxNameRunes+1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			display, normalized, err := NormalizeName(tt.input)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidName) {
					t.Fatalf("expected ErrInvalidName, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeName() error = %v", err)
			}
			if display != tt.display || normalized != tt.normalized {
				t.Fatalf("NormalizeName() = (%q, %q), want (%q, %q)", display, normalized, tt.display, tt.normalized)
			}
		})
	}
}
