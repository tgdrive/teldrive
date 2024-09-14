package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tgdrive/teldrive/pkg/httputil"
	"github.com/tgdrive/teldrive/pkg/schemas"
)

func (sc *Controller) GetShareById(c *gin.Context) {

	res, err := sc.ShareService.GetShareById(c.Param("shareID"))
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (sc *Controller) ShareUnlock(c *gin.Context) {
	var payload schemas.ShareAccess
	if err := c.ShouldBindJSON(&payload); err != nil {
		httputil.NewError(c, http.StatusBadRequest, err)
		return
	}
	err := sc.ShareService.ShareUnlock(c.Param("shareID"), &payload)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.Status(http.StatusOK)
}

func (sc *Controller) ListShareFiles(c *gin.Context) {

	query := schemas.ShareFileQuery{
		Limit: 500,
		Page:  1,
		Order: "asc",
		Sort:  "name",
	}

	if err := c.ShouldBindQuery(&query); err != nil {
		httputil.NewError(c, http.StatusBadRequest, err)
		return
	}

	res, err := sc.ShareService.ListShareFiles(c.Param("shareID"), &query, c.GetHeader("Authorization"))
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (sc *Controller) StreamSharedFile(c *gin.Context) {
	sc.ShareService.StreamSharedFile(c, false)
}

func (sc *Controller) DownloadSharedFile(c *gin.Context) {
	sc.ShareService.StreamSharedFile(c, true)
}
