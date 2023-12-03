package main

import (
	"fmt"
	"mime"
	"path/filepath"

	"github.com/divyam234/teldrive/api"
	"github.com/divyam234/teldrive/internal/cron"
	"github.com/divyam234/teldrive/internal/logger"
	"github.com/divyam234/teldrive/internal/utils"
	"github.com/divyam234/teldrive/pkg/database"

	cnf "github.com/divyam234/teldrive/config"
	"github.com/divyam234/teldrive/internal/cache"
	"github.com/gin-gonic/gin"
)

func main() {

	gin.SetMode(gin.ReleaseMode)

	cnf.InitConfig()

	logger.InitLogger()

	database.InitDB()

	cache.InitCache()

	cron.StartCronJobs()

	mime.AddExtensionType(".js", "application/javascript")

	r := api.InitRouter()

	config := cnf.GetConfig()
	certDir := filepath.Join(config.ExecDir, "sslcerts")
	ok, _ := utils.PathExists(certDir)
	if ok && config.Https {
		r.RunTLS(fmt.Sprintf(":%d", config.Port), filepath.Join(certDir, "cert.pem"), filepath.Join(certDir, "key.pem"))
	} else {
		r.Run(fmt.Sprintf(":%d", config.Port))
	}
}
