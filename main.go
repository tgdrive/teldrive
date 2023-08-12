package main

import (
	"time"

	"github.com/divyam234/teldrive-go/cache"
	"github.com/divyam234/teldrive-go/database"
	"github.com/divyam234/teldrive-go/routes"
	"github.com/divyam234/teldrive-go/utils"

	"github.com/divyam234/cors"
	"github.com/gin-gonic/gin"
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

	router.Run(":8080")
}
