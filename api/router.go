package api

import (
	"github.com/gin-gonic/gin"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/middleware"
	"github.com/tgdrive/teldrive/pkg/controller"
	"github.com/tgdrive/teldrive/ui"
	"gorm.io/gorm"
)

func InitRouter(r *gin.Engine, c *controller.Controller, cnf *config.Config, db *gorm.DB, cache cache.Cacher) *gin.Engine {
	authmiddleware := middleware.Authmiddleware(cnf.JWT.Secret, db, cache)
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
			files.HEAD(":fileID/download/:fileName", c.GetFileDownload)
			files.GET(":fileID/download/:fileName", c.GetFileDownload)
			files.PUT(":fileID/parts", authmiddleware, c.UpdateParts)
			files.POST(":fileID/share", authmiddleware, c.CreateShare)
			files.GET(":fileID/share", authmiddleware, c.GetShareByFileId)
			files.PATCH(":fileID/share", authmiddleware, c.EditShare)
			files.DELETE(":fileID/share", authmiddleware, c.DeleteShare)
			files.GET("/category/stats", authmiddleware, c.GetCategoryStats)
			files.POST("/move", authmiddleware, c.MoveFiles)
			files.POST("/directories", authmiddleware, c.MakeDirectory)
			files.POST("/delete", authmiddleware, c.DeleteFiles)
			files.POST("/copy", authmiddleware, c.CopyFile)
			files.POST("/directories/move", authmiddleware, c.MoveDirectory)
		}
		uploads := api.Group("/uploads")
		{
			uploads.Use(authmiddleware)
			uploads.GET("/stats", c.UploadStats)
			uploads.GET("/:id", c.GetUploadFileById)
			uploads.POST("/:id", c.UploadFile)
			uploads.DELETE("/:id", c.DeleteUploadFile)
		}
		users := api.Group("/users")
		{
			users.Use(authmiddleware)
			users.GET("/profile", c.GetProfilePhoto)
			users.GET("/stats", c.GetStats)
			users.GET("/channels", c.ListChannels)
			users.GET("/sessions", c.ListSessions)
			users.PATCH("/channels", c.UpdateChannel)
			users.POST("/bots", c.AddBots)
			users.DELETE("/bots", c.RemoveBots)
			users.DELETE("/sessions/:id", c.RemoveSession)
		}
		share := api.Group("/share")
		{
			share.GET("/:shareID", c.GetShareById)
			share.GET("/:shareID/files", c.ListShareFiles)
			share.GET("/:shareID/files/:fileID/stream/:fileName", c.StreamSharedFile)
			share.GET("/:shareID/files/:fileID/download/:fileName", c.StreamSharedFile)
			share.POST("/:shareID/unlock", c.ShareUnlock)
		}
	}

	ui.AddRoutes(r)

	return r
}
