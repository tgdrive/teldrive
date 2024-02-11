package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/divyam234/cors"
	"github.com/divyam234/teldrive/internal/auth"
	"github.com/gin-contrib/secure"
	"github.com/go-jose/go-jose/v3/jwt"

	"github.com/gin-gonic/gin"
)

func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)

		defer func() {
			if ctx.Err() == context.DeadlineExceeded {
				c.AbortWithStatus(http.StatusGatewayTimeout)
			}
			cancel()
		}()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func Cors() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders: []string{"Authorization", "Content-Length", "Content-Type"},
		AllowOriginFunc: func(origin string) bool {
			return true
		},
		MaxAge: 12 * time.Hour,
	})
}

func Authmiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		jwePayload, err := auth.Decode(secret, token)

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
}

func SecurityMiddleware() gin.HandlerFunc {
	return secure.New(secure.Config{
		STSSeconds:            315360000,
		STSIncludeSubdomains:  true,
		FrameDeny:             true,
		ContentTypeNosniff:    true,
		BrowserXssFilter:      true,
		ContentSecurityPolicy: "default-src 'self'",
		IENoOpen:              true,
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		SSLProxyHeaders:       map[string]string{"X-Forwarded-Proto": "https"},
	})
}
