package jobs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateLocalImportSourceRequiresConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "movie.bin")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateLocalImportSource(file, nil); err == nil {
		t.Fatal("expected local imports to be disabled without roots")
	}
	if err := validateLocalImportSource(file, []string{root}); err != nil {
		t.Fatalf("allowed file rejected: %v", err)
	}
}

func TestValidateLocalImportSourceRejectsOutsideAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateLocalImportSource(outsideFile, []string{root}); err == nil {
		t.Fatal("expected outside path to be rejected")
	}

	link := filepath.Join(root, "escape")
	if err := os.Symlink(outsideFile, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := validateLocalImportSource(link, []string{root}); err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}
