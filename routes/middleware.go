package routes

import (
	"net/http"
	"time"

	"github.com/divyam234/teldrive/utils"
	"github.com/divyam234/teldrive/utils/auth"
	"github.com/gin-gonic/gin"
	"github.com/go-jose/go-jose/v3/jwt"
)

func Authmiddleware(c *gin.Context) {

	if c.FullPath() == "/api/files/:fileID/:fileName" && utils.GetConfig().MultiClient {
		c.Next()
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
