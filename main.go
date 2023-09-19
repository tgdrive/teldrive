package main

import (
	"fmt"
	"mime"
	"path/filepath"
	"time"

	"github.com/divyam234/teldrive/database"
	"github.com/divyam234/teldrive/routes"
	"github.com/divyam234/teldrive/ui"
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

	scheduler := gocron.NewScheduler(time.UTC)

	scheduler.Every(1).Hour().Do(cron.FilesDeleteJob)

	scheduler.StartAsync()

	router.Use(cors.New(cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Content-Length", "Content-Type", "If-Modified-Since", "Range"},
		ExposeHeaders:    []string{"Content-Length", "Content-Range"},
		AllowCredentials: true,
		AllowOriginFunc: func(origin string) bool {
			return true
		},
		MaxAge: 12 * time.Hour,
	}))

	mime.AddExtensionType(".js", "application/javascript")

	router.Use(gin.ErrorLogger())

	routes.AddRoutes(router)

	ui.AddRoutes(router)

	config := utils.GetConfig()
	certDir := filepath.Join(config.ExecDir, "sslcerts")
	ok, _ := utils.PathExists(certDir)
	if ok && config.Https {
		router.RunTLS(fmt.Sprintf(":%d", config.Port), filepath.Join(certDir, "cert.pem"), filepath.Join(certDir, "key.pem"))
	} else {
		router.Run(fmt.Sprintf(":%d", config.Port))
	}
}
