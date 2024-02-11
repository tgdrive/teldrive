package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/divyam234/teldrive/api"
	"github.com/divyam234/teldrive/internal/config"
	"github.com/divyam234/teldrive/internal/database"
	"github.com/divyam234/teldrive/internal/kv"
	"github.com/divyam234/teldrive/internal/middleware"
	"github.com/divyam234/teldrive/internal/tgc"
	"github.com/divyam234/teldrive/internal/utils"
	"github.com/divyam234/teldrive/pkg/controller"
	"github.com/divyam234/teldrive/pkg/cron"
	"github.com/divyam234/teldrive/pkg/logging"
	"github.com/divyam234/teldrive/pkg/services"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"go.uber.org/zap/zapcore"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start Teldrive Server",
	Run:   runApplication,
}

func runApplication(cmd *cobra.Command, args []string) {
	if configFile == "" {
		configFile = filepath.Join(utils.ExecutableDir(), "teldrive.yml")
	}
	conf, err := config.Load(configFile)
	if err != nil {
		log.Fatal(err)
	}
	logging.SetConfig(&logging.Config{
		Encoding:    conf.LoggingConfig.Encoding,
		Level:       zapcore.Level(conf.LoggingConfig.Level),
		Development: conf.LoggingConfig.Development,
	})
	defer logging.DefaultLogger().Sync()

	app := fx.New(
		fx.Supply(conf),
		fx.Supply(logging.DefaultLogger().Desugar()),
		fx.NopLogger,
		fx.StopTimeout(conf.ServerConfig.GracefulShutdown+time.Second),
		fx.Invoke(
			initApp,
			cron.StartCronJobs,
		),
		fx.Provide(
			database.NewDatabase,
			kv.NewBoltKV,
			tgc.NewStreamWorker,
			tgc.NewUploadWorker,
			services.NewAuthService,
			services.NewFileService,
			services.NewUploadService,
			services.NewUserService,
			controller.NewController,
		),
	)
	app.Run()
}

func initApp(lc fx.Lifecycle, cfg *config.Config, c *controller.Controller) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(ginzap.GinzapWithConfig(logging.DefaultLogger().Desugar(), &ginzap.Config{
		TimeFormat: time.RFC3339,
		UTC:        true,
		SkipPaths:  []string{"/favicon.ico", "/assets"},
	}))

	r.Use(middleware.Cors())

	r = api.InitRouter(r, c, cfg)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.ServerConfig.Port),
		Handler:      r,
		ReadTimeout:  cfg.ServerConfig.ReadTimeout,
		WriteTimeout: cfg.ServerConfig.WriteTimeout,
	}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logging.FromContext(ctx).Infof("Started server http://localhost:%d", cfg.ServerConfig.Port)
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logging.DefaultLogger().Errorw("failed to close http server", "err", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logging.FromContext(ctx).Info("Stopped server")
			return srv.Shutdown(ctx)
		},
	})
	return r
}
