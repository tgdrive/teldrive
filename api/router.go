package api

import (
	"github.com/divyam234/teldrive/internal/config"
	"github.com/divyam234/teldrive/internal/middleware"
	"github.com/divyam234/teldrive/pkg/controller"
	"github.com/divyam234/teldrive/ui"
	"github.com/gin-gonic/gin"
)

func InitRouter(r *gin.Engine, c *controller.Controller, cnf *config.Config) *gin.Engine {
	authmiddleware := middleware.Authmiddleware(cnf.JWT.Secret)
	api := r.Group("/api")
	{
		auth := api.Group("/auth")
		{
			auth.GET("/session", c.GetSession)
			auth.POST("/login", c.LogIn)
			auth.POST("/logout", authmiddleware, c.Logout)
			auth.GET("/ws", c.HandleMultipleLogin)

		}
		files := api.Group("/files")
		{
			files.GET("", authmiddleware, c.ListFiles)
			files.POST("", authmiddleware, c.CreateFile)
			files.GET(":fileID", authmiddleware, c.GetFileByID)
			files.PATCH(":fileID", authmiddleware, c.UpdateFile)
			files.HEAD(":fileID/stream/:fileName", c.GetFileStream)
			files.GET(":fileID/stream/:fileName", c.GetFileStream)
			files.POST("/move", authmiddleware, c.MoveFiles)
			files.POST("/directories", authmiddleware, c.MakeDirectory)
			files.POST("/delete", authmiddleware, c.DeleteFiles)
			files.POST("/copy", authmiddleware, c.CopyFile)
			files.POST("/directories/move", authmiddleware, c.MoveDirectory)
		}
		uploads := api.Group("/uploads")
		{
			uploads.Use(authmiddleware)
			uploads.GET(":id", c.GetUploadFileById)
			uploads.POST(":id", c.UploadFile)
			uploads.DELETE(":id", c.DeleteUploadFile)
		}
		users := api.Group("/users")
		{
			users.Use(authmiddleware)
			users.GET("/profile", c.GetProfilePhoto)
			users.GET("/stats", c.GetStats)
			users.GET("/bots", c.GetBots)
			users.GET("/channels", c.ListChannels)
			users.PATCH("/channels", c.UpdateChannel)
			users.POST("/bots", c.AddBots)
			users.DELETE("/bots", c.RemoveBots)
		}
	}

	ui.AddRoutes(r)

	return r
}
