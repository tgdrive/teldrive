package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/divyam234/teldrive/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestHandleRequest(t *testing.T) {
	cases := []struct {
		Name string
		Func func(c *gin.Context) *Response
		// expected
		Code int
		Body string
	}{
		{
			Name: "Success with data",
			Func: func(c *gin.Context) *Response {
				return NewSuccessResponse(http.StatusOK, map[string]interface{}{
					"data": "ok",
				})
			},
			Code: http.StatusOK,
			Body: `
			{
				"data": "ok"
			}
			`,
		}, {
			Name: "Fail with ErrorResponse",
			Func: func(c *gin.Context) *Response {
				return NewErrorResponse(http.StatusBadRequest, InvalidQueryValue, "invalid query", nil)
			},
			Code: http.StatusBadRequest,
			Body: `
			{
				"code": "InvalidQueryValue",
				"message": "[InvalidQueryValue] invalid query"
			}
			`,
		}, {
			Name: "Fail with any error",
			Func: func(c *gin.Context) *Response {
				return &Response{
					Err: errors.New("any error"),
				}
			},
			Code: http.StatusInternalServerError,
			Body: `
			{
				"code": "InternalServerError",
				"message": "[InternalServerError] An error has occurred, please try again later"
			}
			`,
		}, {
			Name: "Timeout with ErrorResponse",
			Func: func(c *gin.Context) *Response {
				time.Sleep(250 * time.Millisecond)
				return nil
			},
			Code: http.StatusGatewayTimeout,
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			s := setupRouterWithHandler(func(c *gin.Engine) {
				c.Use(middleware.TimeoutMiddleware(200 * time.Millisecond))
			}, func(c *gin.Context) {
				HandleRequest(c, tc.Func)
			})

			res := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "http://localhost/foo", nil)

			// when
			s.ServeHTTP(res, req)

			// then
			assert.Equal(t, tc.Code, res.Code)
			if tc.Body != "" {
				assert.JSONEq(t, tc.Body, res.Body.String())
			}
		})
	}
}

func setupRouterWithHandler(middlewareFunc func(c *gin.Engine), handler func(c *gin.Context)) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	middlewareFunc(r)
	r.GET("/foo", handler)
	return r
}
