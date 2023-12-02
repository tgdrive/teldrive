package api

import (
	"github.com/divyam234/teldrive/docs"
	"github.com/divyam234/teldrive/pkg/controller"
	"github.com/divyam234/teldrive/pkg/middleware"
	"github.com/divyam234/teldrive/ui"
	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func InitRouter() *gin.Engine {

	r := gin.Default()

	r.Use(gin.Logger())

	// if gin.Mode() == gin.ReleaseMode {
	// 	r.Use(middleware.Security())
	// }
	r.Use(middleware.Cors())

	docs.SwaggerInfo.BasePath = "/api"

	c := controller.NewController()

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
			uploads.Use(middleware.Authmiddleware)
			users.GET("/profile", c.GetProfilePhoto)
			users.GET("/stats", c.Stats)
			users.GET("/bots", c.GetBots)
			users.GET("/channels", c.ListChannels)
			users.PATCH("/channels", c.UpdateChannel)
			users.POST("/bots", c.AddBots)
			users.DELETE("/bots", c.RemoveBots)
			users.DELETE("/bots/session", c.RevokeBotSession)
		}
	}
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	ui.AddRoutes(r)

	return r
}
