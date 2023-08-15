package routes

import (
	"net/http"

	"github.com/divyam234/teldrive/database"
	"github.com/divyam234/teldrive/services"

	"github.com/gin-gonic/gin"
)

func addAuthRoutes(rg *gin.RouterGroup) {

	r := rg.Group("/auth")

	authService := services.AuthService{Db: database.DB, SessionMaxAge: 30 * 24 * 60 * 60}

	r.POST("/login", func(c *gin.Context) {

		res, err := authService.LogIn(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}
		c.JSON(http.StatusOK, res)
	})

	r.GET("/logout", Authmiddleware, func(c *gin.Context) {

		res, err := authService.Logout(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}
		c.JSON(http.StatusOK, res)

	})

	r.GET("/ws", authService.HandleQrCodeLogin)

	r.GET("/session", func(c *gin.Context) {

		session := authService.GetSession(c)

		c.JSON(http.StatusOK, session)
	})

}
