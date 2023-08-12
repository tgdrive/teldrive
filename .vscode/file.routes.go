package routes

import (
	"context"
	"net/http"
	"os"
	"strconv"

	"github.com/divyam234/teldrive-go/database"
	"github.com/divyam234/teldrive-go/services"

	"github.com/gin-gonic/gin"
)

func addFileRoutes(rg *gin.RouterGroup) {

	r := rg.Group("/files")
	r.Use(Authmiddleware)
	channelID, _ := strconv.ParseInt(os.Getenv("CHANNEL_ID"), 10, 64)
	fileService := services.FileService{Db: database.DB, ChannelID: channelID}

	r.GET("/", func(c *gin.Context) {
		res, err := fileService.ListFiles(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}

		c.JSON(http.StatusOK, res)
	})

	r.POST("/", func(c *gin.Context) {

		res, err := fileService.CreateFile(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}

		c.JSON(http.StatusOK, res)
	})

	r.GET("/:fileID", func(c *gin.Context) {

		res, err := fileService.GetFileByID(c)

		if err != nil {
			c.AbortWithError(http.StatusNotFound, err)
			return
		}

		c.JSON(http.StatusOK, res)
	})

	r.PATCH("/:fileID", func(c *gin.Context) {

		res, err := fileService.UpdateFile(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}

		c.JSON(http.StatusOK, res)
	})

	r.GET("/:fileID/:fileName", func(c *gin.Context) {

		fileService.GetFileStream(context.Background())(c)
	})

}
