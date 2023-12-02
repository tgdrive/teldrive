package controller

import (
	"github.com/divyam234/teldrive/pkg/database"
	"github.com/divyam234/teldrive/pkg/services"
)

type Controller struct {
	FileService   *services.FileService
	UserService   *services.UserService
	UploadService *services.UploadService
	AuthService   *services.AuthService
}

func NewController() *Controller {
	return &Controller{
		FileService:   services.NewFileService(database.DB),
		UserService:   services.NewUserService(database.DB),
		UploadService: services.NewUploadService(database.DB),
		AuthService:   services.NewAuthService(database.DB),
	}
}
