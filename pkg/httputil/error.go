package httputil

import (
	"github.com/gin-gonic/gin"
	"github.com/tgdrive/teldrive/internal/logging"
)

func NewError(ctx *gin.Context, status int, err error) {
	logger := logging.FromContext(ctx)
	logger.Error(err)
	if status == 0 {
		status = 500
	}
	ctx.JSON(status, HTTPError{
		Code:    status,
		Message: err.Error(),
	})
}

type HTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
