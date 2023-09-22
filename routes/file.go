package routes

import (
	"net/http"

	"github.com/divyam234/teldrive/database"
	"github.com/divyam234/teldrive/services"

	"github.com/gin-gonic/gin"
)

func addFileRoutes(rg *gin.RouterGroup) {

	r := rg.Group("/files")
	fileService := services.FileService{Db: database.DB}

	r.GET("", Authmiddleware, func(c *gin.Context) {
		res, err := fileService.ListFiles(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}

		c.JSON(http.StatusOK, res)
	})

	r.POST("", Authmiddleware, func(c *gin.Context) {

		res, err := fileService.CreateFile(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}

		c.JSON(http.StatusOK, res)
	})

	r.GET("/:fileID", Authmiddleware, func(c *gin.Context) {

		res, err := fileService.GetFileByID(c)

		if err != nil {
			c.AbortWithError(http.StatusNotFound, err)
			return
		}

		c.JSON(http.StatusOK, res)
	})

	r.PATCH("/:fileID", Authmiddleware, func(c *gin.Context) {

		res, err := fileService.UpdateFile(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}

		c.JSON(http.StatusOK, res)
	})

	r.HEAD("/:fileID/:fileName", func(c *gin.Context) {

		fileService.GetFileStream(c)
	})

	r.GET("/:fileID/:fileName", func(c *gin.Context) {

		fileService.GetFileStream(c)
	})

	r.POST("/movefiles", Authmiddleware, func(c *gin.Context) {

		res, err := fileService.MoveFiles(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}

		c.JSON(http.StatusOK, res)
	})

	r.POST("/makedir", Authmiddleware, func(c *gin.Context) {

		res, err := fileService.MakeDirectory(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}

		c.JSON(http.StatusOK, res)
	})

	r.POST("/deletefiles", Authmiddleware, func(c *gin.Context) {

		res, err := fileService.DeleteFiles(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}

		c.JSON(http.StatusOK, res)
	})

}
