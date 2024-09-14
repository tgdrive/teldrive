package controller

import (
	"github.com/tgdrive/teldrive/pkg/services"
)

type Controller struct {
	FileService   *services.FileService
	UserService   *services.UserService
	UploadService *services.UploadService
	AuthService   *services.AuthService
	ShareService  *services.ShareService
}

func NewController(fileService *services.FileService,
	userService *services.UserService,
	uploadService *services.UploadService,
	authService *services.AuthService,
	shareService *services.ShareService) *Controller {
	return &Controller{
		FileService:   fileService,
		UserService:   userService,
		UploadService: uploadService,
		AuthService:   authService,
		ShareService:  shareService,
	}
}
