package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	varccache "github.com/tgdrive/varc/cache"

	api "github.com/tgdrive/teldrive/v2/internal/api"
	"github.com/tgdrive/teldrive/v2/internal/authn"
	"github.com/tgdrive/teldrive/v2/internal/bots"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/channels"
	"github.com/tgdrive/teldrive/v2/internal/config"
	"github.com/tgdrive/teldrive/v2/internal/database"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	userevents "github.com/tgdrive/teldrive/v2/internal/events"
	"github.com/tgdrive/teldrive/v2/internal/fileops"
	"github.com/tgdrive/teldrive/v2/internal/health"
	"github.com/tgdrive/teldrive/v2/internal/jobs"
	"github.com/tgdrive/teldrive/v2/internal/legacymigrate"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
	"github.com/tgdrive/teldrive/v2/internal/shares"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

var (
	ErrInvalidDependencies = errors.New("invalid application dependencies")
	ErrAlreadyRunning      = errors.New("application is already running")
)

type Dependencies struct {
	Storage       telegramstore.Storage
	Authenticator api.Authenticator
	Logger        *slog.Logger
	Version       string
}

type App struct {
	config            config.Config
	pool              *pgxpool.Pool
	http              *http.Server
	jobs              *jobs.Runtime
	events            *userevents.Service
	telegramDownloads *telegramstore.DownloadClientPool
	streamCache       *varccache.Cache

	mu      sync.Mutex
	running bool
	closed  bool
}

