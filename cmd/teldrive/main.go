package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
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
	root.AddCommand(newRunCommand(), newCheckCommand(), newVersionCommand())
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
	var validateOnly bool
	var desktopInfo bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate configuration and initialize dependencies",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loader.Load(cmd.Flags())
			if err != nil {
				return err
			}
			if desktopInfo {
				connection, err := pgxpool.ParseConfig(cfg.Database.URL)
				if err != nil {
					return fmt.Errorf("invalid database URL")
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					Address         string `json:"address"`
					DatabaseAddress string `json:"databaseAddress"`
				}{Address: cfg.HTTP.Address, DatabaseAddress: net.JoinHostPort(connection.ConnConfig.Host, strconv.Itoa(int(connection.ConnConfig.Port)))})
			}
			if validateOnly {
				fmt.Fprintln(cmd.OutOrStdout(), "Configuración válida")
				return nil
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
	cmd.Flags().BoolVar(&validateOnly, "validate-only", false, "Validate configuration without initializing services or migrating the database")
	cmd.Flags().BoolVar(&desktopInfo, "desktop-info", false, "Validate configuration and print the HTTP listen address as JSON without secrets or service initialization")
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
