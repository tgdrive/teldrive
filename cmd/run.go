package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-co-op/gocron"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/appcontext"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/chizap"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/database"
	"github.com/tgdrive/teldrive/internal/duration"
	"github.com/tgdrive/teldrive/internal/kv"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/middleware"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/ui"

	"github.com/tgdrive/teldrive/pkg/cron"
	"github.com/tgdrive/teldrive/pkg/services"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
)

func NewRun() *cobra.Command {
	config := config.Config{}
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Start Teldrive Server",
		Run: func(cmd *cobra.Command, args []string) {
			runApplication(&config)

		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initViperConfig(cmd)
		},
	}

	runCmd.Flags().StringP("config", "c", "", "Config file path (default $HOME/.teldrive/config.toml)")
	runCmd.Flags().IntVarP(&config.Server.Port, "server-port", "p", 8080, "Server port")
	duration.DurationVar(runCmd.Flags(), &config.Server.GracefulShutdown, "server-graceful-shutdown", 10*time.Second, "Server graceful shutdown timeout")
	runCmd.Flags().BoolVar(&config.Server.EnablePprof, "server-enable-pprof", false, "Enable Pprof Profiling")
	duration.DurationVar(runCmd.Flags(), &config.Server.ReadTimeout, "server-read-timeout", 1*time.Hour, "Server read timeout")
	duration.DurationVar(runCmd.Flags(), &config.Server.WriteTimeout, "server-write-timeout", 1*time.Hour, "Server write timeout")

	runCmd.Flags().BoolVar(&config.CronJobs.Enable, "cronjobs-enable", true, "Run cron jobs")
	duration.DurationVar(runCmd.Flags(), &config.CronJobs.CleanFilesInterval, "cronjobs-clean-files-interval", 1*time.Hour, "Clean files interval")
	duration.DurationVar(runCmd.Flags(), &config.CronJobs.CleanUploadsInterval, "cronjobs-clean-uploads-interval", 12*time.Hour, "Clean uploads interval")
	duration.DurationVar(runCmd.Flags(), &config.CronJobs.FolderSizeInterval, "cronjobs-folder-size-interval", 2*time.Hour, "Folder size update  interval")

	runCmd.Flags().IntVar(&config.Cache.MaxSize, "cache-max-size", 10*1024*1024, "Max Cache max size (memory)")
	runCmd.Flags().StringVar(&config.Cache.RedisAddr, "cache-redis-addr", "", "Redis address")
	runCmd.Flags().StringVar(&config.Cache.RedisPass, "cache-redis-pass", "", "Redis password")

	runCmd.Flags().IntVarP(&config.Log.Level, "log-level", "", -1, "Logging level")
	runCmd.Flags().StringVar(&config.Log.File, "log-file", "", "Logging file path")
	runCmd.Flags().BoolVar(&config.Log.Development, "log-development", false, "Enable development mode")

	runCmd.Flags().StringVar(&config.JWT.Secret, "jwt-secret", "", "JWT secret key")
	duration.DurationVar(runCmd.Flags(), &config.JWT.SessionTime, "jwt-session-time", (30*24)*time.Hour, "JWT session duration")
	runCmd.Flags().StringSliceVar(&config.JWT.AllowedUsers, "jwt-allowed-users", []string{}, "Allowed users")

	runCmd.Flags().StringVar(&config.DB.DataSource, "db-data-source", "", "Database connection string")
	runCmd.Flags().IntVar(&config.DB.LogLevel, "db-log-level", 1, "Database log level")
	runCmd.Flags().BoolVar(&config.DB.PrepareStmt, "db-prepare-stmt", true, "Enable prepared statements")
	runCmd.Flags().BoolVar(&config.DB.Pool.Enable, "db-pool-enable", true, "Enable database pool")
	runCmd.Flags().IntVar(&config.DB.Pool.MaxIdleConnections, "db-pool-max-open-connections", 25, "Database max open connections")
	runCmd.Flags().IntVar(&config.DB.Pool.MaxIdleConnections, "db-pool-max-idle-connections", 25, "Database max idle connections")
	duration.DurationVar(runCmd.Flags(), &config.DB.Pool.MaxLifetime, "db-pool-max-lifetime", 10*time.Minute, "Database max connection lifetime")

	runCmd.Flags().IntVar(&config.TG.AppId, "tg-app-id", 0, "Telegram app ID")
	runCmd.Flags().StringVar(&config.TG.AppHash, "tg-app-hash", "", "Telegram app hash")
	runCmd.Flags().StringVar(&config.TG.SessionFile, "tg-session-file", "", "Bot session file path")
	runCmd.Flags().BoolVar(&config.TG.RateLimit, "tg-rate-limit", true, "Enable rate limiting for telegram client")
	runCmd.Flags().IntVar(&config.TG.RateBurst, "tg-rate-burst", 5, "Limiting burst for telegram client")
	runCmd.Flags().IntVar(&config.TG.Rate, "tg-rate", 100, "Limiting rate for telegram client")
	runCmd.Flags().StringVar(&config.TG.DeviceModel, "tg-device-model",
		"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/116.0", "Device model")
	runCmd.Flags().StringVar(&config.TG.SystemVersion, "tg-system-version", "Win32", "System version")
	runCmd.Flags().StringVar(&config.TG.AppVersion, "tg-app-version", "4.6.3 K", "App version")
	runCmd.Flags().StringVar(&config.TG.LangCode, "tg-lang-code", "en", "Language code")
	runCmd.Flags().StringVar(&config.TG.SystemLangCode, "tg-system-lang-code", "en-US", "System language code")
	runCmd.Flags().StringVar(&config.TG.LangPack, "tg-lang-pack", "webk", "Language pack")
	runCmd.Flags().StringVar(&config.TG.Proxy, "tg-proxy", "", "HTTP OR SOCKS5 proxy URL")
	runCmd.Flags().BoolVar(&config.TG.DisableStreamBots, "tg-disable-stream-bots", false, "Disable Stream bots")
	runCmd.Flags().BoolVar(&config.TG.EnableLogging, "tg-enable-logging", false, "Enable telegram client logging")
	runCmd.Flags().StringVar(&config.TG.Uploads.EncryptionKey, "tg-uploads-encryption-key", "", "Uploads encryption key")
	runCmd.Flags().IntVar(&config.TG.Uploads.Threads, "tg-uploads-threads", 8, "Uploads threads")
	runCmd.Flags().IntVar(&config.TG.Uploads.MaxRetries, "tg-uploads-max-retries", 10, "Uploads Retries")
	runCmd.Flags().Int64Var(&config.TG.PoolSize, "tg-pool-size", 8, "Telegram Session pool size")
	duration.DurationVar(runCmd.Flags(), &config.TG.ReconnectTimeout, "tg-reconnect-timeout", 5*time.Minute, "Reconnect Timeout")
	duration.DurationVar(runCmd.Flags(), &config.TG.Uploads.Retention, "tg-uploads-retention", (24*7)*time.Hour, "Uploads retention duration")
	duration.DurationVar(runCmd.Flags(), &config.TG.BgBotsCheckInterval, "tg-bg-bots-check-interval", 4*time.Hour, "Interval for checking Idle background bots")
	runCmd.Flags().IntVar(&config.TG.Stream.MultiThreads, "tg-stream-multi-threads", 0, "Stream multi-threads")
	runCmd.Flags().IntVar(&config.TG.Stream.Buffers, "tg-stream-buffers", 8, "No of Stream buffers")
	duration.DurationVar(runCmd.Flags(), &config.TG.Stream.ChunkTimeout, "tg-stream-chunk-timeout", 20*time.Second, "Chunk Fetch Timeout")
	runCmd.MarkFlagRequired("tg-app-id")
	runCmd.MarkFlagRequired("tg-app-hash")
	runCmd.MarkFlagRequired("db-data-source")
	runCmd.MarkFlagRequired("jwt-secret")

	return runCmd
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

