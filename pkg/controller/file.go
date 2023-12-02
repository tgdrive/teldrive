package controller

import (
	"net/http"

	"github.com/divyam234/teldrive/pkg/httputil"
	"github.com/gin-gonic/gin"
)

// CreateFile godoc
//	@Summary		Create a file
//	@Description	Create a new file by JSON payload
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			file	body		schemas.CreateFile	true	"Create File"
//	@Success		201		{object}	schemas.FileOut
//	@Failure		400		{object}	httputil.HTTPError
//	@Failure		404		{object}	httputil.HTTPError
//	@Failure		500		{object}	httputil.HTTPError
//	@Router			/files [post]

func (fc *Controller) CreateFile(c *gin.Context) {
	res, err := fc.FileService.CreateFile(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusCreated, res)
}

// UpdateFile godoc
//	@Summary		Update a file
//	@Description	Update a file by JSON payload
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			fileID	path		string				true	"File ID"
//	@Param			file	body		schemas.UpdateFile	true	"Update File"
//	@Success		200		{object}	schemas.FileOut
//	@Failure		400		{object}	httputil.HTTPError
//	@Failure		404		{object}	httputil.HTTPError
//	@Failure		500		{object}	httputil.HTTPError
//	@Router			/files/{fileID} [patch]

func (fc *Controller) UpdateFile(c *gin.Context) {
	res, err := fc.FileService.UpdateFile(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// GetFileByID godoc
//	@Summary		Get a file by ID
//	@Description	Get a file by its unique ID
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			fileID	path		string	true	"File ID"
//	@Success		200		{object}	schemas.FileOutFull
//	@Failure		404		{object}	httputil.HTTPError
//	@Router			/files/{fileID} [get]

func (fc *Controller) GetFileByID(c *gin.Context) {
	res, err := fc.FileService.GetFileByID(c)
	if err != nil {
		httputil.NewError(c, http.StatusNotFound, err)
		return
	}

	c.JSON(http.StatusOK, res)
}

// ListFiles godoc
//	@Summary		List files
//	@Description	List files with pagination and filtering options
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			perPage	query		int		false	"Items per page"
//	@Param			page	query		int		false	"Page number"
//	@Param			order	query		string	false	"Sort order (asc/desc)"
//	@Param			sortBy	query		string	false	"Sort by (name, size, etc.)"
//	@Success		200		{object}	schemas.FileResponse
//	@Failure		400		{object}	httputil.HTTPError
//	@Failure		500		{object}	httputil.HTTPError
//	@Router			/files [get]

func (fc *Controller) ListFiles(c *gin.Context) {
	res, err := fc.FileService.ListFiles(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// MakeDirectory godoc
//	@Summary		Make a directory
//	@Description	Create a new directory by JSON payload
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			directory	body		schemas.MkDir	true	"Make Directory"
//	@Success		200			{object}	schemas.FileOut
//	@Failure		400			{object}	httputil.HTTPError
//	@Failure		500			{object}	httputil.HTTPError
//	@Router			/directories [post]

func (fc *Controller) MakeDirectory(c *gin.Context) {
	res, err := fc.FileService.MakeDirectory(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// CopyFile godoc
//	@Summary		Copy a file
//	@Description	Copy a file to a new destination by JSON payload
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			copyRequest	body		schemas.Copy	true	"Copy File Request"
//	@Success		200			{object}	schemas.FileOut
//	@Failure		400			{object}	httputil.HTTPError
//	@Failure		500			{object}	httputil.HTTPError
//	@Router			/files/copy [post]

func (fc *Controller) CopyFile(c *gin.Context) {
	res, err := fc.FileService.CopyFile(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// MoveFiles godoc
//	@Summary		Move files
//	@Description	Move files to a new destination by JSON payload
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			moveRequest	body		schemas.FileOperation	true	"Move Files Request"
//	@Success		200			{object}	schemas.Message
//	@Failure		400			{object}	httputil.HTTPError
//	@Failure		500			{object}	httputil.HTTPError
//	@Router			/files/move [post]

func (fc *Controller) MoveFiles(c *gin.Context) {
	res, err := fc.FileService.MoveFiles(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// DeleteFiles godoc
//	@Summary		Delete files
//	@Description	Delete files by JSON payload
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			deleteRequest	body		schemas.FileOperation	true	"Delete Files Request"
//	@Success		200				{object}	schemas.Message
//	@Failure		400				{object}	httputil.HTTPError
//	@Failure		500				{object}	httputil.HTTPError
//	@Router			/files/delete [post]

func (fc *Controller) DeleteFiles(c *gin.Context) {
	res, err := fc.FileService.DeleteFiles(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// MoveDirectory godoc
//	@Summary		Move directory
//	@Description	Move a directory to a new destination by JSON payload
//	@Tags			files
//	@Accept			json
//	@Produce		json
//	@Param			moveDirRequest	body		schemas.DirMove	true	"Move Directory Request"
//	@Success		200				{object}	schemas.Message
//	@Failure		400				{object}	httputil.HTTPError
//	@Failure		500				{object}	httputil.HTTPError
//	@Router			/directories/move [post]

func (fc *Controller) MoveDirectory(c *gin.Context) {
	res, err := fc.FileService.MoveDirectory(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// GetFileStream godoc
//	@Summary		Get file stream
//	@Description	Get the stream of a file by ID
//	@Tags			files
//	@Accept			json
//	@Produce		octet-stream
//	@Param			fileID		path		string	true	"File ID"
//	@Param			fileName	path		string	true	"File Name"
//	@Param			hash		query		string	true	"Authentication hash"
//	@Success		200			{string}	string
//	@Failure		400			{object}	httputil.HTTPError
//	@Router			/files/{fileID}/stream/{fileNaae} [get]

func (fc *Controller) GetFileStream(c *gin.Context) {
	fc.FileService.GetFileStream(c)
}
