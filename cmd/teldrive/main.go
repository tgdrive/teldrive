package main

import (
	"fmt"
	"mime"
	"path/filepath"

	"github.com/divyam234/teldrive/api"
	"github.com/divyam234/teldrive/internal/logger"
	"github.com/divyam234/teldrive/internal/utils"
	"github.com/divyam234/teldrive/pkg/database"

	cnf "github.com/divyam234/teldrive/config"
	"github.com/divyam234/teldrive/internal/cache"
	"github.com/gin-gonic/gin"
)

//	@title			Swagger Example API
//	@version		1.0
//	@description	This is a sample server celler server.
//	@termsOfService	http://swagger.io/terms/

//	@contact.name	API Support
//	@contact.url	http://www.swagger.io/support
//	@contact.email	support@swagger.io

//	@license.name	Apache 2.0
//	@license.url	http://www.apache.org/licenses/LICENSE-2.0.html

//	@host		localhost:5000
//	@BasePath	/api

//	@securityDefinitions.apikey	JwtAuth
//	@in							header
//	@name						Authorization

//	@externalDocs.description	OpenAPI
//	@externalDocs.url			https://swagger.io/resources/open-api/

func main() {

	gin.SetMode(gin.ReleaseMode)

	cnf.InitConfig()

	logger.InitLogger()

	database.InitDB()

	cache.InitCache()

	// scheduler := gocron.NewScheduler(time.UTC)

	// scheduler.Every(1).Hour().Do(cron.FilesDeleteJob)

	// scheduler.Every(12).Hour().Do(cron.UploadCleanJob)

	// scheduler.StartAsync()

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
