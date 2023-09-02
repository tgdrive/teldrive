package routes

import (
	"net/http"
	"time"

	"github.com/divyam234/teldrive/database"
	"github.com/divyam234/teldrive/services"
	"github.com/divyam234/teldrive/utils"
	"github.com/divyam234/teldrive/utils/auth"
	"github.com/gin-gonic/gin"
	"github.com/go-jose/go-jose/v3/jwt"
)

func Authmiddleware(c *gin.Context) {
	isSharedAPI := c.FullPath() == "/api/files" && c.DefaultQuery("op", "") == "shared"
	isFileAPI := c.FullPath() == "/api/files/:fileID/:fileName"
	isCheckFileVisibilityAPI := c.FullPath() == "/api/files/checkFileVisibility/:fileID"

	if isCheckFileVisibilityAPI {
		fileService := services.FileService{Db: database.DB, ChannelID: utils.GetConfig().ChannelID}
		fileVisibility := fileService.CheckFileVisibility(c.DefaultQuery("fileId", c.Param("fileID")))
		c.Set("fileVisibility", fileVisibility)
		c.Next()
		return
	}

	if isFileAPI || isSharedAPI {
		fileService := services.FileService{Db: database.DB, ChannelID: utils.GetConfig().ChannelID}
		fileVisibility := fileService.CheckFileVisibility(c.DefaultQuery("fileId", c.Param("fileID")))
		accessFromPublic := c.DefaultQuery("accessFromPublic", "")

		if fileVisibility == "public" && accessFromPublic == "true" {
			c.Set("fileVisibility", fileVisibility)
			c.Next()
			return
		}
	}

	cookie, err := c.Request.Cookie("user-session")

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing session cookie"})
		c.Abort()
		return
	}

	now := time.Now().UTC()

	jwePayload, err := auth.Decode(cookie.Value)

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		c.Abort()
		return
	}

	if *jwePayload.Expiry < *jwt.NewNumericDate(now) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token expired"})
		c.Abort()
		return
	}

	c.Set("jwtUser", jwePayload)

	c.Next()

}
