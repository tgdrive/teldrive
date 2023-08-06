package main

import (
	"github.com/divyam234/teldrive-go/cache"
	"github.com/divyam234/teldrive-go/database"
	"github.com/divyam234/teldrive-go/routes"
	"github.com/divyam234/teldrive-go/utils"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {

	gin.SetMode(gin.ReleaseMode)

	router := gin.Default()

	godotenv.Load()

	database.InitDB()

	cache.CacheInit()

	utils.StartClients()

	router.Use(gin.ErrorLogger())

	routes.GetRoutes(router)

	router.Run(":8080")

}
