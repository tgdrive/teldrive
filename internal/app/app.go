package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
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
	"github.com/tgdrive/teldrive/pkg/queue"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"github.com/tgdrive/teldrive/pkg/services"
	"github.com/tgdrive/teldrive/ui"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	defaultReadHeaderTimeout = 10 * time.Second
	defaultIdleTimeout       = 60 * time.Second
)

type Hook struct {
	Name  string
	Start func(context.Context) error
	Stop  func(context.Context) error
}

type App struct {
	cfg         *config.ServerCmdConfig
	log         *zap.Logger
	hooks       []Hook
	serverErrCh chan error
}

func New(ctx context.Context, cfg *config.ServerCmdConfig) (_ *App, err error) {
	log := configureLogging(cfg)
	cleanups := make([]func(), 0, 4)
	defer func() {
		if err == nil {
			return
		}
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
		_ = log.Sync()
	}()

	port, err := findAvailablePort(cfg.Server.Port)
	if err != nil {
		return nil, fmt.Errorf("find available port: %w", err)
	}
	if port != cfg.Server.Port {
		log.Info("server.port_occupied", zap.Int("occupied_port", cfg.Server.Port), zap.Int("new_port", port))
		cfg.Server.Port = port
	}

	banner.PrintBanner(banner.StartupInfo{
		Version:  version.Version,
		Addr:     fmt.Sprintf(":%d", cfg.Server.Port),
		LogLevel: cfg.Log.Level,
	})

	pool, err := database.NewDatabase(ctx, &cfg.DB, &cfg.Log.DB, log)
	if err != nil {
		return nil, fmt.Errorf("create database: %w", err)
	}
	cleanups = append(cleanups, pool.Close)

	if err := database.MigrateDB(pool, true); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	repos := repositories.NewRepositories(pool)

	redisClient, err := cache.NewRedisClient(ctx, &cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("create redis client: %w", err)
	}
	if redisClient != nil {
		cleanups = append(cleanups, func() {
			_ = redisClient.Close()
		})
	}

	cacher := cache.NewCache(ctx, cfg.Cache.MaxSize, redisClient, log)
	botSelector := tgc.NewBotSelector(redisClient)
	broadcaster := events.NewBroadcaster(ctx, repos.Events, redisClient, cfg.Events.PollInterval, events.BroadcasterConfig{
		DBWorkers:        cfg.Events.DBWorkers,
		DBBufferSize:     cfg.Events.DBBufferSize,
		DeduplicationTTL: cfg.Events.DeduplicationTTL,
	}, logging.Component("EVENT"))
	cleanups = append(cleanups, broadcaster.Shutdown)

	httpServer, riverClient, err := buildHTTPServer(cfg, repos, cacher, log, botSelector, broadcaster)
	if err != nil {
		return nil, err
	}

	app := &App{
		cfg:         cfg,
		log:         log,
		serverErrCh: make(chan error, 1),
	}

	app.hooks = []Hook{
		{Name: "database", Stop: func(context.Context) error { pool.Close(); return nil }},
		{Name: "redis", Stop: func(context.Context) error {
			if redisClient != nil {
				return redisClient.Close()
			}
			return nil
		}},
		{Name: "events", Stop: func(context.Context) error {
			broadcaster.Shutdown()
			return nil
		}},
		{Name: "queue", Start: func(ctx context.Context) error {
			if err := riverClient.Start(ctx); err != nil {
				return fmt.Errorf("start queue: %w", err)
			}
			return nil
		}, Stop: func(ctx context.Context) error {
			if err := riverClient.Stop(ctx); err != nil {
				return fmt.Errorf("stop queue: %w", err)
			}
			return nil
		}},
		{Name: "http", Start: func(context.Context) error {
			go func() {
				log.Info("server.started", zap.String("address", fmt.Sprintf("http://localhost:%d", cfg.Server.Port)))
				if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					select {
					case app.serverErrCh <- err:
					default:
					}
				}
			}()
			return nil
		}, Stop: func(ctx context.Context) error {
			if err := httpServer.Shutdown(ctx); err != nil {
				return fmt.Errorf("shutdown http server: %w", err)
			}
			return nil
		}},
	}

	cleanups = nil
	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	defer func() { _ = a.log.Sync() }()

	started := 0
	for i, hook := range a.hooks {
		if hook.Start != nil {
			if err := hook.Start(ctx); err != nil {
				shutdownCtx, cancel := a.shutdownContext()
				defer cancel()
				stopErr := a.stop(shutdownCtx, i)
				if stopErr != nil {
					return errors.Join(err, stopErr)
				}
				return err
			}
		}
		started = i + 1
	}

	var runErr error
	select {
	case <-ctx.Done():
		a.log.Info("server.shutdown_signal_received")
	case err := <-a.serverErrCh:
		runErr = fmt.Errorf("server crashed: %w", err)
		a.log.Error("server.crashed", zap.Error(err))
	}

	a.log.Info("server.shutdown.starting")
	shutdownCtx, cancel := a.shutdownContext()
	defer cancel()
	stopErr := a.stop(shutdownCtx, started)
	if stopErr != nil {
		if runErr != nil {
			return errors.Join(runErr, stopErr)
		}
		return stopErr
	}

	a.log.Info("server.stopped")
	return runErr
}

