package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/database"
)

func TestMigrationCommandsAreRegistered(t *testing.T) {
	for _, direction := range []string{"up", "down"} {
		cmd, _, err := newRootCommand().Find([]string{"migrate", direction})
		if err != nil || cmd.Name() != direction {
			t.Fatalf("find migrate %s = %v, %v", direction, cmd, err)
		}
	}
}

func TestMigrationCommandUsesDatabaseConfigWithoutServerSecrets(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TELDRIVE_SECURITY_SIGNING_KEY", "")
	t.Setenv("TELDRIVE_SECURITY_DATA_KEY", "")
	for _, direction := range []string{"up", "down"} {
		t.Run(direction, func(t *testing.T) {
			called := false
			cmd := newMigrationCommand(direction, "Test migration", func(ctx context.Context, cfg database.Config) error {
				called = true
				if ctx == nil || cfg.URL != "postgres://example/migrations" || cfg.Schema != "cli_schema" {
					t.Fatalf("unexpected migration config: %+v", cfg)
				}
				return nil
			})
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetArgs([]string{"--database-url", "postgres://example/migrations", "--database-schema", "cli_schema"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if !called || !strings.Contains(output.String(), `schema "cli_schema"`) {
				t.Fatalf("command not executed or schema not reported: %s", output.String())
			}
			if direction == "down" && !strings.Contains(output.String(), "one application migration") {
				t.Fatalf("down did not report its single-migration scope: %s", output.String())
			}
		})
	}
}

func TestMigrationCommandPropagatesFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	want := errors.New("migration failed")
	cmd := newMigrationCommand("down", "Test migration", func(context.Context, database.Config) error { return want })
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"--database-url", "postgres://example/migrations"})
	if err := cmd.Execute(); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
