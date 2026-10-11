package migrations

import (
	"io/fs"
	"strings"
	"testing"
)

func TestRenderedFilesQualifyConfiguredSchema(t *testing.T) {
	t.Parallel()

	rendered, err := renderedFiles("tenant_alpha")
	if err != nil {
		t.Fatalf("render migrations: %v", err)
	}
	entries, err := fs.ReadDir(rendered, ".")
	if err != nil {
		t.Fatalf("read rendered migrations: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected rendered migrations")
	}
	for _, entry := range entries {
		data, err := fs.ReadFile(rendered, entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		text := string(data)
		if strings.Contains(text, schemaTemplateMarker) {
			t.Fatalf("migration %s still contains schema marker", entry.Name())
		}
		if !strings.Contains(text, `"tenant_alpha".`) {
			t.Fatalf("migration %s does not contain qualified schema", entry.Name())
		}
	}
}

func TestRenderedFilesRejectInvalidSchema(t *testing.T) {
	t.Parallel()
	if _, err := renderedFiles(`tenant; DROP SCHEMA public`); err == nil {
		t.Fatal("expected invalid schema to fail")
	}
}
