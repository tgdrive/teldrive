package sqlcgen

import (
	"strings"
	"testing"
)

func TestRenderSchema(t *testing.T) {
	if err := ConfigureSchema("tenant_alpha"); err != nil {
		t.Fatalf("configure schema: %v", err)
	}
	t.Cleanup(func() { _ = ConfigureSchema("teldrive") })

	query := "SELECT * FROM " + schemaTemplateMarker + "files WHERE status = $1::" + schemaTemplateMarker + "file_status"
	rendered := renderSchema(query)
	if strings.Contains(rendered, schemaTemplateMarker) {
		t.Fatalf("rendered query still contains marker: %s", rendered)
	}
	if want := `"tenant_alpha".files`; !strings.Contains(rendered, want) {
		t.Fatalf("rendered query %q does not contain %q", rendered, want)
	}
	if want := `"tenant_alpha".file_status`; !strings.Contains(rendered, want) {
		t.Fatalf("rendered query %q does not contain %q", rendered, want)
	}
}

func TestConfigureSchemaRejectsInvalidIdentifier(t *testing.T) {
	if err := ConfigureSchema("tenant-alpha"); err == nil {
		t.Fatal("expected invalid schema to fail")
	}
}
