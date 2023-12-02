package controller

import (
	"net/http"

	"github.com/divyam234/teldrive/pkg/httputil"
	"github.com/gin-gonic/gin"
)

// GetSession godoc
//
//	@Summary		Get user session information
//	@Description	Get detailed information about the user's session.
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	schemas.Session
//	@Router			/auth/session [get]
func (ac *Controller) GetSession(c *gin.Context) {
	session := ac.AuthService.GetSession(c)

	c.JSON(http.StatusOK, session)
}

// LogIn godoc
//
//	@Summary		Log in to the application
//	@Description	Log in to the application with Telegram session details
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			session	body		schemas.TgSession	true	"Telegram Session Details"
//	@Success		200		{object}	schemas.Message
//	@Failure		400		{object}	httputil.HTTPError
//	@Failure		401		{object}	httputil.HTTPError
//	@Failure		500		{object}	httputil.HTTPError
//	@Router			/auth/login [post]
func (ac *Controller) LogIn(c *gin.Context) {
	res, err := ac.AuthService.LogIn(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// Logout godoc
//
//	@Summary		Log out from the application
//	@Description	Log out from the application and invalidate the session
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	schemas.Message
//	@Failure		500	{object}	httputil.HTTPError
//	@Router			/auth/logout [post]
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
