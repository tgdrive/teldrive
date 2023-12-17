package main

import (
	"fmt"
	"mime"

	"github.com/divyam234/teldrive/api"
	"github.com/divyam234/teldrive/internal/cron"
	"github.com/divyam234/teldrive/internal/logger"
	"github.com/divyam234/teldrive/pkg/database"

	"github.com/divyam234/teldrive/config"
	"github.com/divyam234/teldrive/internal/cache"
	"github.com/gin-gonic/gin"
)

func main() {

	config.InitConfig()

	if config.GetConfig().Dev {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	log := logger.InitLogger()

	database.InitDB()

	cache.InitCache()

	cron.StartCronJobs(log)

	mime.AddExtensionType(".js", "application/javascript")

	r := api.InitRouter(log)

	r.Run(fmt.Sprintf(":%d", config.GetConfig().Port))

}