func (a *App) shutdownContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), a.cfg.Server.GracefulShutdown)
}

func (a *App) stop(ctx context.Context, count int) error {
	var errs []error
	for i := count - 1; i >= 0; i-- {
		hook := a.hooks[i]
		if hook.Stop == nil {
			continue
		}
		if err := hook.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s stop: %w", hook.Name, err))
		}
	}
	return errors.Join(errs...)
}

func configureLogging(cfg *config.ServerCmdConfig) *zap.Logger {
	lvl, err := zapcore.ParseLevel(cfg.Log.Level)
	if err != nil {
		lvl = zapcore.InfoLevel
	}

	logging.SetConfig(&logging.Config{
		Level:    lvl,
		FilePath: cfg.Log.File,
	})

	return logging.Component("APP")
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

func buildHTTPServer(cfg *config.ServerCmdConfig, repos *repositories.Repositories, cacher cache.Cacher, log *zap.Logger, botSelector tgc.BotSelector, broadcaster events.EventBroadcaster) (*http.Server, *river.Client[pgx.Tx], error) {
	channelManager := tgc.NewChannelManager(repos, cacher, &cfg.TG)
	telegramService := services.NewTelegramService(repos, cacher, &cfg.TG, botSelector)
	jobClientRef := services.NewJobClientRef()
	periodicRegistryRef := services.NewPeriodicJobRegistryRef()

	apiSrv := services.NewApiService(repos, channelManager, cfg, cacher, telegramService, broadcaster, jobClientRef, periodicRegistryRef)
	riverClient, err := queue.NewClient(repos.Pool, services.NewJobExecutor(apiSrv), cfg.Queue, cfg.Jobs)
	if err != nil {
		return nil, nil, fmt.Errorf("create river client: %w", err)
	}
	jobClientRef.Set(riverClient)
	periodicRegistryRef.Set(riverClient.PeriodicJobs())
	if err := apiSrv.RegisterPeriodicJobs(context.Background()); err != nil {
		return nil, nil, fmt.Errorf("register periodic jobs: %w", err)
	}

	sec := auth.NewSecurityHandler(repos.Sessions, repos.APIKeys, cacher, &cfg.JWT)
	rawSrv := services.NewRawService(apiSrv)
	srv, err := api.NewServer(apiSrv, rawSrv, sec)
	if err != nil {
		return nil, nil, fmt.Errorf("create api server: %w", err)
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
	mux.Use(middleware.InjectLogger(log))
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
	}, riverClient, nil
}
