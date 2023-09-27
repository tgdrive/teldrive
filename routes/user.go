package routes

import (
	"net/http"

	"github.com/divyam234/teldrive/database"
	"github.com/divyam234/teldrive/services"

	"github.com/gin-gonic/gin"
)

func addUserRoutes(rg *gin.RouterGroup) {
	r := rg.Group("/users")
	r.Use(Authmiddleware)
	userService := services.UserService{Db: database.DB}

	r.GET("/profile", func(c *gin.Context) {
		if c.Query("photo") != "" {
			userService.GetProfilePhoto(c)
		}
	})

	r.GET("/stats", func(c *gin.Context) {
		res, err := userService.Stats(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}
		c.JSON(http.StatusOK, res)
	})

	r.GET("/bots", func(c *gin.Context) {
		res, err := userService.GetBots(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}
		c.JSON(http.StatusOK, res)
	})

	r.GET("/channels", func(c *gin.Context) {
		res, err := userService.ListChannels(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}
		c.JSON(http.StatusOK, res)
	})

	r.PATCH("/channels", func(c *gin.Context) {
		res, err := userService.UpdateChannel(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}
		c.JSON(http.StatusOK, res)

	})

	r.POST("/bots", func(c *gin.Context) {

		res, err := userService.AddBots(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}

		c.JSON(http.StatusCreated, res)
	})

	r.DELETE("/bots", func(c *gin.Context) {

		res, err := userService.RemoveBots(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}

		c.JSON(http.StatusOK, res)
	})

	r.GET("/bots/revoke", func(c *gin.Context) {

		res, err := userService.RevokeBotSession(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}

		c.JSON(http.StatusOK, res)
	})

	r.DELETE("/cache", func(c *gin.Context) {
		res, err := userService.ClearCache(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}

		c.JSON(http.StatusOK, res)
	})
}
