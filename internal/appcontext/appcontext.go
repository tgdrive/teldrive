package appcontext

import (
	"context"
	"net/http"
)

type Context struct {
	Writer  http.ResponseWriter
	Request *http.Request
	context.Context
}

func newAppContext(w http.ResponseWriter, r *http.Request) *Context {
	return &Context{
		Writer:  w,
		Request: r,
		Context: r.Context(),
	}
}

func (c *Context) Write(code int, message string) {
	c.Writer.WriteHeader(code)
	c.Writer.Write([]byte(message))
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := newAppContext(w, r)
		next.ServeHTTP(ctx.Writer, r.WithContext(ctx))
	})
}