func runApplication(conf *config.Config) {
	logging.SetConfig(&logging.Config{
		Level:       zapcore.Level(conf.Log.Level),
		Development: conf.Log.Development,
		FilePath:    conf.Log.File,
	})

	ctx, cancel := context.WithCancel(context.Background())

	lg := logging.DefaultLogger().Sugar()

	defer func() {
		logging.DefaultLogger().Sync()
		cancel()
	}()

	port, err := findAvailablePort(conf.Server.Port)
	if err != nil {
		lg.Fatalw("failed to find available port", "err", err)
	}
	if port != conf.Server.Port {
		lg.Infof("Port %d is occupied, using port %d instead", conf.Server.Port, port)
		conf.Server.Port = port
	}

	scheduler := gocron.NewScheduler(time.UTC)

	cacher := cache.NewCache(ctx, conf)

	db, err := database.NewDatabase(conf, lg)

	if err != nil {
		lg.Fatalw("failed to create database", "err", err)
	}

	kv := kv.NewBoltKV(conf)

	worker := tgc.NewBotWorker()

	srv := setupServer(conf, db, cacher, kv, worker)

	stop := make(chan os.Signal, 1)

	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	cron.StartCronJobs(scheduler, db, conf)

	go func() {
		lg.Infof("Server started at http://localhost:%d", conf.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			lg.Errorw("failed to start server", "err", err)
		}
	}()

	<-stop

	lg.Info("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), conf.Server.GracefulShutdown)

	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		lg.Errorw("server shutdown failed", "err", err)
	}

	scheduler.Stop()

	lg.Info("Server stopped")
}

