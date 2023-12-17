package api

import (
	"time"

	"github.com/divyam234/teldrive/pkg/controller"
	"github.com/divyam234/teldrive/pkg/middleware"
	"github.com/divyam234/teldrive/ui"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func InitRouter(logger *zap.Logger) *gin.Engine {

	r := gin.New()

	r.Use(ginzap.Ginzap(logger, time.RFC3339, true))

	r.Use(ginzap.RecoveryWithZap(logger, true))

	r.Use(middleware.Cors())

	c := controller.NewController(logger)

	api := r.Group("/api")
	{
		auth := api.Group("/auth")
		{
			auth.GET("/session", c.GetSession)
			auth.POST("/login", c.LogIn)
			auth.POST("/logout", middleware.Authmiddleware, c.Logout)
			auth.GET("/ws", c.HandleMultipleLogin)

		}
		files := api.Group("/files")
		{
			files.GET("", middleware.Authmiddleware, c.ListFiles)
			files.POST("", middleware.Authmiddleware, c.CreateFile)
			files.GET(":fileID", middleware.Authmiddleware, c.GetFileByID)
			files.PATCH(":fileID", middleware.Authmiddleware, c.UpdateFile)
			files.HEAD(":fileID/stream/:fileName", c.GetFileStream)
			files.GET(":fileID/stream/:fileName", c.GetFileStream)
			files.POST("/move", middleware.Authmiddleware, c.MoveFiles)
			files.POST("/directories", middleware.Authmiddleware, c.MakeDirectory)
			files.POST("/delete", middleware.Authmiddleware, c.DeleteFiles)
			files.POST("/copy", middleware.Authmiddleware, c.CopyFile)
			files.POST("/directories/move", middleware.Authmiddleware, c.MoveDirectory)
		}
		uploads := api.Group("/uploads")
		{
			uploads.Use(middleware.Authmiddleware)
			uploads.POST("/parts", c.CreateUploadPart)
			uploads.GET(":id", c.GetUploadFileById)
			uploads.POST(":id", c.UploadFile)
			uploads.DELETE(":id", c.DeleteUploadFile)
		}
		users := api.Group("/users")
		{
			users.Use(middleware.Authmiddleware)
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
