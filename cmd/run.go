package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"
	"github.com/spf13/cobra"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/banner"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/chizap"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/database"
	"github.com/tgdrive/teldrive/internal/events"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/middleware"
	"github.com/tgdrive/teldrive/internal/requestmeta"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/version"
	"github.com/tgdrive/teldrive/ui"

	"github.com/tgdrive/teldrive/pkg/queue"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"github.com/tgdrive/teldrive/pkg/services"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
			if err := loader.Validate(&cfg); err != nil {
				return err
			}
			return nil
		},
	}
	loader.RegisterFlags(cmd.Flags(), reflect.TypeFor[config.ServerCmdConfig]())
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

	lg := logging.Component("APP")
	defer lg.Sync()

	banner.PrintBanner(banner.StartupInfo{
		Version:  version.Version,
		Addr:     fmt.Sprintf(":%d", conf.Server.Port),
		LogLevel: conf.Log.Level,
	})

	port, err := findAvailablePort(conf.Server.Port)
	if err != nil {
		lg.Error("failed to find available port", zap.Error(err))
		os.Exit(1)
	}
	if port != conf.Server.Port {
		lg.Info("server.port_occupied", zap.Int("occupied_port", conf.Server.Port), zap.Int("new_port", port))
		conf.Server.Port = port
	}

	// Channel for background service initialization errors
	initErrCh := make(chan error, 3)

	// Create cancellable context for background services
	bgCtx, bgCancel := context.WithCancel(ctx)
	defer bgCancel()

	// Start Redis and cache initialization in background
	var redisClient *redis.Client
	var cacher cache.Cacher
	var botSelector tgc.BotSelector
	var redisOnce sync.Once
	var redisReady = make(chan struct{})

	go func() {
		client, err := cache.NewRedisClient(bgCtx, &conf.Redis)
		if err != nil {
			lg.Error("redis.client_failed", zap.Error(err))
			initErrCh <- fmt.Errorf("redis connection failed: %w", err)
			return
		}
		redisClient = client
		cacher = cache.NewCache(bgCtx, conf.Cache.MaxSize, redisClient, lg)
		botSelector = tgc.NewBotSelector(redisClient)
		redisOnce.Do(func() { close(redisReady) })
	}()

	// Initialize database (blocking - server needs this)
	pool, err := database.NewDatabase(ctx, &conf.DB, &conf.Log.DB, lg)
	if err != nil {
		lg.Error("failed to create database", zap.Error(err))
		os.Exit(1)
	}

	if err := database.MigrateDB(pool, true); err != nil {
		lg.Error("failed to migrate database", zap.Error(err))
		os.Exit(1)
	}
	repos := repositories.NewRepositories(pool)

	// Wait for cache to be ready before setting up server
	select {
	case <-redisReady:
		// Cache ready, continue
	case <-ctx.Done():
		lg.Error("server.startup_cancelled")
		os.Exit(1)
	}

	// Create broadcaster config from settings
	broadcasterConfig := events.BroadcasterConfig{
		DBWorkers:        conf.Events.DBWorkers,
		DBBufferSize:     conf.Events.DBBufferSize,
		DeduplicationTTL: conf.Events.DeduplicationTTL,
	}

	// Start event broadcaster in background
	var eventBroadcaster events.EventBroadcaster
	var eventsOnce sync.Once
	var eventsReady = make(chan struct{})

	go func() {
		eventBroadcaster = events.NewBroadcaster(bgCtx, repos.Events, redisClient, conf.Events.PollInterval, broadcasterConfig, logging.Component("EVENT"))
		eventsOnce.Do(func() { close(eventsReady) })
	}()

	// Wait for events to be ready
	select {
	case <-eventsReady:
		// Events ready, continue
	case <-ctx.Done():
		lg.Error("server.startup_cancelled")
		os.Exit(1)
	}

	// Setup server and queue client
	srv, riverClient := setupServer(conf, repos, cacher, lg, botSelector, eventBroadcaster)
	if err := riverClient.Start(bgCtx); err != nil {
		lg.Error("queue.start.failed", zap.Error(err))
		os.Exit(1)
	}

	serverErrCh := make(chan error, 1)
	go func() {
		lg.Info("server.started", zap.String("address", fmt.Sprintf("http://localhost:%d", conf.Server.Port)))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
		}
	}()

	// Main thread: wait for shutdown signal or fatal error
	select {
	case <-ctx.Done():
		lg.Info("server.shutdown_signal_received")
	case err := <-initErrCh:
		lg.Error("background_service.failed", zap.Error(err))
		os.Exit(1)
	case err := <-serverErrCh:
		lg.Error("server.crashed", zap.Error(err))
		os.Exit(1)
	}

	// Graceful shutdown sequence
	lg.Info("server.shutdown.starting")

	// Cancel background context to stop all background services
	bgCancel()

	// Shutdown event broadcaster
	if eventBroadcaster != nil {
		eventBroadcaster.Shutdown()
	}

	// Shutdown HTTP server with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), conf.Server.GracefulShutdown)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		lg.Error("server.shutdown.failed", zap.Error(err))
	}

	if err := riverClient.Stop(shutdownCtx); err != nil {
		lg.Error("queue.shutdown.failed", zap.Error(err))
	}

	// Close Redis client if it was created
	if redisClient != nil {
		redisClient.Close()
	}
	pool.Close()

	lg.Info("server.stopped")
}

const (
	defaultReadHeaderTimeout = 10 * time.Second
	defaultIdleTimeout       = 60 * time.Second
)

func setupServer(cfg *config.ServerCmdConfig, repos *repositories.Repositories, cache cache.Cacher, lg *zap.Logger, botSelector tgc.BotSelector, eventBroadcaster events.EventBroadcaster) (*http.Server, *river.Client[pgx.Tx]) {
	channelManager := tgc.NewChannelManager(repos, cache, &cfg.TG)
	telegramService := services.NewTelegramService(repos, cache, &cfg.TG, botSelector)

	apiSrv := services.NewApiService(repos, channelManager, cfg, cache, telegramService, eventBroadcaster, nil)
	riverClient, err := queue.NewClient(repos.Pool, services.NewJobExecutor(apiSrv), cfg.Queue, cfg.Jobs)
	if err != nil {
		lg.Error("failed to create river client", zap.Error(err))
		os.Exit(1)
	}
	apiSrv.SetJobClient(riverClient)
	apiSrv.SetPeriodicJobRegistry(riverClient.PeriodicJobs())
	if err := apiSrv.RegisterPeriodicJobs(context.Background()); err != nil {
		lg.Error("failed to register periodic jobs", zap.Error(err))
		os.Exit(1)
	}

	sec := auth.NewSecurityHandler(repos.Sessions, repos.APIKeys, cache, &cfg.JWT)
	rawSrv := services.NewRawService(apiSrv)
	srv, err := api.NewServer(apiSrv, rawSrv, sec)

	if err != nil {
		lg.Error("failed to create server", zap.Error(err))
		os.Exit(1)
	}

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
	mux.Use(chizap.ChizapWithConfig(logging.Component("HTTP"), &chizap.Config{
		SkipPathRegexps: []*regexp.Regexp{
			regexp.MustCompile(`^/(assets|images|docs)/.*`),
		},
		HTTPConfig: &cfg.Log.HTTP,
	}))
	mux.Use(requestmeta.Middleware)
	mux.Mount("/api/", http.StripPrefix("/api", srv))
	mux.Handle("/*", middleware.SPAHandler(ui.StaticFS))

	return &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           mux,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		IdleTimeout:       defaultIdleTimeout,
	}, riverClient
}
