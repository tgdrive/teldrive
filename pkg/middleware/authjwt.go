package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/divyam234/teldrive/internal/auth"
	"github.com/gin-gonic/gin"
	"github.com/go-jose/go-jose/v3/jwt"
)

func Authmiddleware(c *gin.Context) {

	var token string

	cookie, err := c.Request.Cookie("user-session")

	if err != nil {
		authHeader := c.GetHeader("Authorization")
		bearerToken := strings.Split(authHeader, "Bearer ")
		if len(bearerToken) != 2 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing auth token"})
			c.Abort()
			return
		}
		token = bearerToken[1]
	} else {
		token = cookie.Value
	}

	now := time.Now().UTC()

	jwePayload, err := auth.Decode(token)

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