// New builds the complete v2 backend. It upgrades legacy databases and runs all
// TelDrive, River, and RiverPro migrations before opening the long-lived pool.
func New(ctx context.Context, cfg config.Config, dependencies Dependencies) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	report, migrated, err := legacymigrate.MigrateIfNeeded(ctx, cfg.Database, cfg.Security.DataKey)
	if err != nil {
		return nil, fmt.Errorf("migrate legacy database: %w", err)
	}
	if migrated {
		logger := dependencies.Logger
		if logger == nil {
			logger = slog.Default()
		}
		logger.Info("database.legacy_migration.completed",
			"users", report.Users,
			"channels", report.Channels,
			"bots", report.Bots,
			"folders", report.Folders,
			"files", report.Files,
			"file_parts", report.FileParts,
			"backup_schema", report.BackupSchema,
		)
	}
	if err := sqlcgen.ConfigureSchema(cfg.Database.Schema); err != nil {
		return nil, fmt.Errorf("configure database schema: %w", err)
	}
	if err := database.Migrate(ctx, cfg.Database); err != nil {
		return nil, fmt.Errorf("migrate application database: %w", err)
	}
	pool, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}
	cleanupPool := true
	defer func() {
		if cleanupPool {
			pool.Close()
		}
	}()

	secureCipher, err := secureblob.New(cfg.Security.DataKey)
	if err != nil {
		return nil, fmt.Errorf("create secure data cipher: %w", err)
	}
	telegram, err := buildTelegramComponents(cfg, pool, secureCipher, dependencies.Storage)
	if err != nil {
		return nil, err
	}
	cleanupTelegramDownloads := telegram.downloadClients != nil
	defer func() {
		if cleanupTelegramDownloads {
			closeCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
			defer cancel()
			_ = telegram.downloadClients.Close(closeCtx)
		}
	}()
	authService, err := authn.NewService(pool, secureCipher, telegram.login, authn.Config{
		SigningKey: cfg.Security.SigningKey, Issuer: cfg.Security.Issuer,
		AllowedUsers:   cfg.Security.AllowedUsers,
		AccessTokenTTL: cfg.Security.AccessTokenTTL, RefreshTokenTTL: cfg.Security.RefreshTokenTTL,
		LoginFlowTTL: cfg.Security.LoginFlowTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("create authentication service: %w", err)
	}
	storage := telegram.storage
	telegramAccount := telegram.account
	botService, err := bots.NewService(pool, secureCipher, telegram.verifier)
	if err != nil {
		return nil, fmt.Errorf("create bot service: %w", err)
	}
	authenticator := dependencies.Authenticator
	if authenticator == nil {
		authenticator = authService
	}

	eventService, err := userevents.NewService(pool, dependencies.Logger, userevents.Config{
		BatchSize:             int32(cfg.Events.BatchSize),
		MaxConnectionsPerUser: cfg.Events.MaxConnectionsPerUser,
		Heartbeat:             cfg.Events.Heartbeat,
		WriteTimeout:          cfg.Events.WriteTimeout,
		TicketTTL:             cfg.Events.TicketTTL,
		Retention:             cfg.Events.Retention,
		CleanupInterval:       cfg.Events.CleanupInterval,
		ConnectTimeout:        cfg.Events.ConnectTimeout,
		PingInterval:          cfg.Events.PingInterval,
		ReconnectMin:          cfg.Events.ReconnectMin,
		ReconnectMax:          cfg.Events.ReconnectMax,
	})
	if err != nil {
		return nil, fmt.Errorf("create event service: %w", err)
	}
	catalogService := catalog.NewService(pool)
	uploadService := uploads.NewService(pool, cfg.Uploads.SessionTTL)
	channelService := channels.NewService(pool, channels.TelegramCreator{Storage: storage}, channels.Config{
		PartLimit: cfg.Telegram.ChannelPartLimit, AutoCreate: cfg.Telegram.AutoChannelCreate,
		NamePrefix: cfg.Telegram.ChannelNamePrefix,
	})
	keys := make(transfer.StaticKeyProvider, len(cfg.Encryption.Keys))
	for version, key := range cfg.Encryption.Keys {
		keys[version] = key
	}
	uploadPipeline := transfer.NewPipeline(uploadService, channelService, storage, keys, transfer.Config{
		UploadThreads: cfg.Telegram.UploadThreads, RandomizePartNames: cfg.Telegram.RandomizePartNames,
	})
	var streamCache *varccache.Cache
	streamCacheDir := strings.TrimSpace(cfg.Cache.Stream.Dir)
	if streamCacheDir != "" {
		streamCacheDir, err = expandHomePath(streamCacheDir)
		if err != nil {
			return nil, fmt.Errorf("resolve stream cache directory: %w", err)
		}
		cacheOptions := varccache.DefaultOptions()
		cacheOptions.CacheMaxAge = cfg.Cache.Stream.MaxAge
		cacheOptions.CacheMaxSize = int64(cfg.Cache.Stream.MaxSize)
		cacheOptions.CacheMinFreeSpace = int64(cfg.Cache.Stream.MinFreeSpace)
		cacheOptions.CachePollInterval = cfg.Cache.Stream.PollInterval
		cacheOptions.CacheShardDepth = cfg.Cache.Stream.ShardDepth
		cacheOptions.ChunkSize = int64(cfg.Cache.Stream.ChunkSize)
		cacheOptions.ChunkStreams = cfg.Cache.Stream.ChunkStreams
		cacheOptions.ReadAhead = int64(cfg.Cache.Stream.ReadAhead)
		streamCache, err = varccache.New(context.Background(), streamCacheDir, cacheOptions)
		if err != nil {
			return nil, fmt.Errorf("create stream cache: %w", err)
		}
	}
	cleanupStreamCache := streamCache != nil
	defer func() {
		if cleanupStreamCache {
			_ = streamCache.Close()
		}
	}()
	downloader := transfer.NewDownloader(catalogService, storage, keys, streamCache)
	fileService, err := fileops.NewService(pool, catalogService, channelService, storage)
	if err != nil {
		return nil, fmt.Errorf("create file operations service: %w", err)
	}
	shareService, err := shares.NewService(pool, catalogService)
	if err != nil {
		return nil, fmt.Errorf("create share service: %w", err)
	}
	healthService := health.NewService(dependencies.Version, pool)
	jobRuntime, err := jobs.NewRuntimeWithServices(
		pool, storage, cfg.Database.Schema, botService, secureCipher, cfg.Uploads.SessionTTL,
		jobs.UploaderServices{Catalog: catalogService, Uploads: uploadService, Pipeline: uploadPipeline, DefaultKeyVersion: cfg.Encryption.DefaultVersion},
		fileService,
	)
	if err != nil {
		return nil, fmt.Errorf("create job runtime: %w", err)
	}
	handler := api.NewHandler(
		catalogService, uploadService, uploadPipeline, downloader, healthService,
		cfg.Encryption.DefaultVersion, eventService,
	).ConfigureDomains(authService, botService, channelService, fileService, shareService, telegramAccount).
		ConfigureJobs(jobRuntime)
	httpServer, err := api.NewServer(handler, api.NewSecurity(authenticator, eventService))
	if err != nil {
		return nil, fmt.Errorf("create generated HTTP server: %w", err)
	}
	webUI, err := newWebUIHandler()
	if err != nil {
		return nil, fmt.Errorf("configure web UI: %w", err)
	}
	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Use(requestIDHeader)
	requestSecurity, err := newRequestSecurity(cfg.HTTP.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("configure trusted proxies: %w", err)
	}
	routeApplication(mux, requestSecurity.middleware(browserCSRFMiddleware(browserSessionRenewalMiddleware(authService, httpServer))), webUI)

	application := &App{
		config:            cfg,
		pool:              pool,
		jobs:              jobRuntime,
		events:            eventService,
		telegramDownloads: telegram.downloadClients,
		streamCache:       streamCache,
		http: &http.Server{
			Addr:              cfg.HTTP.Address,
			Handler:           mux,
			ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
			ReadTimeout:       cfg.HTTP.ReadTimeout,
			WriteTimeout:      cfg.HTTP.WriteTimeout,
			IdleTimeout:       cfg.HTTP.IdleTimeout,
		},
	}
	cleanupPool = false
	cleanupTelegramDownloads = false
	cleanupStreamCache = false
	return application, nil
}

func expandHomePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	if strings.HasPrefix(path, "~") {
		return "", fmt.Errorf("unsupported home-relative path %q", path)
	}
	return path, nil
}

func requestIDHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := middleware.GetReqID(r.Context())
		if requestID != "" {
			w.Header().Set("X-Request-ID", requestID)
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) Handler() http.Handler {
	if a == nil || a.http == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "application is not configured", http.StatusServiceUnavailable)
		})
	}
	return a.http.Handler
}

func (a *App) Pool() *pgxpool.Pool {
	if a == nil {
		return nil
	}
	return a.pool
}

// Run binds the configured address and owns the full server lifecycle until the
// context is cancelled or the HTTP server fails. It never searches for another
// port, which prevents production deployments from silently becoming unreachable.
func (a *App) Run(ctx context.Context) error {
	if a == nil || a.http == nil || a.jobs == nil || a.events == nil || a.pool == nil {
		return ErrInvalidDependencies
	}
	listener, err := net.Listen("tcp", a.config.HTTP.Address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", a.config.HTTP.Address, err)
	}
	return a.Serve(ctx, listener)
}

// Serve is Run with a caller-provided listener, which makes lifecycle behavior
// deterministic in tests and supports socket activation.
func (a *App) Serve(ctx context.Context, listener net.Listener) error {
	if a == nil || a.http == nil || a.jobs == nil || a.events == nil || a.pool == nil || listener == nil {
		return ErrInvalidDependencies
	}
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return ErrAlreadyRunning
	}
	if a.closed {
		a.mu.Unlock()
		return errors.New("application is closed")
	}
	a.running = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
	}()

	if err := a.events.Start(ctx); err != nil {
		_ = listener.Close()
		return fmt.Errorf("start event service: %w", err)
	}
	if err := a.jobs.Start(ctx); err != nil {
		_ = listener.Close()
		closeCtx, cancel := context.WithTimeout(context.Background(), a.config.HTTP.ShutdownTimeout)
		defer cancel()
		return errors.Join(err, a.events.Close(closeCtx))
	}
	serveErrors := make(chan error, 1)
	go func() {
		err := a.http.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveErrors <- err
	}()

	select {
	case err := <-serveErrors:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.config.HTTP.ShutdownTimeout)
		defer cancel()
		return errors.Join(err, a.Shutdown(shutdownCtx))
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.config.HTTP.ShutdownTimeout)
		defer cancel()
		shutdownErr := a.Shutdown(shutdownCtx)
		serveErr := <-serveErrors
		return errors.Join(shutdownErr, serveErr)
	}
}

// Shutdown stops long-lived SSE handlers and HTTP, closes the stream cache and warm Telegram download clients,
// drains RiverPro workers, and finally closes PostgreSQL. It is safe to call repeatedly.
func (a *App) Shutdown(ctx context.Context) error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	a.mu.Unlock()

	var result error
	if a.events != nil {
		result = errors.Join(result, a.events.Close(ctx))
	}
	if a.http != nil {
		result = errors.Join(result, a.http.Shutdown(ctx))
	}
	if a.streamCache != nil {
		result = errors.Join(result, a.streamCache.Close())
	}
	if a.telegramDownloads != nil {
		result = errors.Join(result, a.telegramDownloads.Close(ctx))
	}
	if a.jobs != nil {
		if err := a.jobs.Stop(ctx); err != nil && !errors.Is(err, jobs.ErrRuntimeNotConfigured) {
			result = errors.Join(result, err)
		}
	}
	if a.pool != nil {
		a.pool.Close()
	}
	return result
}

// Close provides a bounded non-request-context shutdown for callers that use
// App as a resource rather than running Serve directly.
func (a *App) Close() error {
	if a == nil {
		return nil
	}
	timeout := a.config.HTTP.ShutdownTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return a.Shutdown(ctx)
}
