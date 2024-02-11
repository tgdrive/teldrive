package logging

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/magiconair/properties/assert"
	"go.uber.org/zap"
)

func TestDefaultLogger(t *testing.T) {
	repeat := 5
	var wait sync.WaitGroup
	loggerChan := make(chan *zap.SugaredLogger, repeat)

	for i := 0; i < repeat; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			loggerChan <- DefaultLogger()
		}()
	}
	wait.Wait()

	l := DefaultLogger()
	for i := 0; i < repeat; i++ {
		assert.Equal(t, <-loggerChan, l)
	}
}

func TestFromContext(t *testing.T) {
	cases := []struct {
		Name string
		Ctx  context.Context
	}{
		{
			Name: "Background context",
			Ctx:  context.Background(),
		}, {
			Name: "Gin context",
			Ctx: &gin.Context{
				Request: httptest.NewRequest("GET", "http://localhost:8080", nil),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			l1 := FromContext(tc.Ctx)

			ctx := WithLogger(tc.Ctx, l1)
			l2 := FromContext(ctx)

			assert.Equal(t, l2, l1)
		})
	}
}
