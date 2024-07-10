package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/divyam234/cors"
	"github.com/divyam234/teldrive/internal/auth"
	"github.com/divyam234/teldrive/internal/cache"
	"github.com/gin-contrib/secure"
	"gorm.io/gorm"

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

func Authmiddleware(secret string, db *gorm.DB, cache *cache.Cache) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := auth.VerifyUser(c, db, cache, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.Set("jwtUser", user)
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
