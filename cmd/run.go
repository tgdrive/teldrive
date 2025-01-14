package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
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
	"github.com/tgdrive/teldrive/internal/duration"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/middleware"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/ui"
	"go.etcd.io/bbolt"

	"github.com/tgdrive/teldrive/pkg/cron"
	"github.com/tgdrive/teldrive/pkg/services"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
)

func NewRun() *cobra.Command {
	var cfg config.ServerCmdConfig
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start Teldrive Server",
		Run: func(cmd *cobra.Command, args []string) {
			runApplication(cmd.Context(), &cfg)

		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			loader := config.NewConfigLoader()
			if err := loader.InitializeConfig(cmd); err != nil {
				return err
			}
			if err := loader.Load(&cfg); err != nil {
				return err
			}
			if err := checkRequiredRunFlags(&cfg); err != nil {
				return err
			}
			return nil
		},
	}
	addServerFlags(cmd, &cfg)
	return cmd
}
func addServerFlags(cmd *cobra.Command, cfg *config.ServerCmdConfig) {

	flags := cmd.Flags()

	config.AddCommonFlags(flags, cfg)

	// Server config
	flags.IntVarP(&cfg.Server.Port, "server-port", "p", 8080, "Server port")
	duration.DurationVar(flags, &cfg.Server.GracefulShutdown, "server-graceful-shutdown", 10*time.Second, "Server graceful shutdown timeout")
	flags.BoolVar(&cfg.Server.EnablePprof, "server-enable-pprof", false, "Enable Pprof Profiling")
	duration.DurationVar(flags, &cfg.Server.ReadTimeout, "server-read-timeout", 1*time.Hour, "Server read timeout")
	duration.DurationVar(flags, &cfg.Server.WriteTimeout, "server-write-timeout", 1*time.Hour, "Server write timeout")

	// CronJobs config
	flags.BoolVar(&cfg.CronJobs.Enable, "cronjobs-enable", true, "Run cron jobs")
	duration.DurationVar(flags, &cfg.CronJobs.CleanFilesInterval, "cronjobs-clean-files-interval", 1*time.Hour, "Clean files interval")
	duration.DurationVar(flags, &cfg.CronJobs.CleanUploadsInterval, "cronjobs-clean-uploads-interval", 12*time.Hour, "Clean uploads interval")
	duration.DurationVar(flags, &cfg.CronJobs.FolderSizeInterval, "cronjobs-folder-size-interval", 2*time.Hour, "Folder size update  interval")

	// Cache config
	flags.IntVar(&cfg.Cache.MaxSize, "cache-max-size", 10*1024*1024, "Max Cache max size (memory)")
	flags.StringVar(&cfg.Cache.RedisAddr, "cache-redis-addr", "", "Redis address")
	flags.StringVar(&cfg.Cache.RedisPass, "cache-redis-pass", "", "Redis password")

	// JWT config
	flags.StringVar(&cfg.JWT.Secret, "jwt-secret", "", "JWT secret key")
	duration.DurationVar(flags, &cfg.JWT.SessionTime, "jwt-session-time", (30*24)*time.Hour, "JWT session duration")
	flags.StringSliceVar(&cfg.JWT.AllowedUsers, "jwt-allowed-users", []string{}, "Allowed users")

	// Telegram Uploads config
	flags.StringVar(&cfg.TG.Uploads.EncryptionKey, "tg-uploads-encryption-key", "", "Uploads encryption key")
	flags.IntVar(&cfg.TG.Uploads.Threads, "tg-uploads-threads", 8, "Uploads threads")
	flags.IntVar(&cfg.TG.Uploads.MaxRetries, "tg-uploads-max-retries", 10, "Uploads Retries")
	duration.DurationVar(flags, &cfg.TG.ReconnectTimeout, "tg-reconnect-timeout", 5*time.Minute, "Reconnect Timeout")
	duration.DurationVar(flags, &cfg.TG.Uploads.Retention, "tg-uploads-retention", (24*7)*time.Hour, "Uploads retention duration")
	flags.IntVar(&cfg.TG.Stream.MultiThreads, "tg-stream-multi-threads", 0, "Stream multi-threads")
	flags.IntVar(&cfg.TG.Stream.Buffers, "tg-stream-buffers", 8, "No of Stream buffers")
	duration.DurationVar(flags, &cfg.TG.Stream.ChunkTimeout, "tg-stream-chunk-timeout", 20*time.Second, "Chunk Fetch Timeout")

}

func checkRequiredRunFlags(cfg *config.ServerCmdConfig) error {
	var missingFields []string

	if cfg.DB.DataSource == "" {
		missingFields = append(missingFields, "db-data-source")
	}
	if cfg.JWT.Secret == "" {
		missingFields = append(missingFields, "jwt-secret")
	}
	if cfg.TG.AppHash == "" {
		missingFields = append(missingFields, "tg-app-hash")
	}
	if cfg.TG.AppId == 0 {
		missingFields = append(missingFields, "tg-app-id")
	}

	if len(missingFields) > 0 {
		return fmt.Errorf("required configuration values not set: %s", strings.Join(missingFields, ", "))
	}

	return nil
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

	boltDb, err := tgc.NewBoltDB(conf.TG.SessionFile)

	if err != nil {
		lg.Fatalw("failed to create bolt db", "err", err)
	}

	worker := tgc.NewBotWorker()

	srv := setupServer(conf, db, cacher, boltDb, worker)

	cron.StartCronJobs(scheduler, db, conf)

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

	scheduler.Stop()

	lg.Info("Server stopped")
}

func setupServer(cfg *config.ServerCmdConfig, db *gorm.DB, cache cache.Cacher, boltdb *bbolt.DB, worker *tgc.BotWorker) *http.Server {

	lg := logging.DefaultLogger()

	apiSrv := services.NewApiService(db, cfg, cache, boltdb, worker)

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
