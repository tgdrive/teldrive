package controller

import (
	"net/http"

	"github.com/divyam234/teldrive/pkg/httputil"
	"github.com/gin-gonic/gin"
)

// GetUploadFileById godoc
//	@Summary		Get information about an uploaded file
//	@Description	Get details of an uploaded file by its ID
//	@Tags			uploads
//	@Param			id	path	string	true	"Upload ID"
//	@Produce		json
//	@Success		200	{object}	schemas.UploadOut
//	@Failure		500	{object}	httputil.HTTPError
//	@Router			/uploads/{id} [get]
func (uc *Controller) GetUploadFileById(c *gin.Context) {
	res, err := uc.UploadService.GetUploadFileById(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// DeleteUploadFile godoc
//	@Summary		Delete an uploaded file
//	@Description	Delete an uploaded file by its ID
//	@Tags			uploads
//	@Param			id	path		string	true	"Upload ID"
//	@Success		200	{object}	schemas.Message
//	@Failure		500	{object}	httputil.HTTPError
//	@Router			/uploads/{id} [delete]
func (uc *Controller) DeleteUploadFile(c *gin.Context) {
	res, err := uc.UploadService.DeleteUploadFile(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// CreateUploadPart godoc
//	@Summary		Create a new upload part
//	@Description	Create a new upload part for a file
//	@Tags			uploads
//	@Accept			json
//	@Produce		json
//	@Param			part	body		schemas.UploadPart	true	"Upload Part"
//	@Success		201		{object}	schemas.UploadPartOut
//	@Failure		400		{object}	httputil.HTTPError
//	@Failure		500		{object}	httputil.HTTPError
//	@Router			/uploads/parts [post]
func (uc *Controller) CreateUploadPart(c *gin.Context) {
	res, err := uc.UploadService.CreateUploadPart(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusCreated, res)
}

// UploadFile godoc
//	@Summary		Upload a file
//	@Description	Upload a file in parts to a channel
//	@Tags			uploads
//	@Accept			application/octet-stream
//	@Param			id			path		string	true	"Upload ID"
//	@Param			filename	query		string	true	"File Name"
//	@Param			channelId	query		int		false	"Channel ID"
//	@Param			partNo		query		int		false	"Part Number"
//	@Param			totalParts	query		int		false	"Total Parts"
//	@Success		201			{object}	schemas.UploadPartOut
//	@Failure		400			{object}	httputil.HTTPError
//	@Failure		500			{object}	httputil.HTTPError
//	@Router			/uploads/{id} [post]
func (uc *Controller) UploadFile(c *gin.Context) {
	res, err := uc.UploadService.UploadFile(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusCreated, res)
}
