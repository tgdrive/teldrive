package routes

import (
	"net/http"
	"time"

	"github.com/divyam234/teldrive/services"
	"github.com/divyam234/teldrive/utils/auth"
	"github.com/gin-gonic/gin"
	"github.com/go-jose/go-jose/v3/jwt"
)

func Authmiddleware(c *gin.Context) {
	cookie, err := c.Request.Cookie(services.GetUserSessionCookieName(c))

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing session cookie"})
	}

	now := time.Now().UTC()

	jwePayload, err := auth.Decode(cookie.Value)

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	}

	if *jwePayload.Expiry < *jwt.NewNumericDate(now) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token expired"})
	}

	c.Set("jwtUser", jwePayload)

	c.Next()

}
