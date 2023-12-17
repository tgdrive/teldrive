package controller

import (
	"net/http"

	"github.com/divyam234/teldrive/pkg/httputil"
	"github.com/gin-gonic/gin"
)

func (fc *Controller) CreateFile(c *gin.Context) {
	res, err := fc.FileService.CreateFile(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusCreated, res)
}

func (fc *Controller) UpdateFile(c *gin.Context) {
	res, err := fc.FileService.UpdateFile(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) GetFileByID(c *gin.Context) {
	res, err := fc.FileService.GetFileByID(c)
	if err != nil {
		httputil.NewError(c, http.StatusNotFound, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) ListFiles(c *gin.Context) {
	res, err := fc.FileService.ListFiles(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) MakeDirectory(c *gin.Context) {
	res, err := fc.FileService.MakeDirectory(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) CopyFile(c *gin.Context) {
	res, err := fc.FileService.CopyFile(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) MoveFiles(c *gin.Context) {
	res, err := fc.FileService.MoveFiles(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) DeleteFiles(c *gin.Context) {
	res, err := fc.FileService.DeleteFiles(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) MoveDirectory(c *gin.Context) {
	res, err := fc.FileService.MoveDirectory(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) GetFileStream(c *gin.Context) {
	fc.FileService.GetFileStream(c)
}
