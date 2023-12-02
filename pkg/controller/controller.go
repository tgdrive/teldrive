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
		FileService:   &services.FileService{Db: database.DB},
		UserService:   &services.UserService{Db: database.DB},
		UploadService: &services.UploadService{Db: database.DB},
		AuthService: &services.AuthService{
			Db:                database.DB,
			SessionMaxAge:     30 * 24 * 60 * 60,
			SessionCookieName: "user-session"},
	}
}
