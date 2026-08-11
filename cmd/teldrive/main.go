package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"

	"github.com/tgdrive/teldrive/v2/internal/app"
	"github.com/tgdrive/teldrive/v2/internal/config"
	"github.com/tgdrive/teldrive/v2/internal/database"
	"github.com/tgdrive/teldrive/v2/internal/legacymigrate"
	"github.com/tgdrive/teldrive/v2/internal/logging"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := newRootCommand().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "teldrive",
		Short:         "Telegram-backed cloud storage server",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newRunCommand(), newCheckCommand(), newMigrateCommand(), newVersionCommand())
	return root
}

func newRunCommand() *cobra.Command {
	loader := config.NewLoader()
	cmd := &cobra.Command{
		Use:     "run",
		Aliases: []string{"serve"},
		Short:   "Start the TelDrive server",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loader.Load(cmd.Flags())
			if err != nil {
				return err
			}
			logger, err := logging.NewLogger(os.Stdout, cfg.Logging.LogLevel, cfg.Logging.LogFormat)
			if err != nil {
				return err
			}
			slog.SetDefault(logger)
			application, err := app.New(cmd.Context(), cfg, app.Dependencies{Logger: logger, Version: buildVersion()})
			if err != nil {
				return fmt.Errorf("initialize TelDrive: %w", err)
			}
			logger.Info("application.starting", "address", cfg.HTTP.Address, "version", buildVersion(), "commit", commit)
			if err := application.Run(cmd.Context()); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("application.stopped", "error", err)
				return err
			}
			logger.Info("application.stopped")
			return nil
		},
	}
	loader.RegisterFlags(cmd.Flags())
	return cmd
}

func newCheckCommand() *cobra.Command {
	loader := config.NewLoader()
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate configuration and initialize dependencies",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loader.Load(cmd.Flags())
			if err != nil {
				return err
			}
			logger, err := logging.NewLogger(os.Stdout, cfg.Logging.LogLevel, cfg.Logging.LogFormat)
			if err != nil {
				return err
			}
			slog.SetDefault(logger)
			application, err := app.New(cmd.Context(), cfg, app.Dependencies{Logger: logger, Version: buildVersion()})
			if err != nil {
				return fmt.Errorf("initialize TelDrive: %w", err)
			}
			if err := application.Close(); err != nil {
				return fmt.Errorf("close checked application: %w", err)
			}
			logger.Info("application.check.succeeded", "version", buildVersion())
			return nil
		},
	}
	loader.RegisterFlags(cmd.Flags())
	return cmd
}

func newMigrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Inspect and migrate data from older TelDrive versions",
	}
	cmd.AddCommand(newLegacyPreflightCommand(), newLegacyApplyCommand())
	return cmd
}

func newLegacyPreflightCommand() *cobra.Command {
	var sourceURL string
	cmd := &cobra.Command{
		Use:   "legacy-preflight",
		Short: "Inspect an old TelDrive database before migration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(sourceURL) == "" {
				return errors.New("--source-database-url is required")
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "WARNING: Back up and verify the old TelDrive database before migration. This preflight is read-only, but the later migration will transform production data.")

			conn, err := pgx.Connect(cmd.Context(), sourceURL)
			if err != nil {
				return fmt.Errorf("connect to legacy database: %w", err)
			}
			defer conn.Close(cmd.Context())

			var legacy bool
			if err := conn.QueryRow(cmd.Context(), `SELECT to_regclass('public.goose_db_version') IS NOT NULL`).Scan(&legacy); err != nil {
				return fmt.Errorf("inspect legacy schema: %w", err)
			}
			if !legacy {
				return errors.New("legacy migration table public.goose_db_version was not found; this command is only for production TelDrive 1.x databases")
			}

			var users, channels, bots, files int64
			if err := conn.QueryRow(cmd.Context(), `SELECT
(SELECT count(*) FROM teldrive.users),
(SELECT count(*) FROM teldrive.channels),
(SELECT count(*) FROM teldrive.bots),
(SELECT count(*) FROM teldrive.files)`).Scan(&users, &channels, &bots, &files); err != nil {
				return fmt.Errorf("count legacy rows: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Legacy TelDrive migration preflight passed.")
			fmt.Fprintf(cmd.OutOrStdout(), "Users: %d\nChannels: %d\nBots: %d\nFiles: %d\n", users, channels, bots, files)
			fmt.Fprintln(cmd.OutOrStdout(), "No database changes were made. Create and verify a database backup before running the eventual migration, and keep the old application stopped during cutover.")
			return nil
		},
	}
	cmd.Flags().StringVar(&sourceURL, "source-database-url", "", "PostgreSQL URL for the old TelDrive database")
	return cmd
}

func newLegacyApplyCommand() *cobra.Command {
	var databaseURL, dataKey string
	cmd := &cobra.Command{
		Use:   "legacy-apply",
		Short: "Migrate a TelDrive 1.x schema in place using an atomic schema swap",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(databaseURL) == "" {
				return errors.New("--database-url is required")
			}
			if strings.TrimSpace(dataKey) == "" {
				dataKey = os.Getenv("TELDRIVE_MIGRATION_DATA_KEY")
			}
			if strings.TrimSpace(dataKey) == "" {
				return errors.New("--data-key or TELDRIVE_MIGRATION_DATA_KEY is required")
			}
			suffix := time.Now().UTC().Format("20060102_150405")
			stagingSchema := database.DefaultSchema + "_v2_staging_" + suffix
			backupSchema := database.DefaultSchema + "_legacy_backup_" + suffix
			fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: This command migrates schema %q in place. It creates staging schema %q, retains the original as %q, and atomically promotes staging only after validation succeeds. Stop TelDrive and verify a database backup first.\n", database.DefaultSchema, stagingSchema, backupSchema)
			report, err := legacymigrate.Run(cmd.Context(), legacymigrate.Config{
				SourceURL: databaseURL,
				Target: database.Config{
					URL:    databaseURL,
					Schema: stagingSchema,
				},
				LegacySchema:         database.DefaultSchema,
				FinalSchema:          database.DefaultSchema,
				BackupSchema:         backupSchema,
				DataKey:              dataKey,
				EncryptionKeyVersion: 1,
				Apply:                true,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Legacy TelDrive migration completed.")
			fmt.Fprintf(cmd.OutOrStdout(), "Users: %d\nChannels: %d\nBots: %d\nFolders: %d\nFiles: %d\nFile parts: %d\nEncrypted files: %d\n", report.Users, report.Channels, report.Bots, report.Folders, report.Files, report.FileParts, report.Encrypted)
			fmt.Fprintf(cmd.OutOrStdout(), "Original legacy schema retained as %s.\n", backupSchema)
			fmt.Fprintln(cmd.OutOrStdout(), "Legacy part sizes will be resolved from Telegram metadata and backfilled on first download.")
			return nil
		},
	}
	cmd.Flags().StringVar(&databaseURL, "database-url", "", "PostgreSQL URL containing the legacy TelDrive schema")
	cmd.Flags().StringVar(&dataKey, "data-key", "", "Base64 key used to encrypt migrated credentials; prefer TELDRIVE_MIGRATION_DATA_KEY")
	return cmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build version information",
		Args:  cobra.NoArgs,
		Run: func(*cobra.Command, []string) {
			fmt.Printf("teldrive %s commit=%s built=%s\n", buildVersion(), commit, date)
		},
	}
}

func buildVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
