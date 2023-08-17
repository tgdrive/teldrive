package routes

import (
	"github.com/divyam234/teldrive/services"

	"github.com/gin-gonic/gin"
)

func addUserRoutes(rg *gin.RouterGroup) {
	r := rg.Group("/users")
	r.Use(Authmiddleware)
	userService := services.UserService{}

	r.GET("/profile", func(c *gin.Context) {
		if c.Query("photo") != "" {
			userService.GetProfilePhoto(c)
		}
	})
}
