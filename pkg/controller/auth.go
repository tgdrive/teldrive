package controller

import (
	"net/http"

	"github.com/divyam234/teldrive/pkg/httputil"
	"github.com/gin-gonic/gin"
)

func (ac *Controller) GetSession(c *gin.Context) {
	session := ac.AuthService.GetSession(c)

	c.JSON(http.StatusOK, session)
}

func (ac *Controller) LogIn(c *gin.Context) {
	res, err := ac.AuthService.LogIn(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (ac *Controller) Logout(c *gin.Context) {
	res, err := ac.AuthService.Logout(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (ac *Controller) HandleMultipleLogin(c *gin.Context) {
	ac.AuthService.HandleMultipleLogin(c)
}
