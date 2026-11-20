package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/tgdrive/teldrive/v2/internal/app"
	"github.com/tgdrive/teldrive/v2/internal/config"
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
	root.AddCommand(newRunCommand(), newCheckCommand(), newMigrateCommand(), newVersionCommand(), newBackfillPartSizesCommand())
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
			logger, closeLog, err := logging.NewLoggerWithFile(os.Stdout, cfg.Logging.LogLevel, cfg.Logging.LogFormat, cfg.Logging.LogFile)
			if err != nil {
				return err
			}
			defer closeLog()
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
			logger, closeLog, err := logging.NewLoggerWithFile(os.Stdout, cfg.Logging.LogLevel, cfg.Logging.LogFormat, cfg.Logging.LogFile)
			if err != nil {
				return err
			}
			defer closeLog()
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

func newBackfillPartSizesCommand() *cobra.Command {
	loader := config.NewLoader()
	var batchSize int
	var delay time.Duration
	var limit int64
	cmd := &cobra.Command{
		Use:   "backfill-part-sizes",
		Short: "Backfill missing part sizes from Telegram into database",
		Long:  "Query file parts that lack plain_size or stored_size (e.g. from v1 migrations), fetch their sizes in batches from Telegram, and update the database.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loader.Load(cmd.Flags())
			if err != nil {
				return err
			}
			logger, closeLog, err := logging.NewLoggerWithFile(os.Stdout, cfg.Logging.LogLevel, cfg.Logging.LogFormat, cfg.Logging.LogFile)
			if err != nil {
				return err
			}
			defer closeLog()
			slog.SetDefault(logger)

			application, err := app.New(cmd.Context(), cfg, app.Dependencies{Logger: logger, Version: buildVersion()})
			if err != nil {
				return fmt.Errorf("initialize TelDrive: %w", err)
			}
			defer application.Close()

			return application.BackfillPartSizes(cmd.Context(), batchSize, delay, limit)
		},
	}
	cmd.Flags().IntVar(&batchSize, "batch-size", 100, "Maximum number of messages to query per Telegram request (1-100)")
	cmd.Flags().DurationVar(&delay, "delay", time.Second, "Delay between Telegram batch requests")
	cmd.Flags().Int64Var(&limit, "limit", 0, "Maximum total parts to process (0 for unlimited)")
	loader.RegisterFlags(cmd.Flags())
	return cmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build version information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			shortCommit := commit
			if len(shortCommit) > 7 {
				shortCommit = shortCommit[:7]
			}
			fmt.Fprintf(cmd.OutOrStdout(), "teldrive %s\n- commit: %s\n- built: %s\n- go/version: %s\n- os/arch: %s/%s\n",
				buildVersion(), shortCommit, date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
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
