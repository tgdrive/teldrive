package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tgdrive/teldrive/v2/internal/config"
	"github.com/tgdrive/teldrive/v2/internal/database"
)

func newMigrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply or roll back database migrations without starting the server",
		Long:  "Manage embedded database migrations. Stop all server instances before rolling back.\nRollback can delete data; server startup automatically reapplies pending migrations.",
	}
	cmd.AddCommand(
		newMigrationCommand("up", "Apply all pending application, River, and RiverPro migrations", database.Migrate),
		newMigrationCommand("down", "Roll back exactly one application migration (not River or RiverPro)", database.Rollback),
	)
	return cmd
}

func newMigrationCommand(direction, description string, migrate func(context.Context, database.Config) error) *cobra.Command {
	loader := config.NewLoader()
	cmd := &cobra.Command{
		Use:   direction,
		Short: description,
		Long:  description + ".\nStop all server instances before rolling back. Rollback can delete data.\nServer startup automatically reapplies pending migrations.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loader.LoadDatabase(cmd.Flags())
			if err != nil {
				return err
			}
			if err := migrate(cmd.Context(), cfg); err != nil {
				return err
			}
			schema := cfg.Schema
			if schema == "" {
				schema = database.DefaultSchema
			}
			if direction == "down" {
				fmt.Fprintf(cmd.OutOrStdout(), "Rolled back one application migration in schema %q.\n", schema)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Applied pending migrations in schema %q.\n", schema)
			}
			return nil
		},
	}
	loader.RegisterFlags(cmd.Flags())
	return cmd
}
