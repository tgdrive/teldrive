package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/divyam234/teldrive/cache"
	"github.com/divyam234/teldrive/database"
	"github.com/divyam234/teldrive/routes"
	"github.com/divyam234/teldrive/utils"

	"github.com/divyam234/cors"
	"github.com/divyam234/teldrive/utils/cron"
	"github.com/gin-gonic/gin"
	"github.com/go-co-op/gocron"
)

func main() {

	gin.SetMode(gin.ReleaseMode)

	router := gin.Default()

	utils.InitConfig()

	utils.InitializeLogger()

	database.InitDB()

	cache.CacheInit()

	utils.InitBotClients()

	cron.FilesDeleteJob()

	scheduler := gocron.NewScheduler(time.UTC)

	scheduler.Every(1).Hours().Do(cron.FilesDeleteJob)

	router.Use(cors.New(cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type"},
		AllowCredentials: true,
		AllowOriginFunc: func(origin string) bool {
			return true
		},
		MaxAge: 12 * time.Hour,
	}))

	router.Use(gin.ErrorLogger())

	routes.GetRoutes(router)

	config := utils.GetConfig()
	certDir := filepath.Join(config.ExecDir, "sslcerts")
	ok, _ := utils.PathExists(certDir)
	if ok && config.Https {
		router.RunTLS(fmt.Sprintf(":%d", config.Port), filepath.Join(certDir, "cert.pem"), filepath.Join(certDir, "key.pem"))
	} else {
		router.Run(fmt.Sprintf(":%d", config.Port))
	}
	scheduler.StartAsync()
}
