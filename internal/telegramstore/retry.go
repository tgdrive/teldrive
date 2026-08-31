package telegramstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

var transientTelegramErrors = []string{
	"RPC_CALL_FAIL",
	"RPC_MCGET_FAIL",
	"WORKER_BUSY_TOO_LONG_RETRY",
	"STORAGE_CHOOSE_VOLUME_FAILED",
}

var transientTelegramMessages = []string{
	"timeout",
	"timedout",
	"no workers running",
	"memory limit exit",
	"connection dead",
	"engine was closed",
	"broken pipe",
}

type retryMiddleware struct{ max int }

func (m retryMiddleware) Handle(next tg.Invoker) telegram.InvokeFunc {
	return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		for attempt := 0; ; attempt++ {
			err := next.Invoke(ctx, input, output)
			if err == nil {
				return nil
			}
			if attempt >= m.max || !isTransientTelegramError(err) {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}
}

func isTransientTelegramError(err error) bool {
	if tgerr.Is(err, transientTelegramErrors...) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range transientTelegramMessages {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func newRetryMiddleware(max int) (telegram.Middleware, error) {
	if max < 0 {
		return nil, fmt.Errorf("Telegram retry count cannot be negative")
	}
	return retryMiddleware{max: max}, nil
}
