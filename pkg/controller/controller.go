package controller

import (
	"github.com/divyam234/teldrive/pkg/database"
	"github.com/divyam234/teldrive/pkg/services"
	"go.uber.org/zap"
)

type Controller struct {
	FileService   *services.FileService
	UserService   *services.UserService
	UploadService *services.UploadService
	AuthService   *services.AuthService
}

func NewController(logger *zap.Logger) *Controller {
	return &Controller{
		FileService:   services.NewFileService(database.DB, logger),
		UserService:   services.NewUserService(database.DB, logger),
		UploadService: services.NewUploadService(database.DB, logger),
		AuthService:   services.NewAuthService(database.DB, logger),
	}
}
