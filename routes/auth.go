package routes

import (
	"net/http"

	"github.com/divyam234/teldrive-go/services"

	"github.com/gin-gonic/gin"
)

func addAuthRoutes(rg *gin.RouterGroup) {

	r := rg.Group("/auth")

	authService := services.AuthService{SessionMaxAge: 30 * 24 * 60 * 60}

	r.POST("/login", func(c *gin.Context) {

		err := authService.LogIn(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}
	})

	r.GET("/logout", Authmiddleware, func(c *gin.Context) {

		err := authService.Logout(c)

		if err != nil {
			c.AbortWithError(err.Code, err.Error)
			return
		}

	})

	r.GET("/session", func(c *gin.Context) {

		session := authService.GetSession(c)

		c.JSON(http.StatusOK, session)
	})

}
