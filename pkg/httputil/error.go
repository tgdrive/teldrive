package httputil

import (
	"github.com/divyam234/teldrive/internal/logging"
	"github.com/gin-gonic/gin"
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
