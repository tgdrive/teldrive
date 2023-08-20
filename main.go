package main

import (
	"fmt"
	"time"

	"github.com/divyam234/teldrive/cache"
	"github.com/divyam234/teldrive/database"
	"github.com/divyam234/teldrive/routes"
	"github.com/divyam234/teldrive/utils"

	"github.com/divyam234/cors"
	"github.com/divyam234/teldrive/utils/cron"
	"github.com/gin-gonic/gin"
	"github.com/go-co-op/gocron"
	"github.com/joho/godotenv"
)

func main() {

	gin.SetMode(gin.ReleaseMode)

	router := gin.Default()

	godotenv.Load()

	utils.InitConfig()

	utils.InitializeLogger()

	database.InitDB()

	cache.CacheInit()

	utils.StartBotTgClients()

	cron.FilesDeleteJob()

	scheduler := gocron.NewScheduler(time.UTC)

	scheduler.Every(4).Hours().Do(cron.FilesDeleteJob)

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

	ok, _ := utils.PathExists("./sslcerts")
	config := utils.GetConfig()
	if ok && config.Https {
		router.RunTLS(fmt.Sprintf(":%d", config.Port), "./sslcerts/cert.pem", "./sslcerts/key.pem")
	} else {
		router.Run(fmt.Sprintf(":%d", config.Port))
	}
	scheduler.StartAsync()
}
