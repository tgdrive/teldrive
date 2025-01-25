package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-co-op/gocron"
	"github.com/spf13/cobra"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/appcontext"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/chizap"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/database"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/middleware"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/tgstorage"
	"github.com/tgdrive/teldrive/ui"

	"github.com/tgdrive/teldrive/pkg/cron"
	"github.com/tgdrive/teldrive/pkg/services"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
)

func NewRun() *cobra.Command {
	var cfg config.ServerCmdConfig
	loader := config.NewConfigLoader()
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start Teldrive Server",
		Run: func(cmd *cobra.Command, args []string) {
			runApplication(cmd.Context(), &cfg)

		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := loader.Load(cmd, &cfg); err != nil {
				return err
			}
			if err := loader.Validate(); err != nil {
				return err
			}
			return nil
		},
	}
	loader.RegisterPlags(cmd.Flags(), "", cfg, false)
	return cmd
}

func findAvailablePort(startPort int) (int, error) {
	for port := startPort; port < startPort+100; port++ {
		addr := fmt.Sprintf(":%d", port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			continue
		}
		listener.Close()
		return port, nil
	}
	return 0, fmt.Errorf("no available ports found between %d and %d", startPort, startPort+100)
}

func runApplication(ctx context.Context, conf *config.ServerCmdConfig) {
	lvl, err := zapcore.ParseLevel(conf.Log.Level)
	if err != nil {
		lvl = zapcore.InfoLevel
	}
	logging.SetConfig(&logging.Config{
		Level:    lvl,
		FilePath: conf.Log.File,
	})

	lg := logging.DefaultLogger().Sugar()

	defer lg.Sync()

	port, err := findAvailablePort(conf.Server.Port)
	if err != nil {
		lg.Fatalw("failed to find available port", "err", err)
	}
	if port != conf.Server.Port {
		lg.Infof("Port %d is occupied, using port %d instead", conf.Server.Port, port)
		conf.Server.Port = port
	}

	scheduler := gocron.NewScheduler(time.UTC)

	cacher := cache.NewCache(ctx, &conf.Cache)

	db, err := database.NewDatabase(&conf.DB, lg)

	if err != nil {
		lg.Fatalw("failed to create database", "err", err)
	}

	err = database.MigrateDB(db)

	if err != nil {
		lg.Fatalw("failed to migrate database", "err", err)
	}

	tgdb, err := tgstorage.NewDatabase(conf.TG.StorageFile)
	if err != nil {
		lg.Fatalw("failed to create tg db", "err", err)
	}

	err = tgstorage.MigrateDB(tgdb)
	if err != nil {
		lg.Fatalw("failed to migrate tg db", "err", err)
	}

	worker := tgc.NewBotWorker()

	srv := setupServer(conf, db, cacher, tgdb, worker)

	cron.StartCronJobs(ctx, scheduler, db, conf)

	go func() {
		lg.Infof("Server started at http://localhost:%d", conf.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			lg.Errorw("failed to start server", "err", err)
		}
	}()

	<-ctx.Done()

	lg.Info("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), conf.Server.GracefulShutdown)

	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		lg.Errorw("server shutdown failed", "err", err)
	}

	lg.Info("Server stopped")
}

func setupServer(cfg *config.ServerCmdConfig, db *gorm.DB, cache cache.Cacher, tgdb *gorm.DB, worker *tgc.BotWorker) *http.Server {

	lg := logging.DefaultLogger()

	apiSrv := services.NewApiService(db, cfg, cache, tgdb, worker)

	srv, err := api.NewServer(apiSrv, auth.NewSecurityHandler(db, cache, &cfg.JWT))

	if err != nil {
		lg.Fatal("failed to create server", zap.Error(err))
	}

	extendedSrv := services.NewExtendedMiddleware(srv, services.NewExtendedService(apiSrv))

	mux := chi.NewRouter()

	mux.Use(chimiddleware.Recoverer)
	mux.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH", "HEAD"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
		MaxAge:         86400,
	}))
	mux.Use(chimiddleware.RealIP)
	mux.Use(middleware.InjectLogger(lg))
	mux.Use(chizap.ChizapWithConfig(lg, &chizap.Config{
		TimeFormat: time.RFC3339,
		UTC:        true,
		SkipPathRegexps: []*regexp.Regexp{
			regexp.MustCompile(`^/(assets|images|docs)/.*`),
		},
	}))
	mux.Use(appcontext.Middleware)
	mux.Mount("/api/", http.StripPrefix("/api", extendedSrv))
	mux.Handle("/*", middleware.SPAHandler(ui.StaticFS))

	return &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           mux,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
