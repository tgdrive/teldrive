package controller

import (
	"net/http"

	"github.com/divyam234/teldrive/pkg/httputil"
	"github.com/gin-gonic/gin"
)

// Stats godoc
//
//	@Summary		Get user's account statistics
//	@Description	Get statistics related to the authenticated user's account
//	@Tags			users
//	@Produce		json
//	@Success		200	{object}	schemas.AccountStats
//	@Failure		500	{object}	httputil.HTTPError
//	@Router			/users/stats [get]
func (uc *Controller) Stats(c *gin.Context) {
	res, err := uc.UserService.Stats(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// GetBots godoc
//
//	@Summary		Get user's bots for a channel
//	@Description	Get the list of bots associated with the authenticated user and a specific channel
//	@Tags			users
//	@Produce		json
//	@Param			channelId	path		int		true	"Channel ID"
//	@Success		200			{array}		string	"Bot Tokens"
//	@Failure		500			{object}	httputil.HTTPError
//	@Router			/users/bots/{channelId} [get]
func (uc *Controller) GetBots(c *gin.Context) {
	res, err := uc.UserService.GetBots(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// UpdateChannel godoc
//
//	@Summary		Update user's selected channel
//	@Description	Update the selected channel for the authenticated user
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			channel	body		schemas.Channel	true	"Update Channel"
//	@Success		200		{object}	schemas.Message
//	@Failure		400		{object}	httputil.HTTPError
//	@Failure		500		{object}	httputil.HTTPError
//	@Router			/users/channel [put]
func (uc *Controller) UpdateChannel(c *gin.Context) {
	res, err := uc.UserService.UpdateChannel(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// ListChannels godoc
//
//	@Summary		List user's channels
//	@Description	Get the list of channels associated with the authenticated user
//	@Tags			users
//	@Produce		json
//	@Success		200	{array}		schemas.Channel	"Channel List"
//	@Failure		500	{object}	httputil.HTTPError
//	@Router			/users/channels [get]
func (uc *Controller) ListChannels(c *gin.Context) {
	res, err := uc.UserService.ListChannels(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// AddBots godoc
//
//	@Summary		Add bots to user's channel
//	@Description	Add bots to the authenticated user's default channel
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			botsTokens	body		array	true	"Bot Tokens"
//	@Success		200			{object}	schemas.Message
//	@Failure		400			{object}	httputil.HTTPError
//	@Failure		500			{object}	httputil.HTTPError
//	@Router			/users/bots [post]
func (uc *Controller) AddBots(c *gin.Context) {
	res, err := uc.UserService.AddBots(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// RemoveBots godoc
//
//	@Summary		Remove bots from user's channel
//	@Description	Remove bots from the authenticated user's default channel
//	@Tags			users
//	@Success		200	{object}	schemas.Message
//	@Failure		500	{object}	httputil.HTTPError
//	@Router			/users/bots [delete]
func (uc *Controller) RemoveBots(c *gin.Context) {
	res, err := uc.UserService.RemoveBots(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

// RevokeBotSession godoc
//
//	@Summary		Revoke user's bot session
//	@Description	Revoke all bot sessions associated with the authenticated user
//	@Tags			users
//	@Success		200	{object}	schemas.Message
//	@Failure		500	{object}	httputil.HTTPError
//	@Router			/users/bots/session [delete]
func (uc *Controller) RevokeBotSession(c *gin.Context) {
	res, err := uc.UserService.RevokeBotSession(c)
	if err != nil {
		httputil.NewError(c, err.Code, err.Error)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (uc *Controller) GetProfilePhoto(c *gin.Context) {
	if c.Query("photo") != "" {
		uc.UserService.GetProfilePhoto(c)
	}
}