func setupServer(cfg *config.Config, db *gorm.DB, cache cache.Cacher, kv kv.KV, worker *tgc.BotWorker) *http.Server {

	lg := logging.DefaultLogger()

	apiSrv := services.NewApiService(db, cfg, cache, kv, worker)

	srv, err := api.NewServer(apiSrv, auth.NewSecurityHandler(db, cache, cfg))

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

func initViperConfig(cmd *cobra.Command) error {

	viper.SetConfigType("toml")

	cfgFile := cmd.Flags().Lookup("config").Value.String()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, _ := homedir.Dir()
		viper.AddConfigPath(filepath.Join(home, ".teldrive"))
		viper.AddConfigPath(".")
		viper.AddConfigPath(utils.ExecutableDir())
		viper.SetConfigName("config")
	}

	viper.SetEnvPrefix("teldrive")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
	viper.ReadInConfig()
	bindFlags(cmd.Flags(), "", reflect.ValueOf(config.Config{}))
	return nil

}
func bindFlags(flags *pflag.FlagSet, prefix string, v reflect.Value) {
	t := v.Type()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	for i := range t.NumField() {
		field := t.Field(i)
		switch field.Type.Kind() {
		case reflect.Struct:
			bindFlags(flags, fmt.Sprintf("%s.%s", prefix, strings.ToLower(field.Name)), v.Field(i))
		default:
			newPrefix := prefix[1:]
			newName := modifyFlag(field.Name)
			configName := fmt.Sprintf("%s.%s", newPrefix, newName)
			flag := flags.Lookup(fmt.Sprintf("%s-%s", strings.ReplaceAll(newPrefix, ".", "-"), newName))
			if !flag.Changed && viper.IsSet(configName) {
				confVal := viper.Get(configName)
				if field.Type.Kind() == reflect.Slice {
					sliceValue, ok := confVal.([]interface{})
					if ok {
						for _, v := range sliceValue {
							flag.Value.Set(fmt.Sprintf("%v", v))
						}
					}
				} else {
					flags.Set(flag.Name, fmt.Sprintf("%v", confVal))
				}
			}
		}
	}
}

func modifyFlag(s string) string {
	var result []rune

	for i, c := range s {
		if i > 0 && unicode.IsUpper(c) {
			result = append(result, '-')
		}
		result = append(result, unicode.ToLower(c))
	}

	return string(result)
}
