package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestTimeoutMiddleware(t *testing.T) {
	now := time.Now()
	s := setupRouterWithHandler(func(c *gin.Engine) {
		c.Use(TimeoutMiddleware(time.Second))
	}, func(c *gin.Context) {
		deadline, ok := c.Request.Context().Deadline()
		assert.True(t, ok)
		assert.LessOrEqual(t, deadline.Sub(now).Milliseconds(), time.Second.Milliseconds())
	})

	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "http://localhost/foo", nil)

	// when then
	s.ServeHTTP(res, req)
}

func setupRouterWithHandler(middlewareFunc func(c *gin.Engine), handler func(c *gin.Context)) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	middlewareFunc(r)
	r.GET("/foo", handler)
	return r
}
