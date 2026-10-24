package authn

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
)

func TestPreferencesDocumentDefaultsAndFutureKeys(t *testing.T) {
	data, err := mergePreferencesJSON([]byte(`{"future":{"precise":9007199254740993,"other":true},"captions":{"fontFamily":"serif","fontScale":120}}`), []byte(`{"future":{"new":false},"captions":{"fontScale":0}}`))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["future"]) != `{"new":false,"other":true,"precise":9007199254740993}` {
		t.Fatalf("future keys or precision lost: %s", data)
	}
	resolved, err := decodePreferences(&sqlcgen.UserPreference{Preferences: []byte(`{"captions":{"fontScale":125}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Captions.FontScale != 125 || resolved.Captions.FontFamily != "sans" || resolved.Uploads.Concurrency != 3 || resolved.Appearance.Theme != "system" || resolved.UpdatedAt != nil {
		t.Fatalf("defaults = %+v", resolved)
	}
}

func TestCaptionPreferencesValidateRanges(t *testing.T) {
	for _, value := range []float64{-1, 201, math.NaN(), math.Inf(1)} {
		if (CaptionPreferencesUpdate{FontScale: &value}).valid() {
			t.Fatalf("accepted scale %v", value)
		}
	}
	color := "red"
	if (CaptionPreferencesUpdate{TextColor: &color}).valid() {
		t.Fatal("accepted non-hex color")
	}
	zero := 0.0
	if !(CaptionPreferencesUpdate{Delay: &zero, TextOpacity: &zero}).valid() {
		t.Fatal("zero-valued settings must be preserved")
	}
}
