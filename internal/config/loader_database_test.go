package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestLoadDatabasePreservesPrecedenceWithoutServerSecrets(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[database]\nurl = 'postgres://file/test'\nschema = 'file_schema'\nconnect-timeout = '3s'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{"TELDRIVE_DATABASE_URL": "postgres://environment/test", "TELDRIVE_DATABASE_SCHEMA": "env_schema"}
	loader := newLoader(func(key string) (string, bool) { value, ok := environment[key]; return value, ok }, func() (string, error) { return "", nil })
	flags := pflag.NewFlagSet("migrate", pflag.ContinueOnError)
	loader.RegisterFlags(flags)
	if err := flags.Parse([]string{"--config", path, "--database-schema", "flag_schema"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := loader.LoadDatabase(flags)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "postgres://environment/test" || cfg.Schema != "flag_schema" || cfg.ConnectTimeout != 3*time.Second {
		t.Fatalf("database config precedence = %+v", cfg)
	}
	if _, err := loader.Load(flags); !errors.Is(err, ErrInvalid) {
		t.Fatalf("normal server config must still require secrets: %v", err)
	}
}
