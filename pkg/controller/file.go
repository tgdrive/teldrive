package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/pkg/httputil"
	"github.com/tgdrive/teldrive/pkg/schemas"
)

func (fc *Controller) CreateFile(c *gin.Context) {

	var fileIn schemas.FileIn

	if err := c.ShouldBindJSON(&fileIn); err != nil {
		httputil.NewError(c, http.StatusBadRequest, err)
		return
	}

	userId, _ := auth.GetUser(c)

	res, err := fc.FileService.CreateFile(c, userId, &fileIn)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}
	c.JSON(http.StatusCreated, res)
}

func (fc *Controller) UpdateFile(c *gin.Context) {

	userId, _ := auth.GetUser(c)

	var fileUpdate schemas.FileUpdate

	if err := c.ShouldBindJSON(&fileUpdate); err != nil {
		httputil.NewError(c, http.StatusBadRequest, err)
		return
	}
	res, err := fc.FileService.UpdateFile(c.Param("fileID"), userId, &fileUpdate)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) GetFileByID(c *gin.Context) {
	res, err := fc.FileService.GetFileByID(c.Param("fileID"))
	if err != nil {
		httputil.NewError(c, http.StatusNotFound, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) ListFiles(c *gin.Context) {

	userId, _ := auth.GetUser(c)

	fquery := schemas.FileQuery{
		Limit: 500,
		Page:  1,
		Order: "asc",
		Sort:  "name",
		Op:    "list",
	}

	if err := c.ShouldBindQuery(&fquery); err != nil {
		httputil.NewError(c, http.StatusBadRequest, err)
		return
	}

	res, err := fc.FileService.ListFiles(userId, &fquery)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) MakeDirectory(c *gin.Context) {

	userId, _ := auth.GetUser(c)

	var payload schemas.MkDir
	if err := c.ShouldBindJSON(&payload); err != nil {
		httputil.NewError(c, http.StatusBadRequest, err)
		return
	}
	res, err := fc.FileService.MakeDirectory(userId, &payload)
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

	userId, _ := auth.GetUser(c)

	var payload schemas.FileOperation
	if err := c.ShouldBindJSON(&payload); err != nil {
		httputil.NewError(c, http.StatusBadRequest, err)
		return
	}
	res, err := fc.FileService.MoveFiles(userId, &payload)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) DeleteFiles(c *gin.Context) {

	userId, _ := auth.GetUser(c)

	var payload schemas.DeleteOperation
	if err := c.ShouldBindJSON(&payload); err != nil {
		httputil.NewError(c, http.StatusBadRequest, err)
		return
	}
	res, err := fc.FileService.DeleteFiles(userId, &payload)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) CreateShare(c *gin.Context) {

	userId, _ := auth.GetUser(c)

	var payload schemas.FileShareIn
	if err := c.ShouldBindJSON(&payload); err != nil {
		httputil.NewError(c, http.StatusBadRequest, err)
		return
	}

	err := fc.FileService.CreateShare(c.Param("fileID"), userId, &payload)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.Status(http.StatusCreated)
}

func (fc *Controller) EditShare(c *gin.Context) {

	userId, _ := auth.GetUser(c)

	var payload schemas.FileShareIn
	if err := c.ShouldBindJSON(&payload); err != nil {
		httputil.NewError(c, http.StatusBadRequest, err)
		return
	}

	err := fc.FileService.UpdateShare(c.Param("shareID"), userId, &payload)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.Status(http.StatusOK)
}

func (fc *Controller) DeleteShare(c *gin.Context) {

	userId, _ := auth.GetUser(c)

	err := fc.FileService.DeleteShare(c.Param("fileID"), userId)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.Status(http.StatusNoContent)
}

func (fc *Controller) GetShareByFileId(c *gin.Context) {

	userId, _ := auth.GetUser(c)

	res, err := fc.FileService.GetShareByFileId(c.Param("fileID"), userId)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) UpdateParts(c *gin.Context) {

	userId, _ := auth.GetUser(c)

	var payload schemas.PartUpdate
	if err := c.ShouldBindJSON(&payload); err != nil {
		httputil.NewError(c, http.StatusBadRequest, err)
		return
	}

	res, err := fc.FileService.UpdateParts(c, c.Param("fileID"), userId, &payload)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) MoveDirectory(c *gin.Context) {
	userId, _ := auth.GetUser(c)

	var payload schemas.DirMove
	if err := c.ShouldBindJSON(&payload); err != nil {
		httputil.NewError(c, http.StatusBadRequest, err)
		return
	}
	res, err := fc.FileService.MoveDirectory(userId, &payload)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) GetCategoryStats(c *gin.Context) {
	userId, _ := auth.GetUser(c)

	res, err := fc.FileService.GetCategoryStats(userId)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (fc *Controller) GetFileStream(c *gin.Context) {
	fc.FileService.GetFileStream(c, false, nil)
}

func (fc *Controller) GetFileDownload(c *gin.Context) {
	fc.FileService.GetFileStream(c, true, nil)
}
