package tgc

import (
	"context"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/stretchr/testify/assert"
	"github.com/tgdrive/teldrive/internal/config"
)

func chain(invoker tg.Invoker, middlewares []telegram.Middleware) tg.Invoker {
	for i := len(middlewares) - 1; i >= 0; i-- {
		invoker = middlewares[i].Handle(invoker)
	}
	return invoker
}

func floodInvoker(calls *int, errs ...error) tg.Invoker {
	return telegram.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		err := errs[*calls]
		*calls++
		return err
	})
}

func TestFloodWaitLimitFailsFastAndReports(t *testing.T) {
	var reported []time.Duration
	middlewares := NewMiddleware(&config.TGConfig{}, WithFloodWaitLimit(5*time.Second, func(d time.Duration) {
		reported = append(reported, d)
	}))

	calls := 0
	invoker := chain(floodInvoker(&calls, tgerr.New(420, "FLOOD_WAIT_30")), middlewares)

	start := time.Now()
	err := invoker.Invoke(context.Background(), nil, nil)

	d, ok := tgerr.AsFloodWait(err)
	assert.True(t, ok)
	assert.Equal(t, 30*time.Second, d)
	assert.Equal(t, 1, calls)
	assert.Equal(t, []time.Duration{30 * time.Second}, reported)
	assert.Less(t, time.Since(start), time.Second)
}

func TestFloodWaitLimitWaitsOutShortWaits(t *testing.T) {
	reported := 0
	middlewares := NewMiddleware(&config.TGConfig{}, WithFloodWaitLimit(5*time.Second, func(time.Duration) {
		reported++
	}))

	calls := 0
	invoker := chain(floodInvoker(&calls, tgerr.New(420, "FLOOD_WAIT_1"), nil), middlewares)

	err := invoker.Invoke(context.Background(), nil, nil)

	assert.NoError(t, err)
	assert.Equal(t, 2, calls)
	assert.Equal(t, 0, reported)
}
